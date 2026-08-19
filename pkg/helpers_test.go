package pkg

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/lvim-tech/clipack/cnfg"
)

// testConfig builds a configuration rooted at a temporary directory, with every
// managed path created. Tests never touch the user's real installation.
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
			// Named so no test can ever reach the real ~/.local/bin. It is
			// deliberately not created here: EnsureDirs leaves it to the first
			// link, which is the behaviour under test.
			Expose: filepath.Join(base, "local", "bin"),
		},
		Options: cnfg.OptionsConfig{
			CleanupBuild:  true,
			InstallMethod: MethodVersion,
		},
	}

	if err := config.EnsureDirs(); err != nil {
		t.Fatalf("creating test directories: %v", err)
	}

	return config
}

// writeManifest drops a package.yaml into configs/<name>/, imitating a package
// that has already been installed.
func writeManifest(t *testing.T, configsDir, name, contents string) {
	t.Helper()

	dir := filepath.Join(configsDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.yaml"), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

// recorder collects the events an Installer emits so assertions can be made on
// what the user would have seen. It is safe for concurrent use because the
// installer reports from the goroutines draining a command's output pipes.
type recorder struct {
	mu     sync.Mutex
	events []Event
}

// report is the Reporter passed to NewInstaller.
func (r *recorder) report(e Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

// kinds returns the recorded events of a given kind.
func (r *recorder) kinds(kind EventKind) []Event {
	r.mu.Lock()
	defer r.mu.Unlock()

	var out []Event
	for _, e := range r.events {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

// texts returns the text of every recorded event of a given kind.
func (r *recorder) texts(kind EventKind) []string {
	var out []string
	for _, e := range r.kinds(kind) {
		out = append(out, e.Text)
	}
	return out
}

// exists reports whether a path is present.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
