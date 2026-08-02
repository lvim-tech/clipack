package cnfg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCurrentShellRejectsWhatItCannotWrite(t *testing.T) {
	tests := []struct {
		name     string
		override string
		shell    string
	}{
		{name: "nothing to go on", override: "", shell: ""},
		{name: "blank", override: "   ", shell: "   "},
		{name: "unknown override", override: "/usr/bin/shellwedonotknow"},
		{name: "unknown login shell", override: "", shell: "/usr/bin/shellwedonotknow"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(ShellOverrideEnv, tt.override)
			t.Setenv("SHELL", tt.shell)

			if _, err := CurrentShell(); err == nil {
				t.Errorf("CurrentShell() error = nil, want an error for override %q and $SHELL %q",
					tt.override, tt.shell)
			}
		})
	}
}

// TestCurrentShellFallsBackToTheLoginShell covers the last link in the chain.
// The test binary's parent is `go`, which is not a shell, so this exercises the
// path taken when clipack cannot see a shell that launched it.
func TestCurrentShellFallsBackToTheLoginShell(t *testing.T) {
	t.Setenv(ShellOverrideEnv, "")
	t.Setenv("SHELL", "/bin/tcsh")

	sh, err := CurrentShell()
	if err != nil {
		t.Fatalf("CurrentShell() error = %v", err)
	}
	if sh.Name != "tcsh" {
		t.Errorf("CurrentShell() = %q, want tcsh from $SHELL", sh.Name)
	}
}

// TestParentShellIgnoresANonShellLauncher pins the reason parentShell does not
// walk the tree. `go test` runs this binary directly, so the parent is go — and
// a version that kept climbing would find the developer's own shell above it
// and report a shell nobody typed into.
func TestParentShellIgnoresANonShellLauncher(t *testing.T) {
	if name, err := procName(os.Getpid()); err != nil || name == "" {
		t.Skipf("no /proc on this system: %v", err)
	}

	if got := parentShell(); got != "" {
		t.Errorf("parentShell() = %q, want %q: the launcher is go, not a shell", got, "")
	}
}

// TestCurrentShellAcceptsALoginName covers $SHELL carrying the leading dash a
// login shell puts in argv[0]. Without the TrimPrefix in CurrentShell the name
// reads as "-bash" and matches nothing.
func TestCurrentShellAcceptsALoginName(t *testing.T) {
	t.Setenv(ShellOverrideEnv, "/bin/-bash")

	sh, err := CurrentShell()
	if err != nil {
		t.Fatalf("CurrentShell() error = %v", err)
	}
	if sh.Name != "bash" {
		t.Errorf("CurrentShell() = %q, want bash", sh.Name)
	}
}

func TestShellExportLines(t *testing.T) {
	tests := []struct {
		shell string
		want  []string
	}{
		{shell: "/bin/bash", want: []string{`export PATH="/bin:$PATH"`, `export MANPATH="/man:$MANPATH"`}},
		{shell: "/usr/bin/zsh", want: []string{`export PATH="/bin:$PATH"`}},
		{shell: "/bin/ksh", want: []string{`export PATH="/bin:$PATH"`}},
		{shell: "/bin/mksh", want: []string{`export PATH="/bin:$PATH"`}},
		{shell: "/bin/dash", want: []string{`export PATH="/bin:$PATH"`}},
		{shell: "/usr/bin/fish", want: []string{"set -x PATH /bin $PATH", "set -x MANPATH /man $MANPATH"}},
		{shell: "/bin/tcsh", want: []string{`setenv PATH "/bin:$PATH"`, "if ( $?MANPATH ) then"}},
		{shell: "/bin/csh", want: []string{`setenv PATH "/bin:$PATH"`}},
		{shell: "/usr/bin/nu", want: []string{"$env.PATH = ($env.PATH | prepend '/bin')"}},
		{shell: "/usr/bin/elvish", want: []string{"set paths = ['/bin' $@paths]"}},
		{shell: "/usr/bin/xonsh", want: []string{"$PATH.insert(0, '/bin')"}},
		{shell: "/usr/bin/pwsh", want: []string{`$env:PATH = "/bin" + [IO.Path]::PathSeparator`}},
	}

	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			t.Setenv(ShellOverrideEnv, tt.shell)

			got, err := ShellExportLines("/bin", "/man", "")
			if err != nil {
				t.Fatalf("ShellExportLines() error = %v", err)
			}
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("output = %q, want it to contain %q", got, want)
				}
			}
		})
	}
}

// TestCshExportsGuardManpath pins the reason cshExports is not a two-line
// assignment: in csh a reference to an unset variable aborts the file, so an
// unguarded $MANPATH would take the PATH line down with it.
func TestCshExportsGuardManpath(t *testing.T) {
	got := cshExports("/bin", "/man")

	for _, want := range []string{"if ( $?MANPATH ) then", `setenv MANPATH "/man:$MANPATH"`, `setenv MANPATH "/man"`, "endif"} {
		if !strings.Contains(got, want) {
			t.Errorf("cshExports() = %q, want it to contain %q", got, want)
		}
	}
}

func TestGetShellConfigFilePath(t *testing.T) {
	tests := []struct {
		shell    string
		wantBase string
	}{
		{shell: "/bin/bash", wantBase: ".bashrc"},
		{shell: "/usr/bin/zsh", wantBase: ".zshrc"},
		{shell: "/usr/bin/fish", wantBase: "config.fish"},
		{shell: "/bin/ksh", wantBase: ".kshrc"},
		{shell: "/bin/mksh", wantBase: ".mkshrc"},
		{shell: "/bin/dash", wantBase: ".profile"},
		{shell: "/bin/sh", wantBase: ".profile"},
		{shell: "/bin/tcsh", wantBase: ".tcshrc"},
		{shell: "/bin/csh", wantBase: ".cshrc"},
		{shell: "/usr/bin/nu", wantBase: "env.nu"},
		{shell: "/usr/bin/elvish", wantBase: "rc.elv"},
		{shell: "/usr/bin/xonsh", wantBase: ".xonshrc"},
	}

	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			withHome(t)
			t.Setenv(ShellOverrideEnv, tt.shell)

			got, err := GetShellConfigFilePath()
			if err != nil {
				t.Fatalf("GetShellConfigFilePath() error = %v", err)
			}
			if filepath.Base(got) != tt.wantBase {
				t.Errorf("GetShellConfigFilePath() = %q, want it to end in %q", got, tt.wantBase)
			}
		})
	}
}

// TestRCFileFollowsXDGConfigHome covers the shells that keep their files below
// $XDG_CONFIG_HOME rather than a fixed ~/.config. Resolving those against $HOME
// writes to a file the shell never reads.
func TestRCFileFollowsXDGConfigHome(t *testing.T) {
	home := withHome(t)
	xdg := filepath.Join(home, "elsewhere")
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv(ShellOverrideEnv, "/usr/bin/fish")

	got, err := GetShellConfigFilePath()
	if err != nil {
		t.Fatalf("GetShellConfigFilePath() error = %v", err)
	}
	if want := filepath.Join(xdg, "fish", "config.fish"); got != want {
		t.Errorf("GetShellConfigFilePath() = %q, want %q", got, want)
	}
}

func TestCurrentShellStatus(t *testing.T) {
	home := withHome(t)
	t.Setenv(ShellOverrideEnv, "/bin/bash")
	binPath := filepath.Join(home, "clipack", "bin")

	t.Run("needs the paths when nothing references them", func(t *testing.T) {
		t.Setenv("PATH", "/usr/bin:/bin")

		status, err := CurrentShellStatus(binPath)
		if err != nil {
			t.Fatalf("CurrentShellStatus() error = %v", err)
		}
		if !status.NeedsPaths() {
			t.Errorf("NeedsPaths() = false, want true when neither PATH nor the rc file mention %q", binPath)
		}
		if status.NeedsRestart() {
			t.Error("NeedsRestart() = true, want false when the rc file says nothing")
		}
	})

	t.Run("is satisfied when the directory is already on PATH", func(t *testing.T) {
		t.Setenv("PATH", strings.Join([]string{"/usr/bin", binPath}, string(os.PathListSeparator)))

		status, err := CurrentShellStatus(binPath)
		if err != nil {
			t.Fatalf("CurrentShellStatus() error = %v", err)
		}
		if status.NeedsPaths() {
			t.Error("NeedsPaths() = true, want false when the directory is on PATH")
		}
	})

	t.Run("asks for a restart when only the rc file has it", func(t *testing.T) {
		t.Setenv("PATH", "/usr/bin:/bin")
		if _, err := AddPathsToShell(binPath, filepath.Join(home, "clipack", "man"), ""); err != nil {
			t.Fatalf("AddPathsToShell() error = %v", err)
		}

		status, err := CurrentShellStatus(binPath)
		if err != nil {
			t.Fatalf("CurrentShellStatus() error = %v", err)
		}
		if status.NeedsPaths() {
			t.Error("NeedsPaths() = true, want false once the rc file references the directory")
		}
		if !status.NeedsRestart() {
			t.Error("NeedsRestart() = false, want true while this session still has the old PATH")
		}
	})
}

// TestOnPathIgnoresATrailingSlash guards the comparison itself: PATH entries are
// written by hand and "~/clipack/bin/" is the same directory as "~/clipack/bin".
func TestOnPathIgnoresATrailingSlash(t *testing.T) {
	t.Setenv("PATH", "/opt/clipack/bin/")

	if !onPath("/opt/clipack/bin") {
		t.Error("onPath() = false, want true for the same directory written with a trailing slash")
	}
	if onPath("/opt/clipack") {
		t.Error("onPath() = true, want false for a parent of a listed directory")
	}
}

func TestAddPathsToShellIsIdempotent(t *testing.T) {
	home := withHome(t)
	t.Setenv(ShellOverrideEnv, "/bin/bash")
	t.Setenv("PATH", "/usr/bin:/bin")

	binPath := filepath.Join(home, "clipack", "bin")
	manPath := filepath.Join(home, "clipack", "man")

	status, err := AddPathsToShell(binPath, manPath, "")
	if err != nil {
		t.Fatalf("AddPathsToShell() error = %v", err)
	}
	// The rc path has to follow $HOME, the same home ConfigDir uses.
	if !strings.HasPrefix(status.RCFile, home) {
		t.Fatalf("rc path %q is outside the test home %q", status.RCFile, home)
	}

	first, err := os.ReadFile(status.RCFile)
	if err != nil {
		t.Fatalf("the rc file was not written: %v", err)
	}
	if !strings.Contains(string(first), binPath) {
		t.Fatalf("rc file = %q, want it to reference the bin path", first)
	}

	// Running it a second time must not stack a duplicate export block.
	if _, err := AddPathsToShell(binPath, manPath, ""); err != nil {
		t.Fatalf("second AddPathsToShell() error = %v", err)
	}

	second, err := os.ReadFile(status.RCFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Errorf("the rc file grew on the second call:\nfirst:  %q\nsecond: %q", first, second)
	}
	if n := strings.Count(string(second), binPath); n != 1 {
		t.Errorf("the bin path appears %d times, want exactly 1", n)
	}
}

// TestAddPathsToShellWritesOnlyTheCurrentShell is the whole point of keying on
// $SHELL: extending one shell must leave every other shell's file alone, so a
// user who moves between shells sees the offer again in each.
func TestAddPathsToShellWritesOnlyTheCurrentShell(t *testing.T) {
	home := withHome(t)
	t.Setenv("PATH", "/usr/bin:/bin")

	binPath := filepath.Join(home, "clipack", "bin")
	manPath := filepath.Join(home, "clipack", "man")

	t.Setenv(ShellOverrideEnv, "/usr/bin/zsh")
	if _, err := AddPathsToShell(binPath, manPath, ""); err != nil {
		t.Fatalf("AddPathsToShell() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(home, ".bashrc")); !os.IsNotExist(err) {
		t.Errorf("stat .bashrc = %v, want it never created while $SHELL is zsh", err)
	}

	// Now the same user, from bash: the offer stands again, for bash's file.
	t.Setenv(ShellOverrideEnv, "/bin/bash")
	status, err := CurrentShellStatus(binPath)
	if err != nil {
		t.Fatalf("CurrentShellStatus() error = %v", err)
	}
	if !status.NeedsPaths() {
		t.Error("NeedsPaths() = false in bash, want true: .bashrc has not been extended")
	}

	if _, err := AddPathsToShell(binPath, manPath, ""); err != nil {
		t.Fatalf("AddPathsToShell() from bash error = %v", err)
	}
	contents, err := os.ReadFile(filepath.Join(home, ".bashrc"))
	if err != nil {
		t.Fatalf(".bashrc was not written: %v", err)
	}
	if !strings.Contains(string(contents), binPath) {
		t.Errorf(".bashrc = %q, want it to reference the bin path", contents)
	}
}

func TestAddPathsToShellCreatesNestedDirectories(t *testing.T) {
	home := withHome(t)
	t.Setenv(ShellOverrideEnv, "/usr/bin/fish")
	t.Setenv("PATH", "/usr/bin:/bin")

	binPath := filepath.Join(home, "clipack", "bin")
	manPath := filepath.Join(home, "clipack", "man")

	// config.fish sits two directories deep, neither of which exists yet.
	status, err := AddPathsToShell(binPath, manPath, "")
	if err != nil {
		t.Fatalf("AddPathsToShell() error = %v", err)
	}

	contents, err := os.ReadFile(status.RCFile)
	if err != nil {
		t.Fatalf("config.fish was not created: %v", err)
	}
	if !strings.Contains(string(contents), "set -x PATH "+binPath) {
		t.Errorf("config.fish = %q, want fish syntax", contents)
	}
}

func TestAddPathsToShellConfigUnsupportedShell(t *testing.T) {
	withHome(t)
	t.Setenv(ShellOverrideEnv, "/usr/bin/shellwedonotknow")

	if err := AddPathsToShellConfig("/bin", "/man", ""); err == nil {
		t.Error("AddPathsToShellConfig() error = nil, want an error for an unsupported shell")
	}
}

func TestAddPathsToShellAppendsToAnExistingFile(t *testing.T) {
	home := withHome(t)
	t.Setenv(ShellOverrideEnv, "/bin/bash")
	t.Setenv("PATH", "/usr/bin:/bin")

	rc := filepath.Join(home, ".bashrc")
	if err := os.WriteFile(rc, []byte("# existing user configuration\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := AddPathsToShell(filepath.Join(home, "bin"), filepath.Join(home, "man"), ""); err != nil {
		t.Fatalf("AddPathsToShell() error = %v", err)
	}

	contents, err := os.ReadFile(rc)
	if err != nil {
		t.Fatal(err)
	}
	// The user's own configuration must be preserved, not overwritten.
	if !strings.Contains(string(contents), "# existing user configuration") {
		t.Errorf("rc file = %q, want the existing contents kept", contents)
	}
	if !strings.Contains(string(contents), "export PATH=") {
		t.Errorf("rc file = %q, want the export appended", contents)
	}
}

// TestExportLinesOffersTheAggregateOnlyToZsh pins why sourcesIntegration is a
// per-shell flag: the integrations the registry ships are zsh syntax, and a
// bash that sources them does not degrade — it breaks.
func TestExportLinesOffersTheAggregateOnlyToZsh(t *testing.T) {
	integration := "/opt/clipack/configs/clipack.sh"
	sourceLine := `. "` + integration + `"`

	tests := []struct {
		shell string
		want  bool
	}{
		{shell: "/usr/bin/zsh", want: true},
		{shell: "/bin/bash", want: false},
		{shell: "/usr/bin/fish", want: false},
		{shell: "/bin/tcsh", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			t.Setenv(ShellOverrideEnv, tt.shell)

			got, err := ShellExportLines("/bin", "/man", integration)
			if err != nil {
				t.Fatalf("ShellExportLines() error = %v", err)
			}
			if strings.Contains(got, sourceLine) != tt.want {
				t.Errorf("output = %q, want source line present = %v", got, tt.want)
			}
		})
	}

	// No integration path means no line, even for zsh.
	t.Setenv(ShellOverrideEnv, "/usr/bin/zsh")
	got, err := ShellExportLines("/bin", "/man", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "clipack.sh") {
		t.Errorf("output = %q, want no source line without an aggregate path", got)
	}
}
