package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lvim-tech/clipack/cnfg"
	"github.com/lvim-tech/clipack/pkg"
)

func TestNewOpStreamDeliversEventsThenCloses(t *testing.T) {
	config := testConfig(t)

	stream := newOpStream(config, func(in *pkg.Installer) error {
		in.Report(pkg.Event{Kind: pkg.EventInfo, Text: "first"})
		in.Report(pkg.Event{Kind: pkg.EventDone, Text: "second"})
		return nil
	})

	var texts []string
	for event := range stream.events {
		texts = append(texts, event.Text)
	}

	if len(texts) != 2 || texts[0] != "first" || texts[1] != "second" {
		t.Errorf("events = %v, want [first second] in order", texts)
	}
	// The error is written before the channel closes, so a receiver that has
	// observed the close also observes the error.
	if *stream.err != nil {
		t.Errorf("stream error = %v, want nil", *stream.err)
	}
}

func TestNewOpStreamPropagatesTheError(t *testing.T) {
	want := errors.New("build failed")

	stream := newOpStream(testConfig(t), func(*pkg.Installer) error { return want })

	for range stream.events { //nolint:revive // drain
	}
	if !errors.Is(*stream.err, want) {
		t.Errorf("stream error = %v, want %v", *stream.err, want)
	}
}

func TestWaitForEventCmdBatches(t *testing.T) {
	stream := &opStream{events: make(chan pkg.Event, 16), err: new(error)}

	for i := 0; i < 5; i++ {
		stream.events <- pkg.Event{Kind: pkg.EventOutput, Text: "line"}
	}

	msg := waitForEventCmd(stream)()
	batch, ok := msg.(opEventsMsg)
	if !ok {
		t.Fatalf("got %T, want opEventsMsg", msg)
	}
	// Everything already buffered is drained in one go rather than one Bubble
	// Tea round trip per line.
	if len(batch.events) != 5 {
		t.Errorf("batch carries %d events, want all 5 that were buffered", len(batch.events))
	}
	if batch.finished {
		t.Error("finished = true although the stream is still open")
	}
}

func TestWaitForEventCmdRespectsTheBatchLimit(t *testing.T) {
	stream := &opStream{events: make(chan pkg.Event, maxEventBatch*2), err: new(error)}

	for i := 0; i < maxEventBatch*2; i++ {
		stream.events <- pkg.Event{Kind: pkg.EventOutput, Text: "line"}
	}

	batch := waitForEventCmd(stream)().(opEventsMsg)
	// The cap exists so the UI still gets a chance to redraw during a very
	// chatty build.
	if len(batch.events) != maxEventBatch {
		t.Errorf("batch carries %d events, want the cap of %d", len(batch.events), maxEventBatch)
	}
}

func TestWaitForEventCmdDeliversTheTailWithCompletion(t *testing.T) {
	failure := errors.New("boom")
	stream := &opStream{events: make(chan pkg.Event, 4), err: &failure}

	stream.events <- pkg.Event{Kind: pkg.EventOutput, Text: "last line"}
	close(stream.events)

	msg := waitForEventCmd(stream)()
	batch, ok := msg.(opEventsMsg)
	if !ok {
		t.Fatalf("got %T, want opEventsMsg carrying both the tail and the completion", msg)
	}
	if len(batch.events) != 1 || batch.events[0].Text != "last line" {
		t.Errorf("events = %v, want the buffered tail", batch.events)
	}
	if !batch.finished {
		t.Error("finished = false although the stream closed")
	}
	if !errors.Is(batch.err, failure) {
		t.Errorf("err = %v, want %v", batch.err, failure)
	}
}

func TestWaitForEventCmdOnAClosedStream(t *testing.T) {
	stream := &opStream{events: make(chan pkg.Event), err: new(error)}
	close(stream.events)

	if _, ok := waitForEventCmd(stream)().(opFinishedMsg); !ok {
		t.Error("a closed empty stream did not produce opFinishedMsg")
	}
}

func TestLoadRegistryCmdUsesTheCache(t *testing.T) {
	config := testConfig(t)

	if err := pkg.SaveToCache([]*pkg.Package{
		{Name: "zoxide", Version: "v0.9.7", Category: "cli"},
		{Name: "bat", Version: "v0.25.0", Category: "cli"},
	}, config); err != nil {
		t.Fatal(err)
	}

	// No server is listening, so this only succeeds by reading the cache.
	msg, ok := loadRegistryCmd(config, false)().(registryLoadedMsg)
	if !ok {
		t.Fatal("loadRegistryCmd did not return a registryLoadedMsg")
	}
	if msg.err != nil {
		t.Fatalf("err = %v, want the cache to be used", msg.err)
	}
	if len(msg.packages) != 2 {
		t.Fatalf("got %d packages, want 2", len(msg.packages))
	}
	// Packages are sorted by category then name for a stable display order.
	if msg.packages[0].Name != "bat" {
		t.Errorf("packages[0] = %q, want bat — the list should be sorted", msg.packages[0].Name)
	}
	if msg.refreshed {
		t.Error("refreshed = true for a cached load")
	}
}

func TestLoadRegistryCmdReportsFailure(t *testing.T) {
	config := testConfig(t)
	// A registry URL that cannot resolve, and no cache to fall back on.
	config.Registry.URL = "https://github.com/definitely/not-a-real-registry-xyz.git"

	msg := loadRegistryCmd(config, false)().(registryLoadedMsg)
	if msg.err == nil {
		t.Error("err = nil, want the failure reported so the UI can offer a retry")
	}
	if len(msg.packages) != 0 {
		t.Errorf("got %d packages, want none", len(msg.packages))
	}
}

func TestLoadRegistryCmdMarksInstalledPackages(t *testing.T) {
	config := testConfig(t)

	if err := pkg.SaveToCache([]*pkg.Package{{Name: "bat", Version: "v0.25.0"}}, config); err != nil {
		t.Fatal(err)
	}

	manifestDir := filepath.Join(config.Paths.Configs, "bat")
	if err := os.MkdirAll(manifestDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "name: bat\nversion: v0.20.0\ninstall_method: version\n"
	if err := os.WriteFile(filepath.Join(manifestDir, "package.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	msg := loadRegistryCmd(config, false)().(registryLoadedMsg)
	if msg.installed["bat"] == nil {
		t.Fatal("the installed package was not picked up")
	}
	if msg.installed["bat"].Version != "v0.20.0" {
		t.Errorf("installed version = %q, want the manifest's v0.20.0", msg.installed["bat"].Version)
	}
}

func TestSaveConfigCmd(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	installDir := filepath.Join(t.TempDir(), "packages")

	const registryURL = "https://github.com/owner/repo.git"

	msg, ok := saveConfigCmd(installDir, registryURL, false)().(setupDoneMsg)
	if !ok {
		t.Fatal("saveConfigCmd did not return a setupDoneMsg")
	}
	if msg.err != nil {
		t.Fatalf("err = %v", msg.err)
	}
	// The wizard's second answer has to reach the file: clipack has no default
	// registry to fall back on, so losing it writes a config that cannot load.
	if msg.config != nil && msg.config.Registry.URL != registryURL {
		t.Errorf("Registry.URL = %q, want %q", msg.config.Registry.URL, registryURL)
	}
	if msg.config == nil {
		t.Fatal("no config was returned")
	}
	if msg.config.Paths.Base != installDir {
		t.Errorf("Base = %q, want %q", msg.config.Paths.Base, installDir)
	}

	// The file has to be on disk, not just in memory.
	if !cnfg.Exists() {
		t.Error("saveConfigCmd did not write config.yaml")
	}
	for _, dir := range msg.config.Dirs() {
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("%s was not created", dir)
		}
	}
}

func TestSaveConfigCmdReportsFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	// A file where the config directory needs to be, so MkdirAll fails.
	if err := os.MkdirAll(filepath.Join(home, ".config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".config", "clipack"), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}

	msg := saveConfigCmd(filepath.Join(t.TempDir(), "packages"), "https://github.com/owner/repo.git", false)().(setupDoneMsg)
	if msg.err == nil {
		t.Error("err = nil, want the write failure reported to the wizard")
	}
	if msg.config != nil {
		t.Error("a config was returned despite the failure")
	}
}

func TestSaveConfigCmdShellFailureIsNotFatal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// An unsupported shell makes the rc update fail; the configuration itself
	// is still usable, so setup must not be blocked by it.
	t.Setenv(cnfg.ShellOverrideEnv, "/bin/nonexistent-shell")

	msg := saveConfigCmd(filepath.Join(t.TempDir(), "packages"), "https://github.com/owner/repo.git", true)().(setupDoneMsg)
	if msg.err != nil {
		t.Errorf("err = %v, want the shell failure to be tolerated", msg.err)
	}
	if msg.config == nil {
		t.Error("no config was returned")
	}
}

func TestOpStreamDrivesTheInstaller(t *testing.T) {
	config := testConfig(t)

	// A package that "builds" from shell built-ins, so the whole path from the
	// installer through the stream to the UI messages is exercised.
	p := &pkg.Package{
		Name:    "demo",
		Version: "v1.0.0",
		Install: pkg.Install{
			Steps:    []string{"mkdir -p out", "printf x > out/demo"},
			Binaries: []string{"out/demo"},
		},
	}

	stream := newOpStream(config, func(in *pkg.Installer) error {
		return in.Install(p, pkg.MethodVersion)
	})

	var steps, done int
	for event := range stream.events {
		switch event.Kind {
		case pkg.EventStep:
			steps++
		case pkg.EventDone:
			done++
		}
	}

	if *stream.err != nil {
		t.Fatalf("install error = %v", *stream.err)
	}
	if steps != 2 {
		t.Errorf("got %d step events, want 2", steps)
	}
	if done != 1 {
		t.Errorf("got %d done events, want 1", done)
	}
	if _, err := os.Stat(filepath.Join(config.Paths.Bin, "demo")); err != nil {
		t.Errorf("the binary was not installed: %v", err)
	}
}

func TestFormatEventKeepsTheTextIntact(t *testing.T) {
	// Whatever decoration is applied, the underlying text has to survive so
	// the log stays readable and greppable.
	const text = "error[E0432]: unresolved import `foo::bar`"

	for _, kind := range []pkg.EventKind{
		pkg.EventInfo, pkg.EventStep, pkg.EventOutput,
		pkg.EventWarn, pkg.EventError, pkg.EventDone,
	} {
		got := formatEvent(pkg.Event{Kind: kind, Text: text}, DefaultStyles())
		if !strings.Contains(got, text) {
			t.Errorf("formatEvent(kind=%v) = %q, want it to contain the original text", kind, got)
		}
	}
}
