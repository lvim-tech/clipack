package pkg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Desktop entries are installed under the user's data directory, never under a
// system one. A launcher reads both, and the user directory needs no root: an
// installer that asks for a password to add a menu entry is an installer people
// stop using.
const (
	// desktopFilePrefix marks the files clipack owns. Removal only ever deletes
	// a file carrying it, so a package whose entry is named like the distro's
	// cannot make clipack delete the distro's.
	desktopFilePrefix = "clipack-"
	// desktopNameSuffix distinguishes the entry from a system package for the
	// same program. Both are installed, both are listed, and the launcher has to
	// give the user a way to tell them apart.
	desktopNameSuffix = " (clipack)"
)

// dataHome is the base directory for user-specific data files.
func dataHome() (string, error) {
	if dir := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not get home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share"), nil
}

// desktopPaths resolves where one entry's files are installed. The icon lives in
// a clipack-owned directory rather than in the icon theme: an absolute Icon= is
// valid per the desktop entry specification, and it keeps the icon caches of the
// system untouched by an install that needs no privileges.
func desktopPaths(pkgName, source string) (entry string, iconDir string, err error) {
	base, err := dataHome()
	if err != nil {
		return "", "", err
	}

	name := filepath.Base(source)
	if name == "." || name == string(filepath.Separator) || name == "" {
		return "", "", fmt.Errorf("desktop entry source %q has no file name", source)
	}
	if !strings.HasSuffix(name, ".desktop") {
		return "", "", fmt.Errorf("desktop entry source %q is not a .desktop file", source)
	}

	entry = filepath.Join(base, "applications", desktopFilePrefix+pkgName+"-"+name)
	iconDir = filepath.Join(base, "clipack", "icons", pkgName)
	return entry, iconDir, nil
}

// desktopRewrite is what an entry has to be adapted to before it describes the
// copy clipack installed rather than whatever the distro ships.
type desktopRewrite struct {
	// BinDir is clipack's bin directory. A program named in Exec is repointed at
	// it when the binary is there.
	BinDir string
	// Env holds pre-rendered K=V pairs, sorted, written into Exec as an
	// `env K=V …` prefix. Exec only: TryExec must stay a bare executable path,
	// because launchers stat it rather than run it.
	Env []string
	// Name replaces the displayed name outright. Empty appends the suffix to
	// whatever the file already says.
	Name string
	// Icon is an absolute path to an installed icon. Empty leaves Icon= alone,
	// which falls back to the icon theme.
	Icon string
}

// rewriteDesktopEntry adapts a shipped .desktop file to clipack's installation.
//
// The rewriting is the whole point of the feature. A shipped entry says
// `Exec=kitty` and relies on PATH, so with a system package installed the menu
// entry launches *that* one — the clipack build would be listed and never run.
// Pinning Exec to an absolute path is what makes the two entries mean two
// different programs.
func rewriteDesktopEntry(contents []byte, rw desktopRewrite) []byte {
	var out strings.Builder
	group := ""

	for _, line := range strings.Split(string(contents), "\n") {
		trimmed := strings.TrimSpace(line)

		// Group headers decide what a key means: Name in [Desktop Entry] is the
		// program's name, while Name in a [Desktop Action …] group labels a
		// right-click action. Suffixing the latter would rename "New window" to
		// "New window (clipack)".
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			group = trimmed
			out.WriteString(line + "\n")
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found {
			out.WriteString(line + "\n")
			continue
		}

		switch {
		case strings.TrimSpace(key) == "TryExec":
			out.WriteString(key + "=" + rewriteExec(value, rw.BinDir) + "\n")

		case strings.TrimSpace(key) == "Exec":
			// Exec is rewritten in every group: the actions launch the program too.
			out.WriteString(key + "=" + envPrefix(rw.Env) + rewriteExec(value, rw.BinDir) + "\n")

		case group == "[Desktop Entry]" && isNameKey(key) && rw.Name != "":
			out.WriteString(key + "=" + rw.Name + "\n")

		case group == "[Desktop Entry]" && isNameKey(key):
			out.WriteString(key + "=" + strings.TrimRight(value, " ") + desktopNameSuffix + "\n")

		case group == "[Desktop Entry]" && strings.TrimSpace(key) == "Icon" && rw.Icon != "":
			out.WriteString(key + "=" + rw.Icon + "\n")

		default:
			out.WriteString(line + "\n")
		}
	}

	// Split on "\n" produces a trailing empty element for a file that ended in a
	// newline, and writing it back added one line too many.
	return []byte(strings.TrimSuffix(out.String(), "\n"))
}

// envPrefix renders the `env K=V … ` prefix for an Exec line, or nothing when
// there is no environment to carry. Values with spaces are quoted the way the
// desktop entry specification quotes arguments.
func envPrefix(pairs []string) string {
	if len(pairs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(pairs)+1)
	parts = append(parts, "env")
	for _, pair := range pairs {
		if strings.ContainsAny(pair, " \t") {
			pair = `"` + pair + `"`
		}
		parts = append(parts, pair)
	}
	return strings.Join(parts, " ") + " "
}

// isNameKey reports whether a key is the display name or one of its localised
// variants. Suffixing Name but not Name[bg] leaves a Bulgarian desktop showing
// two entries with the same label — the exact confusion this exists to prevent.
func isNameKey(key string) bool {
	key = strings.TrimSpace(key)
	return key == "Name" || (strings.HasPrefix(key, "Name[") && strings.HasSuffix(key, "]"))
}

// rewriteExec repoints the program in an Exec value at binDir, leaving its
// arguments untouched. A program that is not in binDir is left alone: the entry
// may legitimately call something else, and a path to a file that does not exist
// is worse than the name it replaced.
func rewriteExec(value, binDir string) string {
	program, rest := splitExecProgram(strings.TrimSpace(value))
	if program == "" {
		return value
	}

	target := filepath.Join(binDir, filepath.Base(program))
	if _, err := os.Stat(target); err != nil {
		return value
	}

	if strings.ContainsAny(target, " \t") {
		target = `"` + target + `"`
	}
	if rest == "" {
		return target
	}
	return target + " " + rest
}

// splitExecProgram separates the program from its arguments, honouring the
// quoting the desktop entry specification allows for paths with spaces.
func splitExecProgram(value string) (program, rest string) {
	if strings.HasPrefix(value, `"`) {
		if end := strings.Index(value[1:], `"`); end >= 0 {
			return value[1 : end+1], strings.TrimSpace(value[end+2:])
		}
		return "", value
	}

	program, rest, found := strings.Cut(value, " ")
	if !found {
		return value, ""
	}
	return program, strings.TrimSpace(rest)
}
