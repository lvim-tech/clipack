package pkg

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/lvim-tech/clipack/cnfg"
)

// integrationPackage builds a package that ships shell integration, the way
// zoxide or starship do.
func integrationPackage() *Package {
	p := buildablePackage()
	p.Install.AdditionalConfig = append(p.Install.AdditionalConfig,
		AdditionalConfig{Filename: "config.sh", Content: "export DEMO_READY=1\n"})
	return p
}

func TestInstallRegeneratesTheAggregate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shell syntax under test is POSIX")
	}

	config := testConfig(t)
	in := NewInstaller(config, nil)

	p := integrationPackage()
	if err := in.Install(p, MethodVersion); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	aggregate := cnfg.IntegrationPath(config.Paths.Configs)
	contents, err := os.ReadFile(aggregate)
	if err != nil {
		t.Fatalf("the aggregate was not written: %v", err)
	}
	script := filepath.Join(config.Paths.Configs, "demo", "config.sh")
	if !strings.Contains(string(contents), script) {
		t.Errorf("aggregate = %q, want it to source %q", contents, script)
	}

	// Removing the package has to take its line with it — a shell sourcing a
	// file that is gone is the failure the regeneration exists to prevent.
	if err := in.Remove(p); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	contents, err = os.ReadFile(aggregate)
	if err != nil {
		t.Fatalf("the aggregate disappeared on remove: %v", err)
	}
	if strings.Contains(string(contents), script) {
		t.Errorf("aggregate = %q, want the removed package gone from it", contents)
	}
}

// TestAggregateSkipsPackagesWithoutIntegration keeps the file honest: a package
// with no config.sh contributes nothing, rather than a line sourcing a missing
// file.
func TestAggregateSkipsPackagesWithoutIntegration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shell syntax under test is POSIX")
	}

	config := testConfig(t)
	in := NewInstaller(config, nil)

	if err := in.Install(buildablePackage(), MethodVersion); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	contents, err := os.ReadFile(cnfg.IntegrationPath(config.Paths.Configs))
	if err != nil {
		t.Fatalf("the aggregate was not written: %v", err)
	}
	if strings.Contains(string(contents), filepath.Join("demo", "config.sh")) {
		t.Errorf("aggregate = %q, want no line for a package without config.sh", contents)
	}
}

// TestAggregateIsPlainPOSIX guards the file against shell-isms. It used to
// locate itself with ${0:a:h}, which is zsh syntax and a fatal error in dash —
// baking the absolute path in is what keeps it sourceable everywhere.
func TestAggregateIsPlainPOSIX(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shell syntax under test is POSIX")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh on this system")
	}

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "demo", "config.sh"), []byte("DEMO=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	aggregate := filepath.Join(dir, "clipack.sh")
	if err := os.WriteFile(aggregate, []byte(renderIntegration(dir, []string{"demo"})), 0o644); err != nil {
		t.Fatal(err)
	}

	// Not just parsed — executed. `sh -n` would miss a bad substitution that
	// only fails at expansion time.
	out, err := exec.Command(sh, "-c", ". "+aggregate).CombinedOutput()
	if err != nil {
		t.Errorf("sourcing the aggregate under sh failed: %v\n%s", err, out)
	}

	// Textual backstop, because the execution check is only as strict as this
	// machine's sh: on a distribution where sh is bash, ${0:a:h} expands to an
	// empty string instead of failing, and only dash rejects it outright.
	contents, err := os.ReadFile(aggregate)
	if err != nil {
		t.Fatal(err)
	}
	for _, ism := range []string{"${0:", "BASH_SOURCE", "$ZSH_VERSION"} {
		if strings.Contains(string(contents), ism) {
			t.Errorf("aggregate contains %q, want plain POSIX with baked-in paths", ism)
		}
	}
}

// TestInstallRunsTheSetupScript is the point of the setup field: linking a
// theme into ~/.config is done once, by clipack, instead of by every shell that
// happens to source a config.sh.
func TestInstallRunsTheSetupScript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shell syntax under test is POSIX")
	}

	config := testConfig(t)
	in := NewInstaller(config, nil)

	p := buildablePackage()
	p.Install.Setup = `printf ran > setup-marker`

	if err := in.Install(p, MethodVersion); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	// runSetup runs in the base directory, so relative paths land there.
	marker := filepath.Join(config.Paths.Base, "setup-marker")
	contents, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("the setup script did not run: %v", err)
	}
	if string(contents) != "ran" {
		t.Errorf("marker = %q, want %q", contents, "ran")
	}
}

// TestSetupFailureDoesNotFailTheInstall pins the severity. The program is
// installed and runnable by the time setup runs; a broken theme link is not
// worth reporting the whole install as failed.
func TestSetupFailureDoesNotFailTheInstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shell syntax under test is POSIX")
	}

	config := testConfig(t)
	in := NewInstaller(config, nil)

	p := buildablePackage()
	p.Install.Setup = "exit 1"

	if err := in.Install(p, MethodVersion); err != nil {
		t.Errorf("Install() error = %v, want a failing setup tolerated", err)
	}
	if !exists(filepath.Join(config.Paths.Bin, "demo")) {
		t.Error("the binary was not installed")
	}
}
