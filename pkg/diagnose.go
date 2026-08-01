package pkg

import (
	"fmt"
	"regexp"
	"strings"
)

// Diagnosis explains one recognised build failure in the terms the user needs:
// what went wrong, and what to do about it.
//
// This exists because a failing build is loud but unhelpful. A Rust toolchain
// that is too old prints the requirement once and then repeats it for all
// twenty-odd crates in the workspace, so the line that matters scrolls away
// under twenty that do not. The information was always there; finding it meant
// reading the whole log and knowing what to look for.
type Diagnosis struct {
	// Cause is a one-line statement of what actually failed.
	Cause string
	// Fix is the action to take. Empty when there is nothing to suggest
	// beyond the cause itself.
	Fix string
}

// matcher recognises one class of failure in build output.
type matcher struct {
	re *regexp.Regexp
	// diagnose builds the Diagnosis from the regexp's submatches. Returning a
	// zero Diagnosis discards the match, which lets a matcher back out once it
	// has seen the captured values.
	diagnose func(m []string) Diagnosis
}

// matchers are tried in order, and each contributes at most one diagnosis, so
// the most specific patterns come first: "command not found" would otherwise
// swallow a missing compiler that has a better message of its own.
//
// Every pattern here comes from a failure that actually happened rather than
// one imagined for completeness — the toolchain and C standard entries are the
// two that cost real time to work out by hand.
var matchers = []matcher{
	{
		// cargo prints this once per crate in a workspace: "package `yazi-fm
		// v26.5.6` cannot be built because it requires rustc 1.95.0 or newer,
		// while the currently active rustc version is 1.90.0".
		re: regexp.MustCompile(`requires rustc ([0-9][0-9.]*) or newer, while the currently active rustc version is ([0-9][0-9.]*)`),
		diagnose: func(m []string) Diagnosis {
			return Diagnosis{
				Cause: fmt.Sprintf("this package needs Rust %s, but %s is active", m[1], m[2]),
				Fix:   fmt.Sprintf("install a newer toolchain: `rustup update stable`, or `mise use -g rust@%s` if mise manages it", m[1]),
			}
		},
	},
	{
		// The short form, printed without the active version.
		re: regexp.MustCompile(`(?m)^\s*\S+@\S+ requires rustc ([0-9][0-9.]*)\s*$`),
		diagnose: func(m []string) Diagnosis {
			return Diagnosis{
				Cause: fmt.Sprintf("this package needs Rust %s or newer", m[1]),
				Fix:   fmt.Sprintf("install a newer toolchain: `rustup update stable`, or `mise use -g rust@%s` if mise manages it", m[1]),
			}
		},
	},
	{
		re: regexp.MustCompile(`go\.mod requires go >= ([0-9][0-9.]*)`),
		diagnose: func(m []string) Diagnosis {
			return Diagnosis{
				Cause: fmt.Sprintf("this package needs Go %s or newer", m[1]),
				Fix:   fmt.Sprintf("install a newer Go, e.g. `mise use -g go@%s`", m[1]),
			}
		},
	},
	{
		// GCC 14 promoted this to an error; GCC 15 defaults to C23, where an
		// empty parameter list means "no parameters". Pre-C23 C fails on both.
		re: regexp.MustCompile(`(?:error: )?(incompatible pointer type|too many arguments to function)`),
		diagnose: func([]string) Diagnosis {
			return Diagnosis{
				Cause: "a C dependency does not build under this compiler's defaults (GCC 14+ rejects code that older versions accepted)",
				Fix:   "the package needs `install.environment` with `CFLAGS: -std=gnu17 -Wno-incompatible-pointer-types` — report it against the registry entry",
			}
		},
	},
	{
		// pkg-config reports the same failure in three different wordings
		// depending on who invoked it; kitty's setup.py produces the third.
		re: regexp.MustCompile(`Package '([^']+)', required by [^,]+, not found` +
			`|No package '([^']+)' found` +
			`|Package '([^']+)' not found`),
		diagnose: func(m []string) Diagnosis {
			name := m[1]
			for _, alt := range m[2:] {
				if name != "" {
					break
				}
				name = alt
			}
			return Diagnosis{
				Cause: fmt.Sprintf("the build needs the %s development files, which pkg-config cannot find", name),
				Fix:   fmt.Sprintf("install your distribution's %s development package (often named %s-devel or lib%s-dev)", name, name, name),
			}
		},
	},
	{
		re: regexp.MustCompile(`cannot find -l(\S+)`),
		diagnose: func(m []string) Diagnosis {
			return Diagnosis{
				Cause: fmt.Sprintf("the linker cannot find lib%s", m[1]),
				Fix:   fmt.Sprintf("install the development package that provides lib%s (often lib%s-dev or %s-devel)", m[1], m[1], m[1]),
			}
		},
	},
	{
		// The second alternative captures the whole path token, separators
		// included, so the guard below can tell a PATH lookup from a file.
		re: regexp.MustCompile(`(?:^|[:\s])([\w.+-]+): command not found|([\w./+-]+): No such file or directory\s*$`),
		diagnose: func(m []string) Diagnosis {
			name := m[1]
			if name == "" {
				name = m[2]
			}
			// "No such file or directory" covers two different failures. A bare
			// name is a PATH lookup that found nothing — a missing tool. A name
			// with a separator in it is a FILE the build expected to exist,
			// which is a broken package, not something the user can install.
			if name == "" || strings.ContainsAny(name, "/.") {
				return Diagnosis{}
			}
			return Diagnosis{
				Cause: fmt.Sprintf("the build needs %q, which is not installed or not on PATH", name),
				Fix:   fmt.Sprintf("install %s and try again", name),
			}
		},
	},
	{
		re: regexp.MustCompile(`No space left on device`),
		diagnose: func([]string) Diagnosis {
			return Diagnosis{
				Cause: "the disk filled up during the build",
				Fix:   "free some space; `clipack` builds under the configured build directory, which you can clear",
			}
		},
	},
	{
		re: regexp.MustCompile(`(?:Could not resolve host|Connection refused|Temporary failure in name resolution|unable to access '[^']+')`),
		diagnose: func([]string) Diagnosis {
			return Diagnosis{
				Cause: "the build could not reach the network",
				Fix:   "check the connection and retry; nothing is left half-installed",
			}
		},
	},
	{
		re: regexp.MustCompile(`(?m)^error: (?:pathspec|Remote branch) .*(?:did not match|not found)`),
		diagnose: func([]string) Diagnosis {
			return Diagnosis{
				Cause: "the version this package is pinned to does not exist upstream any more",
				Fix:   "refresh the registry, or install with `--method commit` to follow the default branch instead",
			}
		},
	},
}

// Diagnose scans captured build output and returns what it recognises.
//
// Order follows `matchers`, and each matcher fires at most once however many
// times its pattern occurs: the twenty-crate Rust workspace above is one
// problem, not twenty.
func Diagnose(output []string) []Diagnosis {
	if len(output) == 0 {
		return nil
	}
	joined := strings.Join(output, "\n")

	var out []Diagnosis
	for _, m := range matchers {
		hit := m.re.FindStringSubmatch(joined)
		if hit == nil {
			continue
		}
		if d := m.diagnose(hit); d.Cause != "" {
			out = append(out, d)
		}
	}
	return out
}

// outputTail is a bounded ring of the most recent output lines.
//
// Bounded because a build log is unbounded: a full cargo build of a large
// workspace runs to tens of thousands of lines, and holding all of it to scan
// after the fact would cost more memory than the build. The error that killed a
// step is always near the end.
type outputTail struct {
	lines []string
	limit int
}

func newOutputTail(limit int) *outputTail {
	return &outputTail{limit: limit}
}

func (t *outputTail) add(line string) {
	if len(t.lines) == t.limit {
		copy(t.lines, t.lines[1:])
		t.lines[len(t.lines)-1] = line
		return
	}
	t.lines = append(t.lines, line)
}

func (t *outputTail) all() []string {
	out := make([]string, len(t.lines))
	copy(out, t.lines)
	return out
}
