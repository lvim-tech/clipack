package cnfg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// withHome points os.UserHomeDir at a temporary directory for the duration of a
// test, so nothing touches the real ~/.config/clipack.
func withHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

// withStdin replaces os.Stdin with a file holding the given input, so the
// prompting functions can be exercised without a terminal.
func withStdin(t *testing.T, input string) {
	t.Helper()

	f, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(input); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}

	old := os.Stdin
	os.Stdin = f
	t.Cleanup(func() {
		os.Stdin = old
		f.Close()
	})
}

func TestExpandPath(t *testing.T) {
	home := withHome(t)

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"tilde", "~/clipack", filepath.Join(home, "clipack")},
		{"bare tilde", "~", home},
		{"absolute", "/opt/clipack", "/opt/clipack"},
		{"trims whitespace", "  /opt/clipack  ", "/opt/clipack"},
		{"empty stays empty", "", ""},
		{"cleans up", "/opt/./clipack/../clipack", "/opt/clipack"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExpandPath(tt.in); got != tt.want {
				t.Errorf("ExpandPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestExpandPathMakesRelativeAbsolute(t *testing.T) {
	withHome(t)

	got := ExpandPath("relative/dir")
	if !filepath.IsAbs(got) {
		t.Errorf("ExpandPath(relative) = %q, want an absolute path", got)
	}
	if !strings.HasSuffix(got, filepath.Join("relative", "dir")) {
		t.Errorf("ExpandPath(relative) = %q, want it to end with relative/dir", got)
	}
}

func TestNewDefaultConfig(t *testing.T) {
	config := NewDefaultConfig("/opt/clipack")

	if config.Paths.Base != "/opt/clipack" {
		t.Errorf("Base = %q, want /opt/clipack", config.Paths.Base)
	}
	for name, got := range map[string]string{
		"registry": config.Paths.Registry,
		"bin":      config.Paths.Bin,
		"configs":  config.Paths.Configs,
		"build":    config.Paths.Build,
		"man":      config.Paths.Man,
	} {
		want := filepath.Join("/opt/clipack", name)
		if got != want {
			t.Errorf("Paths.%s = %q, want %q", name, got, want)
		}
	}

	if config.Registry.URL != DefaultRegistryURL {
		t.Errorf("Registry.URL = %q, want the default", config.Registry.URL)
	}
	if config.Registry.Branch != DefaultBranch {
		t.Errorf("Registry.Branch = %q, want %q", config.Registry.Branch, DefaultBranch)
	}
	if config.Registry.UpdateInterval != DefaultUpdateInterval {
		t.Errorf("UpdateInterval = %v, want %v", config.Registry.UpdateInterval, DefaultUpdateInterval)
	}
	if config.Options.InstallMethod != "version" {
		t.Errorf("InstallMethod = %q, want version", config.Options.InstallMethod)
	}
	if !config.Options.CleanupBuild {
		t.Error("CleanupBuild = false, want true by default")
	}
}

func TestNewDefaultConfigExpandsTilde(t *testing.T) {
	home := withHome(t)

	config := NewDefaultConfig("~/somewhere")
	if config.Paths.Base != filepath.Join(home, "somewhere") {
		t.Errorf("Base = %q, want the tilde expanded", config.Paths.Base)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	withHome(t)
	installDir := filepath.Join(t.TempDir(), "packages")

	config := NewDefaultConfig(installDir)
	config.Registry.Token = "secret-token"
	config.Options.InstallMethod = "commit"

	if err := config.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Save also creates the directory tree it describes.
	for _, dir := range config.Dirs() {
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("Save() did not create %s", dir)
		}
	}

	loaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if loaded.Registry.Token != "secret-token" {
		t.Errorf("Token = %q, want it to round-trip", loaded.Registry.Token)
	}
	if loaded.Options.InstallMethod != "commit" {
		t.Errorf("InstallMethod = %q, want commit", loaded.Options.InstallMethod)
	}
	if loaded.Paths.Bin != config.Paths.Bin {
		t.Errorf("Bin = %q, want %q", loaded.Paths.Bin, config.Paths.Bin)
	}
	if loaded.Registry.UpdateInterval != DefaultUpdateInterval {
		t.Errorf("UpdateInterval = %v, want it to round-trip", loaded.Registry.UpdateInterval)
	}
}

func TestExistsAndConfigPath(t *testing.T) {
	withHome(t)

	if Exists() {
		t.Error("Exists() = true before anything was written")
	}

	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath() error = %v", err)
	}
	if filepath.Base(path) != "config.yaml" {
		t.Errorf("ConfigPath() = %q, want it to end in config.yaml", path)
	}

	if err := NewDefaultConfig(t.TempDir()).Save(); err != nil {
		t.Fatal(err)
	}
	if !Exists() {
		t.Error("Exists() = false after Save()")
	}
}

func TestLoadConfigMissing(t *testing.T) {
	withHome(t)

	if _, err := LoadConfig(); err == nil {
		t.Error("LoadConfig() error = nil, want an error when no config exists")
	}
}

func TestLoadConfigInvalidYAML(t *testing.T) {
	withHome(t)

	dir, err := ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("registry: [unterminated"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := LoadConfig(); err == nil {
		t.Error("LoadConfig() error = nil, want a parse error")
	}
}

func TestValidateConfigDefaults(t *testing.T) {
	config := &Config{
		Registry: RegistryConfig{URL: "https://github.com/owner/repo.git"},
		Paths: PathsConfig{
			Base: "/a", Registry: "/a/r", Bin: "/a/b", Configs: "/a/c", Build: "/a/bu", Man: "/a/m",
		},
	}

	if err := validateConfig(config); err != nil {
		t.Fatalf("validateConfig() error = %v", err)
	}

	// Missing optional values are filled in rather than rejected, so an older
	// config file keeps working.
	if config.Registry.Branch != DefaultBranch {
		t.Errorf("Branch = %q, want it defaulted to %q", config.Registry.Branch, DefaultBranch)
	}
	if config.Registry.UpdateInterval != DefaultUpdateInterval {
		t.Errorf("UpdateInterval = %v, want it defaulted", config.Registry.UpdateInterval)
	}
	if config.Options.InstallMethod != "version" {
		t.Errorf("InstallMethod = %q, want it defaulted to version", config.Options.InstallMethod)
	}
}

func TestValidateConfigErrors(t *testing.T) {
	full := func() *Config {
		return &Config{
			Registry: RegistryConfig{URL: "https://github.com/owner/repo.git", UpdateInterval: time.Hour},
			Paths: PathsConfig{
				Base: "/a", Registry: "/a/r", Bin: "/a/b", Configs: "/a/c", Build: "/a/bu", Man: "/a/m",
			},
		}
	}

	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{
			name:   "missing registry url",
			mutate: func(c *Config) { c.Registry.URL = "" },
			want:   "registry URL",
		},
		{
			name:   "empty path",
			mutate: func(c *Config) { c.Paths.Bin = "" },
			want:   `"bin" is not set`,
		},
		{
			// The error has to name the offending path; the old message just
			// said "all paths must be absolute".
			name:   "relative path",
			mutate: func(c *Config) { c.Paths.Man = "relative/man" },
			want:   `"man" must be absolute`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := full()
			tt.mutate(config)

			err := validateConfig(config)
			if err == nil {
				t.Fatal("validateConfig() error = nil, want an error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %v, want it to mention %q", err, tt.want)
			}
		})
	}
}

func TestEnsureDirs(t *testing.T) {
	base := t.TempDir()
	config := NewDefaultConfig(base)

	if err := config.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs() error = %v", err)
	}
	for _, dir := range config.Dirs() {
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("%s was not created", dir)
		}
	}

	// Running it again on an existing tree is not an error.
	if err := config.EnsureDirs(); err != nil {
		t.Errorf("EnsureDirs() on an existing tree error = %v, want nil", err)
	}
}

func TestDefaultInstallDir(t *testing.T) {
	home := withHome(t)

	if got := DefaultInstallDir(); got != filepath.Join(home, "clipack") {
		t.Errorf("DefaultInstallDir() = %q, want %q", got, filepath.Join(home, "clipack"))
	}
}

func TestShellExportLines(t *testing.T) {
	tests := []struct {
		shell   string
		want    []string
		wantErr bool
	}{
		{shell: "/bin/bash", want: []string{`export PATH="/bin:$PATH"`, `export MANPATH="/man:$MANPATH"`}},
		{shell: "/usr/bin/zsh", want: []string{`export PATH="/bin:$PATH"`}},
		{shell: "/usr/bin/fish", want: []string{"set -x PATH /bin $PATH", "set -x MANPATH /man $MANPATH"}},
		{shell: "/bin/csh", wantErr: true},
		{shell: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			t.Setenv("SHELL", tt.shell)

			got, err := ShellExportLines("/bin", "/man")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ShellExportLines() error = nil, want an error for %q", tt.shell)
				}
				return
			}
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

func TestGetShellConfigFilePath(t *testing.T) {
	tests := []struct {
		shell    string
		wantBase string
		wantErr  bool
	}{
		{shell: "/bin/bash", wantBase: ".bashrc"},
		{shell: "/usr/bin/zsh", wantBase: ".zshrc"},
		{shell: "/usr/bin/fish", wantBase: "config.fish"},
		{shell: "/bin/ksh", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			t.Setenv("SHELL", tt.shell)

			got, err := GetShellConfigFilePath()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("GetShellConfigFilePath() error = nil, want an error for %q", tt.shell)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetShellConfigFilePath() error = %v", err)
			}
			if filepath.Base(got) != tt.wantBase {
				t.Errorf("GetShellConfigFilePath() = %q, want it to end in %q", got, tt.wantBase)
			}
		})
	}
}

func TestAddPathsToShellConfigIsIdempotent(t *testing.T) {
	home := withHome(t)
	t.Setenv("SHELL", "/bin/bash")

	rc, err := GetShellConfigFilePath()
	if err != nil {
		t.Fatalf("GetShellConfigFilePath() error = %v", err)
	}
	// The rc path has to follow $HOME, the same home ConfigDir uses.
	if !strings.HasPrefix(rc, home) {
		t.Fatalf("rc path %q is outside the test home %q", rc, home)
	}

	binPath := filepath.Join(home, "clipack", "bin")
	manPath := filepath.Join(home, "clipack", "man")

	if err := AddPathsToShellConfig(binPath, manPath); err != nil {
		t.Fatalf("AddPathsToShellConfig() error = %v", err)
	}

	first, err := os.ReadFile(rc)
	if err != nil {
		t.Fatalf("the rc file was not written: %v", err)
	}
	if !strings.Contains(string(first), binPath) {
		t.Fatalf("rc file = %q, want it to reference the bin path", first)
	}

	// Running it a second time must not stack a duplicate export block.
	if err := AddPathsToShellConfig(binPath, manPath); err != nil {
		t.Fatalf("second AddPathsToShellConfig() error = %v", err)
	}

	second, err := os.ReadFile(rc)
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

func TestAskInstallDirectory(t *testing.T) {
	home := withHome(t)

	t.Run("uses the given path", func(t *testing.T) {
		withStdin(t, "/opt/clipack\n")

		got, err := AskInstallDirectory()
		if err != nil {
			t.Fatalf("AskInstallDirectory() error = %v", err)
		}
		if got != "/opt/clipack" {
			t.Errorf("AskInstallDirectory() = %q, want /opt/clipack", got)
		}
	})

	t.Run("empty input falls back to the default", func(t *testing.T) {
		withStdin(t, "\n")

		got, err := AskInstallDirectory()
		if err != nil {
			t.Fatalf("AskInstallDirectory() error = %v", err)
		}
		if got != filepath.Join(home, "clipack") {
			t.Errorf("AskInstallDirectory() = %q, want the default", got)
		}
	})

	t.Run("expands a tilde", func(t *testing.T) {
		withStdin(t, "~/elsewhere\n")

		got, err := AskInstallDirectory()
		if err != nil {
			t.Fatalf("AskInstallDirectory() error = %v", err)
		}
		if got != filepath.Join(home, "elsewhere") {
			t.Errorf("AskInstallDirectory() = %q, want the tilde expanded", got)
		}
	})
}

func TestCreateDefaultConfigIsNoOpWhenPresent(t *testing.T) {
	withHome(t)

	config := NewDefaultConfig(filepath.Join(t.TempDir(), "original"))
	config.Registry.Token = "keep-me"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}

	// No stdin is provided: if CreateDefaultConfig tried to prompt, it would
	// read from the test binary's stdin instead of returning early.
	if err := CreateDefaultConfig(); err != nil {
		t.Fatalf("CreateDefaultConfig() error = %v", err)
	}

	loaded, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Registry.Token != "keep-me" {
		t.Errorf("Token = %q, want the existing config left untouched", loaded.Registry.Token)
	}
}

func TestAddPathsToShellConfigFish(t *testing.T) {
	home := withHome(t)
	t.Setenv("SHELL", "/usr/bin/fish")

	binPath := filepath.Join(home, "clipack", "bin")
	manPath := filepath.Join(home, "clipack", "man")

	// config.fish sits two directories deep, neither of which exists yet.
	if err := AddPathsToShellConfig(binPath, manPath); err != nil {
		t.Fatalf("AddPathsToShellConfig() error = %v", err)
	}

	rc := filepath.Join(home, ".config", "fish", "config.fish")
	contents, err := os.ReadFile(rc)
	if err != nil {
		t.Fatalf("config.fish was not created: %v", err)
	}
	if !strings.Contains(string(contents), "set -x PATH "+binPath) {
		t.Errorf("config.fish = %q, want fish syntax", contents)
	}
}

func TestAddPathsToShellConfigUnsupportedShell(t *testing.T) {
	withHome(t)
	t.Setenv("SHELL", "/bin/tcsh")

	if err := AddPathsToShellConfig("/bin", "/man"); err == nil {
		t.Error("AddPathsToShellConfig() error = nil, want an error for an unsupported shell")
	}
}

func TestAddPathsToShellConfigAppendsToAnExistingFile(t *testing.T) {
	home := withHome(t)
	t.Setenv("SHELL", "/bin/bash")

	rc := filepath.Join(home, ".bashrc")
	if err := os.WriteFile(rc, []byte("# existing user configuration\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := AddPathsToShellConfig(filepath.Join(home, "bin"), filepath.Join(home, "man")); err != nil {
		t.Fatalf("AddPathsToShellConfig() error = %v", err)
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

func TestCreateDefaultConfigFirstRun(t *testing.T) {
	home := withHome(t)
	t.Setenv("SHELL", "/bin/bash")

	installDir := filepath.Join(t.TempDir(), "packages")
	// The trailing "n" declines the shell-configuration prompt.
	withStdin(t, installDir+"\nn\n")

	if err := CreateDefaultConfig(); err != nil {
		t.Fatalf("CreateDefaultConfig() error = %v", err)
	}

	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() after CreateDefaultConfig error = %v", err)
	}
	if config.Paths.Base != installDir {
		t.Errorf("Base = %q, want %q", config.Paths.Base, installDir)
	}
	if config.Registry.URL != DefaultRegistryURL {
		t.Errorf("Registry.URL = %q, want the default", config.Registry.URL)
	}
	for _, dir := range config.Dirs() {
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("%s was not created", dir)
		}
	}

	// The prompt was declined, so the rc file must be untouched.
	if _, err := os.Stat(filepath.Join(home, ".bashrc")); err == nil {
		t.Error("the shell rc file was written even though the prompt was declined")
	}
}

func TestCreateDefaultConfigAddsShellPathsWhenAccepted(t *testing.T) {
	home := withHome(t)
	t.Setenv("SHELL", "/bin/zsh")

	installDir := filepath.Join(t.TempDir(), "packages")
	withStdin(t, installDir+"\ny\n")

	if err := CreateDefaultConfig(); err != nil {
		t.Fatalf("CreateDefaultConfig() error = %v", err)
	}

	contents, err := os.ReadFile(filepath.Join(home, ".zshrc"))
	if err != nil {
		t.Fatalf(".zshrc was not written: %v", err)
	}
	if !strings.Contains(string(contents), filepath.Join(installDir, "bin")) {
		t.Errorf(".zshrc = %q, want it to reference the bin directory", contents)
	}
}

func TestUpdateConfigPreservesRegistryAndOptions(t *testing.T) {
	withHome(t)
	t.Setenv("SHELL", "/bin/bash")

	original := NewDefaultConfig(filepath.Join(t.TempDir(), "old"))
	original.Registry.Token = "secret-token"
	original.Registry.Branch = "develop"
	original.Registry.URL = "https://github.com/someone/private-registry.git"
	original.Options.InstallMethod = "commit"
	original.Options.CleanupBuild = false
	if err := original.Save(); err != nil {
		t.Fatal(err)
	}

	newDir := filepath.Join(t.TempDir(), "new")
	// The trailing "n" answers the "add paths to your shell config?" prompt.
	withStdin(t, newDir+"\nn\n")

	if err := UpdateConfig(); err != nil {
		t.Fatalf("UpdateConfig() error = %v", err)
	}

	loaded, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}

	// The paths move; everything the user configured has to survive. The old
	// implementation rewrote the file from a hardcoded template and lost all
	// of this.
	if loaded.Paths.Base != newDir {
		t.Errorf("Base = %q, want %q", loaded.Paths.Base, newDir)
	}
	if loaded.Registry.Token != "secret-token" {
		t.Errorf("Token = %q, want it preserved", loaded.Registry.Token)
	}
	if loaded.Registry.Branch != "develop" {
		t.Errorf("Branch = %q, want develop preserved", loaded.Registry.Branch)
	}
	if loaded.Registry.URL != "https://github.com/someone/private-registry.git" {
		t.Errorf("URL = %q, want the custom registry preserved", loaded.Registry.URL)
	}
	if loaded.Options.InstallMethod != "commit" {
		t.Errorf("InstallMethod = %q, want commit preserved", loaded.Options.InstallMethod)
	}
	if loaded.Options.CleanupBuild {
		t.Error("CleanupBuild = true, want the configured false preserved")
	}
}
