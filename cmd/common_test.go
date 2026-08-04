package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lvim-tech/clipack/cnfg"
	"github.com/lvim-tech/clipack/pkg"
)

func TestLoadConfigLoadsAnExistingConfiguration(t *testing.T) {
	want := setupCmdTest(t)

	got, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}
	if got.Paths.Base != want.Paths.Base {
		t.Errorf("Base = %q, want %q", got.Paths.Base, want.Paths.Base)
	}
}

func TestLoadConfigBootstrapsOnFirstRun(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(os.Getenv("HOME"), ".config"))
	t.Setenv(cnfg.ShellOverrideEnv, "/bin/bash")

	installDir := filepath.Join(t.TempDir(), "packages")
	// The trailing "n" declines the shell-configuration prompt.
	withStdin(t, installDir+"\nn\n")

	var err error
	capture(t, func() { _, err = loadConfig() })

	// The file is written — every subcommand starts here, so the tree has to
	// exist whichever one the user reached for first — but clipack ships no
	// registry, so the run stops with instructions rather than reaching for
	// somebody else's.
	if err == nil {
		t.Fatal("loadConfig() error = nil, want it to ask for a registry")
	}
	for _, want := range []string{"registry", "url:"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q:\n%v", want, err)
		}
	}
	if !cnfg.Exists() {
		t.Error("loadConfig() did not write config.yaml")
	}
	if _, statErr := os.Stat(installDir); statErr != nil {
		t.Errorf("the installation directory was not created: %v", statErr)
	}
}

func TestLoadConfigRecreatesMissingDirectories(t *testing.T) {
	config := setupCmdTest(t)

	// The user deleted the tree but kept config.yaml.
	if err := os.RemoveAll(config.Paths.Base); err != nil {
		t.Fatal(err)
	}

	if _, err := loadConfig(); err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}
	for _, dir := range config.Dirs() {
		if !exists(dir) {
			t.Errorf("%s was not recreated", dir)
		}
	}
}

func TestLoadConfigRejectsAnInvalidFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(os.Getenv("HOME"), ".config"))

	dir, err := cnfg.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A config with no registry URL and relative paths.
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("paths:\n  base: relative\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := loadConfig(); err == nil {
		t.Error("loadConfig() error = nil, want the invalid configuration rejected")
	}
}

func TestLoadPackagesUsesTheCache(t *testing.T) {
	config := setupCmdTest(t)
	seedCache(t, config, demoPackage(), otherPackage())

	packages, err := loadPackages(config, false)
	if err != nil {
		t.Fatalf("loadPackages() error = %v", err)
	}
	if len(packages) != 2 {
		t.Fatalf("got %d packages, want 2", len(packages))
	}
	if packages[0].Name != "demo" {
		t.Errorf("packages[0] = %q, want demo", packages[0].Name)
	}
}

func TestLoadPackagesForceRefreshClearsTheCache(t *testing.T) {
	config := setupCmdTest(t)
	// A registry that cannot be reached, so the refresh must fail rather than
	// quietly serving the stale cache it was told to discard.
	config.Registry.URL = "https://github.com/definitely/not-a-real-registry-xyz.git"
	seedCache(t, config, demoPackage())

	var err error
	capture(t, func() { _, err = loadPackages(config, true) })

	if err == nil {
		t.Fatal("loadPackages(force) error = nil, want the fetch failure reported")
	}
	if exists(pkg.GetCacheFilePath(config)) {
		t.Error("the cache survived a forced refresh")
	}
}

func TestLoadPackagesReportsAMissingRegistry(t *testing.T) {
	config := setupCmdTest(t)
	config.Registry.URL = "https://github.com/definitely/not-a-real-registry-xyz.git"

	var err error
	capture(t, func() { _, err = loadPackages(config, false) })

	if err == nil {
		t.Error("loadPackages() error = nil, want an error when there is no cache and no network")
	}
}

func TestCliReporter(t *testing.T) {
	tests := []struct {
		name       string
		event      pkg.Event
		wantStdout string
		wantStderr string
	}{
		{
			name:       "step carries a counter",
			event:      pkg.Event{Kind: pkg.EventStep, Step: 2, Total: 4, Text: "cargo build"},
			wantStdout: "▶ [2/4] cargo build",
		},
		{
			name:       "output is indented",
			event:      pkg.Event{Kind: pkg.EventOutput, Text: "Compiling"},
			wantStdout: "  │ Compiling",
		},
		{
			name:       "done goes to stdout",
			event:      pkg.Event{Kind: pkg.EventDone, Text: "installed demo"},
			wantStdout: "✓ installed demo",
		},
		{
			name:       "info goes to stdout",
			event:      pkg.Event{Kind: pkg.EventInfo, Text: "Removing demo"},
			wantStdout: "Removing demo",
		},
		{
			// Diagnostics belong on stderr so "clipack list > file" stays clean.
			name:       "warning goes to stderr",
			event:      pkg.Event{Kind: pkg.EventWarn, Text: "man page missing"},
			wantStderr: "! man page missing",
		},
		{
			name:       "error goes to stderr",
			event:      pkg.Event{Kind: pkg.EventError, Text: "build failed"},
			wantStderr: "✗ build failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr := capture(t, func() { cliReporter(tt.event) })

			if tt.wantStdout != "" && !strings.Contains(stdout, tt.wantStdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout, tt.wantStdout)
			}
			if tt.wantStderr != "" {
				if !strings.Contains(stderr, tt.wantStderr) {
					t.Errorf("stderr = %q, want it to contain %q", stderr, tt.wantStderr)
				}
				if strings.Contains(stdout, tt.event.Text) {
					t.Errorf("stdout = %q, want the diagnostic on stderr only", stdout)
				}
			}
		})
	}
}

func TestNewInstallerReportsThroughTheCLI(t *testing.T) {
	config := setupCmdTest(t)

	installer := newInstaller(config)
	if installer.Config != config {
		t.Error("newInstaller() did not carry the configuration through")
	}

	stdout, _ := capture(t, func() {
		installer.Report(pkg.Event{Kind: pkg.EventDone, Text: "all good"})
	})
	if !strings.Contains(stdout, "all good") {
		t.Errorf("stdout = %q, want the event printed", stdout)
	}
}

func TestAskYes(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"y\n", true},
		{"yes\n", true},
		{"n\n", false},
		{"\n", false},
		{"", false},
	}

	for _, tt := range tests {
		withStdin(t, tt.input)

		var got bool
		capture(t, func() { got = askYes("proceed?") })

		if got != tt.want {
			t.Errorf("askYes(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestFirstLine(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"single line", "single line"},
		{"first\nsecond", "first"},
		{"first\n", "first"},
		{"", ""},
		{"\nleading newline", ""},
	}

	for _, tt := range tests {
		if got := firstLine(tt.in); got != tt.want {
			t.Errorf("firstLine(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
