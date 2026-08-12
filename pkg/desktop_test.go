package pkg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withDataHome points the desktop directory at a temporary tree, so a test can
// never write into the developer's own application menu.
func withDataHome(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_DATA_HOME", filepath.Join(dir, ".local", "share"))
	return dir
}

// kittyLikeEntry is a shipped .desktop file of the shape the mechanism has to
// cope with: a bare program name in Exec and TryExec, an icon resolved through
// the theme, and an action group with an Exec of its own.
const kittyLikeEntry = `[Desktop Entry]
Version=1.0
Type=Application
Name=kitty
Name[bg]=кити
GenericName=Terminal emulator
TryExec=kitty
Exec=kitty --single-instance
Icon=kitty
Categories=System;TerminalEmulator;

[Desktop Action new-window]
Name=New window
Exec=kitty --new-window
`

func TestRewriteDesktopEntryPinsExecToTheInstalledBinary(t *testing.T) {
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "kitty"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := string(rewriteDesktopEntry([]byte(kittyLikeEntry), desktopRewrite{BinDir: binDir}))

	// The rewrite is the feature: without it the entry launches whichever kitty
	// PATH finds, which is the distribution's.
	for _, want := range []string{
		"Exec=" + filepath.Join(binDir, "kitty") + " --single-instance",
		"TryExec=" + filepath.Join(binDir, "kitty"),
		"Exec=" + filepath.Join(binDir, "kitty") + " --new-window",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("entry = %q\nwant it to contain %q", got, want)
		}
	}
	if strings.Contains(got, "Exec=kitty") || strings.Contains(got, "TryExec=kitty") {
		t.Errorf("entry = %q, want no bare program name left", got)
	}
}

// TestRewriteDesktopEntryLeavesAnUnknownProgramAlone covers an entry that
// launches something clipack did not install. Pointing it at a file that does
// not exist would turn a working entry into a broken one.
func TestRewriteDesktopEntryLeavesAnUnknownProgramAlone(t *testing.T) {
	got := string(rewriteDesktopEntry([]byte("[Desktop Entry]\nExec=somethingelse --flag\n"),
		desktopRewrite{BinDir: t.TempDir()}))

	if !strings.Contains(got, "Exec=somethingelse --flag") {
		t.Errorf("entry = %q, want the Exec left untouched", got)
	}
}

func TestRewriteDesktopEntrySuffixesEveryName(t *testing.T) {
	got := string(rewriteDesktopEntry([]byte(kittyLikeEntry), desktopRewrite{BinDir: t.TempDir()}))

	if !strings.Contains(got, "Name=kitty (clipack)") {
		t.Errorf("entry = %q, want the name distinguished from the system entry", got)
	}
	// A localised name left unsuffixed shows the two entries under one label on
	// any desktop running that locale.
	if !strings.Contains(got, "Name[bg]=кити (clipack)") {
		t.Errorf("entry = %q, want the localised name suffixed too", got)
	}
	// An action's Name labels the action, not the program.
	if !strings.Contains(got, "Name=New window\n") {
		t.Errorf("entry = %q, want the action name left alone", got)
	}
	if strings.Contains(got, "New window (clipack)") {
		t.Errorf("entry = %q, want no suffix on an action label", got)
	}
	// GenericName is not a name key.
	if !strings.Contains(got, "GenericName=Terminal emulator\n") {
		t.Errorf("entry = %q, want GenericName untouched", got)
	}
}

func TestRewriteDesktopEntryHonoursAnExplicitName(t *testing.T) {
	got := string(rewriteDesktopEntry([]byte(kittyLikeEntry),
		desktopRewrite{BinDir: t.TempDir(), Name: "kitty 0.48"}))

	if !strings.Contains(got, "Name=kitty 0.48") {
		t.Errorf("entry = %q, want the configured name", got)
	}
	if strings.Contains(got, "(clipack)") {
		t.Errorf("entry = %q, want no suffix when a name was given", got)
	}
}

func TestRewriteDesktopEntryRepointsTheIcon(t *testing.T) {
	got := string(rewriteDesktopEntry([]byte(kittyLikeEntry),
		desktopRewrite{BinDir: t.TempDir(), Icon: "/tmp/icons/kitty.png"}))

	if !strings.Contains(got, "Icon=/tmp/icons/kitty.png") {
		t.Errorf("entry = %q, want the icon repointed", got)
	}
}

// TestRewriteDesktopEntryQuotesAPathWithSpaces guards the one case where an
// unquoted rewrite silently changes the meaning: the launcher would read the
// tail of the directory name as the first argument.
func TestRewriteDesktopEntryQuotesAPathWithSpaces(t *testing.T) {
	binDir := filepath.Join(t.TempDir(), "my tools")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "kitty"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := string(rewriteDesktopEntry([]byte("[Desktop Entry]\nExec=kitty -1\n"),
		desktopRewrite{BinDir: binDir}))

	if !strings.Contains(got, `Exec="`+filepath.Join(binDir, "kitty")+`" -1`) {
		t.Errorf("entry = %q, want the path quoted", got)
	}
}

func TestSplitExecProgram(t *testing.T) {
	tests := []struct {
		in          string
		program     string
		rest        string
		description string
	}{
		{in: "kitty", program: "kitty", description: "no arguments"},
		{in: "kitty --single-instance", program: "kitty", rest: "--single-instance", description: "with arguments"},
		{in: `"/opt/my tools/kitty" -1`, program: "/opt/my tools/kitty", rest: "-1", description: "quoted path"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			program, rest := splitExecProgram(tt.in)
			if program != tt.program || rest != tt.rest {
				t.Errorf("splitExecProgram(%q) = (%q, %q), want (%q, %q)",
					tt.in, program, rest, tt.program, tt.rest)
			}
		})
	}
}

// TestDesktopPathsDoNotCollideWithTheSystemEntry is why the file is prefixed: a
// user file with the same basename as a system one replaces it in every
// launcher, and the point here is to have both.
func TestDesktopPathsDoNotCollideWithTheSystemEntry(t *testing.T) {
	withDataHome(t)

	entry, iconDir, err := desktopPaths("kitty", "linux-package/share/applications/kitty.desktop")
	if err != nil {
		t.Fatalf("desktopPaths() error = %v", err)
	}

	if base := filepath.Base(entry); base == "kitty.desktop" {
		t.Error("the entry has the same basename as the system one, which would override it")
	}
	if !strings.HasPrefix(filepath.Base(entry), desktopFilePrefix) {
		t.Errorf("entry = %q, want the clipack prefix so removal can recognise it", entry)
	}
	if !strings.HasSuffix(filepath.Dir(entry), filepath.Join(".local", "share", "applications")) {
		t.Errorf("entry = %q, want it under the user's application directory", entry)
	}
	if !strings.Contains(iconDir, filepath.Join("clipack", "icons", "kitty")) {
		t.Errorf("iconDir = %q, want a clipack-owned directory", iconDir)
	}
}

func TestDesktopPathsFollowXDGDataHome(t *testing.T) {
	withDataHome(t)
	custom := filepath.Join(t.TempDir(), "data")
	t.Setenv("XDG_DATA_HOME", custom)

	entry, _, err := desktopPaths("kitty", "kitty.desktop")
	if err != nil {
		t.Fatalf("desktopPaths() error = %v", err)
	}
	if want := filepath.Join(custom, "applications"); filepath.Dir(entry) != want {
		t.Errorf("entry directory = %q, want %q", filepath.Dir(entry), want)
	}
}

func TestDesktopPathsRejectWhatIsNotAnEntry(t *testing.T) {
	withDataHome(t)

	for _, source := range []string{"", "share/applications", "share/applications/kitty.png"} {
		if _, _, err := desktopPaths("kitty", source); err == nil {
			t.Errorf("desktopPaths(%q) error = nil, want it rejected", source)
		}
	}
}

// TestRewriteDesktopEntryCarriesTheEnvironment covers programs configured
// through variables their config.sh exports. A menu entry runs with the
// session's environment, so without this prefix yazi launched from the menu
// showed the built-in theme while every terminal showed the configured one.
func TestRewriteDesktopEntryCarriesTheEnvironment(t *testing.T) {
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "yazi"), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}

	entry := "[Desktop Entry]\nTryExec=yazi\nExec=yazi %f\n\n[Desktop Action new]\nExec=yazi --new\n"
	got := string(rewriteDesktopEntry([]byte(entry), desktopRewrite{
		BinDir: binDir,
		Env:    []string{"YAZI_CONFIG_HOME=/opt/clipack/configs/yazi"},
	}))

	bin := filepath.Join(binDir, "yazi")
	if !strings.Contains(got, "Exec=env YAZI_CONFIG_HOME=/opt/clipack/configs/yazi "+bin+" %f") {
		t.Errorf("entry = %q, want the env prefix on Exec", got)
	}
	// Actions launch the program too, so they need the same environment.
	if !strings.Contains(got, "Exec=env YAZI_CONFIG_HOME=/opt/clipack/configs/yazi "+bin+" --new") {
		t.Errorf("entry = %q, want the env prefix on the action's Exec", got)
	}
	// TryExec is a file-existence probe, not a command: an env prefix there
	// makes launchers hide the entry because "env" is not the program.
	if !strings.Contains(got, "TryExec="+bin+"\n") || strings.Contains(got, "TryExec=env") {
		t.Errorf("entry = %q, want TryExec left as the bare binary", got)
	}
}

func TestRenderDesktopEnvExpandsBaseAndSorts(t *testing.T) {
	got := renderDesktopEnv(map[string]string{
		"B_VAR": "plain",
		"A_VAR": "${base}/configs/yazi",
	}, "/home/x/clipack")

	want := []string{"A_VAR=/home/x/clipack/configs/yazi", "B_VAR=plain"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("renderDesktopEnv() = %v, want %v — sorted, with ${base} expanded", got, want)
	}
}

// TestATerminalEntryNamesItsTerminal is the fix for a menu entry that asked instead of answering.
//
// `Terminal=true` lets the launcher choose, and the launcher does not know which terminal the
// program was configured for. Measured 2026-08-12: yazi opened in GNOME Console, drew a palette
// its theme was not written against, and fell through to ueberzugpp for images — which is built
// without Wayland on that machine and aborts. Naming the terminal removes all three at once.
func TestATerminalEntryNamesItsTerminal(t *testing.T) {
	in := []byte("[Desktop Entry]\nName=Yazi\nTerminal=true\nTryExec=yazi\nExec=yazi %f\n")

	got := string(rewriteDesktopEntry(in, desktopRewrite{Terminal: "kitty -e"}))

	if !strings.Contains(got, "Exec=kitty -e yazi %f") {
		t.Errorf("Exec is not wrapped in the terminal:\n%s", got)
	}
	// Left at true the launcher opens a window for an Exec that already opens one.
	if !strings.Contains(got, "Terminal=false") {
		t.Errorf("Terminal was left asking for a window:\n%s", got)
	}
	// TryExec is stat'd, never run: wrapping it would make the entry look absent.
	if !strings.Contains(got, "TryExec=yazi") {
		t.Errorf("TryExec was rewritten:\n%s", got)
	}
}

// TestNoTerminalLeavesTheEntryAlone: the field is opt-in, and an entry that says nothing must come
// out exactly as it went in.
func TestNoTerminalLeavesTheEntryAlone(t *testing.T) {
	in := []byte("[Desktop Entry]\nName=Yazi\nTerminal=true\nExec=yazi %f\n")
	got := string(rewriteDesktopEntry(in, desktopRewrite{}))
	if !strings.Contains(got, "Terminal=true") || !strings.Contains(got, "Exec=yazi %f") {
		t.Errorf("an entry with no terminal named was changed:\n%s", got)
	}
}

// TestTheTerminalWrapsTheEnvPrefixToo: the variables belong to the program, not to the window.
func TestTheTerminalWrapsTheEnvPrefixToo(t *testing.T) {
	in := []byte("[Desktop Entry]\nTerminal=true\nExec=yazi %f\n")
	got := string(rewriteDesktopEntry(in, desktopRewrite{
		Terminal: "kitty -e",
		Env:      []string{"FOO=bar"},
	}))
	if !strings.Contains(got, "Exec=kitty -e env FOO=bar yazi %f") {
		t.Errorf("the terminal and the env prefix are in the wrong order:\n%s", got)
	}
}
