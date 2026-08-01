package pkg

import (
	"runtime"
	"strings"
	"testing"
)

// yaziToolchainFailure is the real output cargo produced when yazi v26.5.6 was
// installed against Rust 1.90. It is kept verbatim, repetition included: the
// repetition is the reason this feature exists, since the one line that matters
// is followed by twenty that say the same thing about other crates.
const yaziToolchainFailure = `    Updating crates.io index
error: rustc 1.90.0 is not supported by the following packages:
  yazi-adapter@26.5.6 requires rustc 1.95.0
  yazi-boot@26.5.6 requires rustc 1.95.0
  yazi-config@26.5.6 requires rustc 1.95.0
  yazi-core@26.5.6 requires rustc 1.95.0
  yazi-dds@26.5.6 requires rustc 1.95.0
  yazi-fm@26.5.6 requires rustc 1.95.0
  yazi-widgets@26.5.6 requires rustc 1.95.0`

func TestDiagnoseRealToolchainFailure(t *testing.T) {
	got := Diagnose(strings.Split(yaziToolchainFailure, "\n"))

	if len(got) != 1 {
		t.Fatalf("got %d diagnoses, want exactly 1 — the same problem repeated per crate is one problem: %+v", len(got), got)
	}
	for _, want := range []string{"1.95.0"} {
		if !strings.Contains(got[0].Cause, want) {
			t.Errorf("cause %q does not mention %q", got[0].Cause, want)
		}
	}
	if !strings.Contains(got[0].Fix, "rust@1.95.0") {
		t.Errorf("fix %q does not name the version to install", got[0].Fix)
	}
}

func TestDiagnoseCargoLongForm(t *testing.T) {
	out := []string{
		"error: package `yazi-fm v26.5.6` cannot be built because it requires rustc 1.95.0 or newer, " +
			"while the currently active rustc version is 1.90.0",
	}
	got := Diagnose(out)
	if len(got) == 0 {
		t.Fatal("the long form of the toolchain error was not recognised")
	}
	// This form knows both versions, so the message says what is active too.
	for _, want := range []string{"1.95.0", "1.90.0"} {
		if !strings.Contains(got[0].Cause, want) {
			t.Errorf("cause %q does not mention %q", got[0].Cause, want)
		}
	}
}

func TestDiagnoseRecognisedFailures(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		wantIn   string
		wantFix  string
		wantNone bool
	}{
		{
			name:    "go toolchain",
			output:  "go: go.mod requires go >= 1.24.0 (running go 1.22.1)",
			wantIn:  "Go 1.24.0",
			wantFix: "go@1.24.0",
		},
		{
			name:   "C23 parameter list",
			output: "st.c:1234:5: error: too many arguments to function 'func'",
			wantIn: "C dependency",
		},
		{
			name:    "incompatible pointer types",
			output:  "regparse.c:5678:9: error: incompatible pointer type passed to argument",
			wantIn:  "C dependency",
			wantFix: "CFLAGS",
		},
		{
			name:    "pkg-config",
			output:  "Package 'fontconfig', required by 'virtual:world', not found",
			wantIn:  "fontconfig",
			wantFix: "fontconfig",
		},
		{
			name:   "pkg-config short form",
			output: "No package 'harfbuzz' found",
			wantIn: "harfbuzz",
		},
		{
			name:    "missing library",
			output:  "/usr/bin/ld: cannot find -lxcb-xfixes",
			wantIn:  "libxcb-xfixes",
			wantFix: "xcb-xfixes",
		},
		{
			name:    "missing tool",
			output:  "/bin/sh: line 1: cmake: command not found",
			wantIn:  `"cmake"`,
			wantFix: "install cmake",
		},
		{
			name:   "disk full",
			output: "error: failed to write; No space left on device",
			wantIn: "disk filled up",
		},
		{
			name:   "offline",
			output: "fatal: unable to access 'https://github.com/x/y.git/': Could not resolve host: github.com",
			wantIn: "network",
		},
		{
			name:   "missing tag",
			output: "error: pathspec 'v9.9.9' did not match any file(s) known to git",
			wantIn: "pinned to does not exist",
		},
		{
			// The point of the whole design: silence beats invention. A build
			// can fail in ways no pattern here knows, and a confident guess is
			// worse than the raw error the user has already read.
			name:     "unrecognised failure says nothing",
			output:   "make: *** [Makefile:42: all] Error 2",
			wantNone: true,
		},
		{
			name:     "empty output",
			output:   "",
			wantNone: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var lines []string
			if tt.output != "" {
				lines = strings.Split(tt.output, "\n")
			}
			got := Diagnose(lines)

			if tt.wantNone {
				if len(got) != 0 {
					t.Fatalf("expected no diagnosis, got %+v", got)
				}
				return
			}
			if len(got) == 0 {
				t.Fatalf("no diagnosis for %q", tt.output)
			}
			if !strings.Contains(got[0].Cause, tt.wantIn) {
				t.Errorf("cause = %q, want it to contain %q", got[0].Cause, tt.wantIn)
			}
			if tt.wantFix != "" && !strings.Contains(got[0].Fix, tt.wantFix) {
				t.Errorf("fix = %q, want it to contain %q", got[0].Fix, tt.wantFix)
			}
		})
	}
}

// A plain missing FILE is not a missing tool, and there is nothing useful to
// suggest about it — the matcher has to tell the two apart.
func TestDiagnoseDoesNotTreatAMissingFileAsAMissingTool(t *testing.T) {
	for _, line := range []string{
		"cp: cannot stat 'target/release/demo': No such file or directory",
		"/bin/sh: ./configure: No such file or directory",
	} {
		if got := Diagnose([]string{line}); len(got) != 0 {
			t.Errorf("Diagnose(%q) = %+v, want nothing", line, got)
		}
	}
}

func TestOutputTailKeepsTheEnd(t *testing.T) {
	tail := newOutputTail(3)
	for _, l := range []string{"a", "b", "c", "d", "e"} {
		tail.add(l)
	}

	got := strings.Join(tail.all(), ",")
	// The error that killed a step is at the end, so that is what survives.
	if got != "c,d,e" {
		t.Errorf("tail = %q, want %q", got, "c,d,e")
	}
}

func TestOutputTailBelowLimit(t *testing.T) {
	tail := newOutputTail(10)
	tail.add("only")
	if got := tail.all(); len(got) != 1 || got[0] != "only" {
		t.Errorf("tail = %v, want [only]", got)
	}
}

// The tail is written from both output pipes at once, so the copy handed to
// Diagnose must not alias the buffer still being appended to.
func TestOutputTailReturnsACopy(t *testing.T) {
	tail := newOutputTail(3)
	tail.add("first")
	snapshot := tail.all()
	tail.add("second")
	tail.add("third")
	tail.add("fourth")

	if snapshot[0] != "first" {
		t.Errorf("the snapshot changed under the caller: %q", snapshot[0])
	}
}

// End to end: a step that fails with a recognised error emits the hint through
// the reporter, which is what both interfaces render.
func TestFailingStepEmitsAHint(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shell syntax under test is POSIX")
	}

	config := testConfig(t)
	rec := &recorder{}
	in := NewInstaller(config, rec.report)

	p := &Package{
		Name:    "demo",
		Version: "v1.0.0",
		Install: Install{
			Steps: []string{
				`echo "  yazi-fm@26.5.6 requires rustc 1.95.0" >&2; exit 1`,
			},
			Binaries: []string{"out/demo"},
		},
	}

	if err := in.Install(p, MethodVersion); err == nil {
		t.Fatal("Install() succeeded despite a failing step")
	}

	hints := rec.texts(EventHint)
	if len(hints) == 0 {
		t.Fatal("a recognised failure produced no hint")
	}
	if !strings.Contains(hints[0], "1.95.0") {
		t.Errorf("hint = %q, want the required version in it", hints[0])
	}
}

func TestUnrecognisedFailureEmitsNoHint(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shell syntax under test is POSIX")
	}

	config := testConfig(t)
	rec := &recorder{}
	in := NewInstaller(config, rec.report)

	p := &Package{
		Name:    "demo",
		Version: "v1.0.0",
		Install: Install{
			Steps:    []string{`echo "something went wrong in a way nobody predicted" >&2; exit 1`},
			Binaries: []string{"out/demo"},
		},
	}

	if err := in.Install(p, MethodVersion); err == nil {
		t.Fatal("Install() succeeded despite a failing step")
	}
	if hints := rec.texts(EventHint); len(hints) != 0 {
		t.Errorf("an unrecognised failure invented advice: %v", hints)
	}
}

// Only the failing step's output is diagnosed. An earlier step that printed
// something matchable and then SUCCEEDED must not be blamed for a later failure.
func TestHintComesFromTheStepThatFailed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shell syntax under test is POSIX")
	}

	config := testConfig(t)
	rec := &recorder{}
	in := NewInstaller(config, rec.report)

	p := &Package{
		Name:    "demo",
		Version: "v1.0.0",
		Install: Install{
			Steps: []string{
				`echo "  something@1.0 requires rustc 9.9.9"`, // succeeds
				`echo "No space left on device" >&2; exit 1`,  // fails
			},
			Binaries: []string{"out/demo"},
		},
	}

	if err := in.Install(p, MethodVersion); err == nil {
		t.Fatal("Install() succeeded despite a failing step")
	}

	hints := strings.Join(rec.texts(EventHint), "\n")
	if !strings.Contains(hints, "disk filled up") {
		t.Errorf("hints = %q, want the failing step's cause", hints)
	}
	if strings.Contains(hints, "9.9.9") {
		t.Errorf("hints = %q, want nothing from the step that succeeded", hints)
	}
}

// The third pkg-config wording, from kitty's setup.py. Missed by the first
// version of this matcher on a real install.
func TestDiagnosePkgConfigThirdWording(t *testing.T) {
	out := []string{
		"Package libxxhash was not found in the pkg-config search path.",
		"Perhaps you should add the directory containing `libxxhash.pc'",
		"to the PKG_CONFIG_PATH environment variable",
		"Package 'libxxhash' not found",
	}
	got := Diagnose(out)
	if len(got) == 0 {
		t.Fatal("the kitty/setup.py pkg-config wording was not recognised")
	}
	if !strings.Contains(got[0].Cause, "libxxhash") {
		t.Errorf("cause = %q, want it to name libxxhash", got[0].Cause)
	}
}

// Both of these stopped a real kitty build, and neither produced a hint before
// this matcher existed: the older "No such file or directory" rule deliberately
// ignores anything with a path separator in it, which every header has.
func TestDiagnoseMissingHeader(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		wantIn  string
		wantFix string
	}{
		{
			name:    "xkbcommon",
			output:  "kitty/keys.c:15:10: fatal error: xkbcommon/xkbcommon.h: No such file or directory",
			wantIn:  "xkbcommon/xkbcommon.h",
			wantFix: "xkbcommon-devel",
		},
		{
			name:    "simde",
			output:  "kitty/simd-string-impl.h:36:10: fatal error: simde/x86/avx2.h: No such file or directory",
			wantIn:  "simde/x86/avx2.h",
			wantFix: "simde-devel",
		},
		{
			// No directory component: the library name comes from the stem.
			name:    "bare header",
			output:  "foo.c:1:10: fatal error: zstd.h: No such file or directory",
			wantIn:  "zstd.h",
			wantFix: "zstd-devel",
		},
		{
			name:    "c++ header",
			output:  "a.cpp:3:10: fatal error: fmt/core.hpp: No such file or directory",
			wantIn:  "fmt/core.hpp",
			wantFix: "fmt-devel",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Diagnose(strings.Split(tt.output, "\n"))
			if len(got) == 0 {
				t.Fatalf("no diagnosis for %q", tt.output)
			}
			if !strings.Contains(got[0].Cause, tt.wantIn) {
				t.Errorf("cause = %q, want it to name %q", got[0].Cause, tt.wantIn)
			}
			if !strings.Contains(got[0].Fix, tt.wantFix) {
				t.Errorf("fix = %q, want it to suggest %q", got[0].Fix, tt.wantFix)
			}
		})
	}
}
