package tui

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lvim-tech/clipack/cnfg"
	"github.com/lvim-tech/clipack/pkg"
)

// testConfig builds a configuration rooted at a temporary directory.
func testConfig(t *testing.T) *cnfg.Config {
	t.Helper()

	base := t.TempDir()
	config := &cnfg.Config{
		Registry: cnfg.RegistryConfig{
			URL:            "https://github.com/example/registry.git",
			Branch:         "main",
			UpdateInterval: time.Hour,
		},
		Paths: cnfg.PathsConfig{
			Base:     base,
			Registry: filepath.Join(base, "registry"),
			Bin:      filepath.Join(base, "bin"),
			Configs:  filepath.Join(base, "configs"),
			Build:    filepath.Join(base, "build"),
			Man:      filepath.Join(base, "man"),
			Expose:   filepath.Join(base, "local", "bin"),
		},
		Options: cnfg.OptionsConfig{InstallMethod: pkg.MethodVersion},
	}
	if err := config.EnsureDirs(); err != nil {
		t.Fatalf("creating test directories: %v", err)
	}
	return config
}

// samplePackages is a three-package registry: one not installed, one installed
// and current, one installed and out of date.
func samplePackages() ([]*pkg.Package, map[string]*pkg.Package) {
	packages := []*pkg.Package{
		{
			Name: "bat", Version: "v0.25.0", Commit: "aaaa1111",
			Description: "A cat clone with syntax highlighting",
			Category:    "cli", Tags: []string{"cat", "syntax"},
			Maintainer: "sharkdp", License: "MIT",
			Install: pkg.Install{
				Steps:    []string{"git clone https://example.com/bat.git .", "cargo build"},
				Binaries: []string{"target/release/bat"},
			},
		},
		{
			Name: "fzf", Version: "v0.62.0", Commit: "bbbb2222",
			Description: "A command-line fuzzy finder",
			Category:    "cli", Tags: []string{"finder"},
		},
		{
			Name: "yazi", Version: "v25.4.8", Commit: "cccc3333",
			Description: "A terminal file manager",
			Category:    "file_managers",
		},
	}

	installed := map[string]*pkg.Package{
		// Current.
		"fzf": {Name: "fzf", Version: "v0.62.0", Commit: "bbbb2222", InstallMethod: pkg.MethodVersion},
		// Out of date.
		"yazi": {Name: "yazi", Version: "v25.0.0", Commit: "old00000", InstallMethod: pkg.MethodVersion},
	}

	return packages, installed
}

// browseModel returns a model sitting on the browse screen with the sample
// registry loaded and a usable window size.
func browseModel(t *testing.T) Model {
	t.Helper()

	m := New(testConfig(t))
	m = applyMsg(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})

	packages, installed := samplePackages()
	m = applyMsg(t, m, registryLoadedMsg{packages: packages, installed: installed})

	if m.screen != screenBrowse {
		t.Fatalf("screen = %v after loading the registry, want screenBrowse", m.screen)
	}
	return m
}

// samplePackagesFirst returns the first sample package, for tests that only
// need one.
func samplePackagesFirst() *pkg.Package {
	packages, _ := samplePackages()
	return packages[0]
}

// errorEvent is an installer failure event, used to check glyph selection.
func errorEvent() pkg.Event {
	return pkg.Event{Kind: pkg.EventError, Text: "build failed"}
}

// applyMsg runs one Update and returns the concrete model.
func applyMsg(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()

	next, _ := m.Update(msg)
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update() returned %T, want tui.Model", next)
	}
	drainOpOnCleanup(t, got)
	return got
}

// drained records the streams already waited on, so repeated Updates on a model
// that is running an operation register one cleanup rather than dozens.
var drained sync.Map

// drainOpOnCleanup makes the test wait for any operation the model just started
// before it returns.
//
// Confirming an action spawns a real installer goroutine that builds under the
// configured directories, and those live in t.TempDir(). Without this the test
// finishes while the goroutine is still writing, and the cleanup fails with
// "directory not empty" — intermittently, on whichever test happened to lose
// the race. The stream closes its event channel when the operation ends, so
// draining it to close is the wait.
func drainOpOnCleanup(t *testing.T, m Model) {
	t.Helper()

	if m.stream == nil {
		return
	}
	if _, seen := drained.LoadOrStore(m.stream, true); seen {
		return
	}
	t.Cleanup(func() {
		for range m.stream.events { //nolint:revive // draining to close is the point
		}
	})
}

// applyMsgCmd runs one Update and returns both the model and the command.
func applyMsgCmd(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()

	next, cmd := m.Update(msg)
	got, ok := next.(Model)
	if !ok {
		t.Fatalf("Update() returned %T, want tui.Model", next)
	}
	drainOpOnCleanup(t, got)
	return got, cmd
}

// namedKeys maps a key name onto the message type Bubble Tea actually delivers
// for it. Anything not listed here is literal text.
//
// This matters more than it looks: a named key sent as runes would arrive as a
// multi-rune message, and code that switches on single characters would then
// see "l", "e", "f", "t" instead of one left-arrow.
var namedKeys = map[string]tea.KeyType{
	"tab":       tea.KeyTab,
	"shift+tab": tea.KeyShiftTab,
	"enter":     tea.KeyEnter,
	"esc":       tea.KeyEsc,
	"ctrl+c":    tea.KeyCtrlC,
	"ctrl+d":    tea.KeyCtrlD,
	"ctrl+u":    tea.KeyCtrlU,
	" ":         tea.KeySpace,
	"up":        tea.KeyUp,
	"down":      tea.KeyDown,
	"left":      tea.KeyLeft,
	"right":     tea.KeyRight,
	"home":      tea.KeyHome,
	"end":       tea.KeyEnd,
	"pgup":      tea.KeyPgUp,
	"pgdown":    tea.KeyPgDown,
}

// keyMsg builds the tea.KeyMsg for a key name as the key bindings spell it.
func keyMsg(name string) tea.KeyMsg {
	if t, ok := namedKeys[name]; ok {
		return tea.KeyMsg{Type: t}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)}
}

// tea_KeyRunes builds the single multi-rune message Bubble Tea produces when
// several keypresses arrive in one read.
func tea_KeyRunes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// selectPackage moves the cursor onto the named package.
func selectPackage(t *testing.T, m Model, name string) Model {
	t.Helper()

	for i, item := range m.list.Items() {
		entry, ok := item.(packageItem)
		if ok && entry.pkg.Name == name {
			m.list.Select(i)
			m.invalidateDetail()
			m.refreshDetail()
			return m
		}
	}
	t.Fatalf("package %q is not in the current list", name)
	return m
}
