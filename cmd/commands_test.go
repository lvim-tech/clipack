package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lvim-tech/clipack/pkg"
)

// ---------------------------------------------------------------------------
// list
// ---------------------------------------------------------------------------

func TestListShowsEveryPackageWithItsStatus(t *testing.T) {
	config := setupCmdTest(t)

	demo := demoPackage()
	other := otherPackage()
	seedCache(t, config, demo, other)

	// demo is installed at an older version, other is not installed at all.
	installed := demoPackage()
	installed.Version = "v0.9.0"
	installed.InstallMethod = pkg.MethodVersion
	installManifest(t, config, installed)

	stdout, _, err := execute(t, "list")
	if err != nil {
		t.Fatalf("list error = %v", err)
	}

	if !strings.Contains(stdout, "NAME") || !strings.Contains(stdout, "STATUS") {
		t.Errorf("no table header in:\n%s", stdout)
	}
	for _, want := range []string{"demo", "other", "update", "cli", "file_managers"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("list output is missing %q:\n%s", want, stdout)
		}
	}
	if !strings.Contains(stdout, "2 of 2 package(s)") {
		t.Errorf("no count line in:\n%s", stdout)
	}
}

func TestListDescriptionsStayOnOneLine(t *testing.T) {
	config := setupCmdTest(t)
	seedCache(t, config, otherPackage())

	stdout, _, err := execute(t, "list")
	if err != nil {
		t.Fatalf("list error = %v", err)
	}

	// otherPackage's description has a newline; a multi-line cell would break
	// the table alignment.
	if strings.Contains(stdout, "with a second line") {
		t.Errorf("the second description line leaked into the table:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Another package") {
		t.Errorf("the first description line is missing:\n%s", stdout)
	}
}

func TestListFilters(t *testing.T) {
	config := setupCmdTest(t)

	demo := demoPackage()
	seedCache(t, config, demo, otherPackage())

	current := demoPackage() // installed and up to date
	current.InstallMethod = pkg.MethodVersion
	installManifest(t, config, current)

	t.Run("installed", func(t *testing.T) {
		stdout, _, err := execute(t, "list", "--installed")
		if err != nil {
			t.Fatalf("list --installed error = %v", err)
		}
		if !strings.Contains(stdout, "demo") {
			t.Errorf("the installed package is missing:\n%s", stdout)
		}
		if strings.Contains(stdout, "other") {
			t.Errorf("a package that is not installed was listed:\n%s", stdout)
		}
		if !strings.Contains(stdout, "1 of 2 package(s)") {
			t.Errorf("wrong count:\n%s", stdout)
		}
	})

	t.Run("updates", func(t *testing.T) {
		// Everything installed is current, so nothing should be listed.
		stdout, _, err := execute(t, "list", "--updates")
		if err != nil {
			t.Fatalf("list --updates error = %v", err)
		}
		if !strings.Contains(stdout, "0 of 2 package(s)") {
			t.Errorf("wrong count for an up-to-date system:\n%s", stdout)
		}
	})
}

func TestListWithoutARegistryFails(t *testing.T) {
	config := setupCmdTest(t)
	config.Registry.URL = "https://github.com/definitely/not-a-real-registry-xyz.git"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}

	if _, _, err := execute(t, "list"); err == nil {
		t.Error("list error = nil, want a failure when the registry is unreachable")
	}
}

// ---------------------------------------------------------------------------
// install
// ---------------------------------------------------------------------------

func TestInstallUnknownPackage(t *testing.T) {
	config := setupCmdTest(t)
	seedCache(t, config, demoPackage())

	_, _, err := execute(t, "install", "nosuchpackage", "-y")
	if err == nil {
		t.Fatal("install error = nil, want an unknown package to fail")
	}
	if !strings.Contains(err.Error(), "nosuchpackage") {
		t.Errorf("error = %v, want it to name the package", err)
	}
}

func TestInstallEndToEnd(t *testing.T) {
	config := setupCmdTest(t)
	seedCache(t, config, demoPackage())

	stdout, _, err := execute(t, "install", "demo", "-y")
	if err != nil {
		t.Fatalf("install error = %v", err)
	}

	if !exists(filepath.Join(config.Paths.Bin, "demo")) {
		t.Error("the binary was not installed")
	}
	if !exists(filepath.Join(config.Paths.Configs, "demo", "package.yaml")) {
		t.Error("the manifest was not written")
	}
	// The step counter and the completion line are what the user watches.
	if !strings.Contains(stdout, "[1/2]") || !strings.Contains(stdout, "✓") {
		t.Errorf("install output is missing the progress:\n%s", stdout)
	}
}

func TestInstallSeveralPackages(t *testing.T) {
	config := setupCmdTest(t)

	second := demoPackage()
	second.Name = "demo2"
	second.Install.Steps = []string{"mkdir -p out", `printf 'x' > out/demo2`}
	second.Install.Binaries = []string{"out/demo2"}
	seedCache(t, config, demoPackage(), second)

	if _, _, err := execute(t, "install", "demo", "demo2", "-y"); err != nil {
		t.Fatalf("install error = %v", err)
	}

	for _, name := range []string{"demo", "demo2"} {
		if !exists(filepath.Join(config.Paths.Bin, name)) {
			t.Errorf("%s was not installed", name)
		}
	}
}

func TestInstallHonoursADeclinedPrompt(t *testing.T) {
	config := setupCmdTest(t)
	seedCache(t, config, demoPackage())

	withStdin(t, "n\n")

	stdout, _, err := execute(t, "install", "demo")
	if err != nil {
		t.Fatalf("install error = %v", err)
	}
	if exists(filepath.Join(config.Paths.Bin, "demo")) {
		t.Error("the package was installed despite the prompt being declined")
	}
	if !strings.Contains(stdout, "Skipped demo") {
		t.Errorf("output does not report the skip:\n%s", stdout)
	}
}

func TestInstallShowsTheSummaryBeforeAsking(t *testing.T) {
	config := setupCmdTest(t)
	seedCache(t, config, demoPackage())

	withStdin(t, "n\n")

	stdout, _, err := execute(t, "install", "demo")
	if err != nil {
		t.Fatalf("install error = %v", err)
	}
	for _, want := range []string{"demo", "A demo package for tests", "v1.0.0", "MIT", "build dir"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("the confirmation summary is missing %q:\n%s", want, stdout)
		}
	}
}

func TestInstallMethodFlagSelectsTheCommit(t *testing.T) {
	config := setupCmdTest(t)
	seedCache(t, config, demoPackage())

	withStdin(t, "n\n")

	stdout, _, err := execute(t, "install", "demo", "-m", "commit")
	if err != nil {
		t.Fatalf("install error = %v", err)
	}
	if !strings.Contains(stdout, "0123456789abcdef") {
		t.Errorf("the summary does not show the commit ref:\n%s", stdout)
	}
}

func TestInstallRefusesAnAlreadyInstalledPackage(t *testing.T) {
	config := setupCmdTest(t)
	seedCache(t, config, demoPackage())
	installManifest(t, config, demoPackage())

	_, _, err := execute(t, "install", "demo", "-y")
	if err == nil {
		t.Fatal("install error = nil, want reinstalling to be refused")
	}
	// Installing over an existing install leaves the old version's binaries
	// behind, so the error points at the two commands that do it properly.
	for _, want := range []string{"already installed", "update", "remove"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %v, want it to mention %q", err, want)
		}
	}
}

func TestInstallDoesNotMutateTheCachedRegistry(t *testing.T) {
	config := setupCmdTest(t)
	seedCache(t, config, demoPackage())

	if _, _, err := execute(t, "install", "demo", "-y"); err != nil {
		t.Fatalf("install error = %v", err)
	}

	// The installer stamps InstallMethod onto the package it is given. The
	// command hands it a copy, so the cached entry has to be untouched.
	cached, err := pkg.LoadFromCache(config)
	if err != nil {
		t.Fatal(err)
	}
	if cached[0].InstallMethod != "" {
		t.Errorf("the cached package now records install_method=%q", cached[0].InstallMethod)
	}
}

// ---------------------------------------------------------------------------
// remove
// ---------------------------------------------------------------------------

func TestRemoveNothingInstalled(t *testing.T) {
	setupCmdTest(t)

	stdout, _, err := execute(t, "remove", "demo")
	if err != nil {
		t.Fatalf("remove error = %v", err)
	}
	if !strings.Contains(stdout, "No packages are installed") {
		t.Errorf("output = %q, want a clear message", stdout)
	}
}

func TestRemoveUnknownPackage(t *testing.T) {
	config := setupCmdTest(t)
	installManifest(t, config, demoPackage())

	_, _, err := execute(t, "remove", "something-else", "-y")
	if err == nil {
		t.Fatal("remove error = nil, want an error for a package that is not installed")
	}
	if !strings.Contains(err.Error(), "something-else") {
		t.Errorf("error = %v, want it to name the package", err)
	}
}

func TestRemoveEndToEnd(t *testing.T) {
	config := setupCmdTest(t)
	seedCache(t, config, demoPackage())

	if _, _, err := execute(t, "install", "demo", "-y"); err != nil {
		t.Fatalf("install error = %v", err)
	}

	stdout, _, err := execute(t, "remove", "demo", "-y")
	if err != nil {
		t.Fatalf("remove error = %v", err)
	}

	if exists(filepath.Join(config.Paths.Bin, "demo")) {
		t.Error("the binary survived the removal")
	}
	if exists(filepath.Join(config.Paths.Configs, "demo")) {
		t.Error("the config directory survived the removal")
	}
	if !strings.Contains(stdout, "Successfully removed demo") {
		t.Errorf("output does not confirm the removal:\n%s", stdout)
	}
}

func TestRemoveHonoursADeclinedPrompt(t *testing.T) {
	config := setupCmdTest(t)
	seedCache(t, config, demoPackage())

	if _, _, err := execute(t, "install", "demo", "-y"); err != nil {
		t.Fatalf("install error = %v", err)
	}

	withStdin(t, "n\n")

	if _, _, err := execute(t, "remove", "demo"); err != nil {
		t.Fatalf("remove error = %v", err)
	}
	if !exists(filepath.Join(config.Paths.Bin, "demo")) {
		t.Error("the package was removed despite the prompt being declined")
	}
}

// ---------------------------------------------------------------------------
// update
// ---------------------------------------------------------------------------

func TestUpdateWithNothingInstalled(t *testing.T) {
	config := setupCmdTest(t)
	seedCache(t, config, demoPackage())

	stdout, _, err := execute(t, "update")
	if err != nil {
		t.Fatalf("update error = %v", err)
	}
	if !strings.Contains(stdout, "No packages are installed") {
		t.Errorf("output = %q, want a clear message", stdout)
	}
}

func TestUpdateListsWhatIsAvailable(t *testing.T) {
	config := setupCmdTest(t)
	seedCache(t, config, demoPackage())

	old := demoPackage()
	old.Version = "v0.9.0"
	old.InstallMethod = pkg.MethodVersion
	installManifest(t, config, old)

	stdout, _, err := execute(t, "update")
	if err != nil {
		t.Fatalf("update error = %v", err)
	}

	// The installed and the available ref both have to be shown, and they have
	// to belong to the same package: the old code printed the version of an
	// unrelated installed package here.
	if !strings.Contains(stdout, "v0.9.0") || !strings.Contains(stdout, "v1.0.0") {
		t.Errorf("output does not show both refs:\n%s", stdout)
	}
	if !strings.Contains(stdout, "1 update(s) available") {
		t.Errorf("output does not count the updates:\n%s", stdout)
	}
	// Listing alone must not change anything on disk.
	if exists(filepath.Join(config.Paths.Bin, "demo")) {
		t.Error("update without --all installed something")
	}
}

func TestUpdateAllAppliesThem(t *testing.T) {
	config := setupCmdTest(t)
	seedCache(t, config, demoPackage())

	old := demoPackage()
	old.Version = "v0.9.0"
	old.InstallMethod = pkg.MethodVersion
	installManifest(t, config, old)

	if _, _, err := execute(t, "update", "--all", "-y"); err != nil {
		t.Fatalf("update --all error = %v", err)
	}

	if !exists(filepath.Join(config.Paths.Bin, "demo")) {
		t.Error("the update did not install the binary")
	}

	installed, err := pkg.InstalledMap(config)
	if err != nil {
		t.Fatal(err)
	}
	if installed["demo"].Version != "v1.0.0" {
		t.Errorf("recorded version = %q, want v1.0.0", installed["demo"].Version)
	}
}

func TestUpdateKeepsTheInstalledMethod(t *testing.T) {
	config := setupCmdTest(t)
	seedCache(t, config, demoPackage())

	// Installed by commit; the update must not silently switch it to a version
	// tag just because that is the configured default.
	old := demoPackage()
	old.Commit = "oldcommit0000000"
	old.InstallMethod = pkg.MethodCommit
	installManifest(t, config, old)

	if _, _, err := execute(t, "update", "--all", "-y"); err != nil {
		t.Fatalf("update --all error = %v", err)
	}

	installed, err := pkg.InstalledMap(config)
	if err != nil {
		t.Fatal(err)
	}
	if installed["demo"].InstallMethod != pkg.MethodCommit {
		t.Errorf("install_method = %q, want it kept as commit", installed["demo"].InstallMethod)
	}
}

func TestUpdateNamedPackageThatIsCurrent(t *testing.T) {
	config := setupCmdTest(t)
	seedCache(t, config, demoPackage())

	current := demoPackage()
	current.InstallMethod = pkg.MethodVersion
	installManifest(t, config, current)

	stdout, _, err := execute(t, "update", "demo")
	if err != nil {
		t.Fatalf("update error = %v", err)
	}
	if !strings.Contains(stdout, "already up to date") {
		t.Errorf("output = %q, want it to say the package is current", stdout)
	}
}

func TestUpdateNamedPackageErrors(t *testing.T) {
	config := setupCmdTest(t)
	seedCache(t, config, demoPackage())
	installManifest(t, config, demoPackage())

	t.Run("not in the registry", func(t *testing.T) {
		_, _, err := execute(t, "update", "ghost")
		if err == nil || !strings.Contains(err.Error(), "not found in registry") {
			t.Errorf("error = %v, want a registry lookup failure", err)
		}
	})

	t.Run("not installed", func(t *testing.T) {
		seedCache(t, config, demoPackage(), otherPackage())
		_, _, err := execute(t, "update", "other")
		if err == nil || !strings.Contains(err.Error(), "not installed") {
			t.Errorf("error = %v, want a not-installed failure", err)
		}
	})
}

func TestUpdateAllIsUpToDate(t *testing.T) {
	config := setupCmdTest(t)
	seedCache(t, config, demoPackage())

	current := demoPackage()
	current.InstallMethod = pkg.MethodVersion
	installManifest(t, config, current)

	stdout, _, err := execute(t, "update")
	if err != nil {
		t.Fatalf("update error = %v", err)
	}
	if !strings.Contains(stdout, "All packages are up to date") {
		t.Errorf("output = %q, want the up-to-date message", stdout)
	}
}

// ---------------------------------------------------------------------------
// preview
// ---------------------------------------------------------------------------

func TestPreviewShowsTheRegistryRecord(t *testing.T) {
	config := setupCmdTest(t)
	seedCache(t, config, demoPackage())

	stdout, _, err := execute(t, "preview", "demo")
	if err != nil {
		t.Fatalf("preview error = %v", err)
	}

	for _, want := range []string{"name: demo", "version: v1.0.0", "status: not installed"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("preview output is missing %q:\n%s", want, stdout)
		}
	}
}

func TestPreviewShowsTheInstalledState(t *testing.T) {
	config := setupCmdTest(t)
	seedCache(t, config, demoPackage())

	old := demoPackage()
	old.Version = "v0.9.0"
	old.InstallMethod = pkg.MethodVersion
	installManifest(t, config, old)

	stdout, _, err := execute(t, "preview", "demo")
	if err != nil {
		t.Fatalf("preview error = %v", err)
	}

	for _, want := range []string{"status: installed", "install_method: version", "installed_ref: v0.9.0", "available_ref: v1.0.0"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("preview output is missing %q:\n%s", want, stdout)
		}
	}
}

func TestPreviewUnknownPackage(t *testing.T) {
	config := setupCmdTest(t)
	config.Registry.URL = "https://github.com/definitely/not-a-real-registry-xyz.git"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	seedCache(t, config, demoPackage())

	if _, _, err := execute(t, "preview", "ghost"); err == nil {
		t.Error("preview error = nil, want an error for an unknown package")
	}
}

// ---------------------------------------------------------------------------
// add-executables-path
// ---------------------------------------------------------------------------

func TestAddExecutablesPath(t *testing.T) {
	config := setupCmdTest(t)

	withStdin(t, "y\n")

	stdout, _, err := execute(t, "add-executables-path")
	if err != nil {
		t.Fatalf("add-executables-path error = %v", err)
	}

	// The paths are shown before anything is written.
	if !strings.Contains(stdout, config.Paths.Bin) || !strings.Contains(stdout, config.Paths.Man) {
		t.Errorf("output does not show the paths:\n%s", stdout)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(filepath.Join(home, ".bashrc"))
	if err != nil {
		t.Fatalf(".bashrc was not written: %v", err)
	}
	if !strings.Contains(string(contents), config.Paths.Bin) {
		t.Errorf(".bashrc = %q, want the bin path appended", contents)
	}
}

func TestAddExecutablesPathCancelled(t *testing.T) {
	setupCmdTest(t)

	withStdin(t, "n\n")

	stdout, _, err := execute(t, "add-executables-path")
	if err != nil {
		t.Fatalf("add-executables-path error = %v", err)
	}
	if !strings.Contains(stdout, "Cancelled") {
		t.Errorf("output = %q, want the cancellation reported", stdout)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(home, ".bashrc")) {
		t.Error(".bashrc was written even though the prompt was declined")
	}
}
