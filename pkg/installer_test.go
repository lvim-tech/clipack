package pkg

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestShellQuote(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"simple", `'simple'`},
		{"with space", `'with space'`},
		{"it's", `'it'\''s'`},
		{"", `''`},
	}

	for _, tt := range tests {
		if got := shellQuote(tt.in); got != tt.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestManTarget(t *testing.T) {
	tests := []struct {
		manPage string
		want    string
		ok      bool
	}{
		{"man/man1/duf.1", "/man/man1/duf.1", true},
		{"docs/bat.5", "/man/man5/bat.5", true},
		{"share/man/man8/tool.8", "/man/man8/tool.8", true},
		{"README", "", false}, // no extension, so no section
		{"noext.", "", false}, // a dot with nothing after it
	}

	for _, tt := range tests {
		got, ok := manTarget("/man", tt.manPage)
		if ok != tt.ok {
			t.Errorf("manTarget(%q) ok = %v, want %v", tt.manPage, ok, tt.ok)
			continue
		}
		if ok && got != tt.want {
			t.Errorf("manTarget(%q) = %q, want %q", tt.manPage, got, tt.want)
		}
	}
}

func TestResolveMethod(t *testing.T) {
	config := testConfig(t)
	config.Options.InstallMethod = MethodCommit
	in := NewInstaller(config, nil)

	if got := in.ResolveMethod(MethodVersion); got != MethodVersion {
		t.Errorf("ResolveMethod(version) = %q, want version — an explicit method wins", got)
	}
	if got := in.ResolveMethod(""); got != MethodCommit {
		t.Errorf("ResolveMethod(\"\") = %q, want the configured commit", got)
	}

	config.Options.InstallMethod = ""
	if got := in.ResolveMethod(""); got != MethodVersion {
		t.Errorf("ResolveMethod(\"\") with no configured method = %q, want version", got)
	}
}

func TestExpandSteps(t *testing.T) {
	in := NewInstaller(testConfig(t), nil)

	p := &Package{
		Version: "v1.2.3",
		Commit:  "abcdef1234567890",
		Install: Install{
			Source: Source{URL: "https://example.com/tool.git"},
			Steps: []string{
				"git clone https://example.com/tool.git .",
				"make build",
			},
		},
	}

	t.Run("version pins the tag", func(t *testing.T) {
		steps := in.expandSteps(p, MethodVersion)
		if len(steps) != 2 {
			t.Fatalf("got %d steps, want 2: %v", len(steps), steps)
		}
		want := `git clone --branch 'v1.2.3' --single-branch --depth 1 'https://example.com/tool.git' .`
		if steps[0] != want {
			t.Errorf("clone step = %q, want %q", steps[0], want)
		}
		if steps[1] != "make build" {
			t.Errorf("non-clone step was rewritten: %q", steps[1])
		}
	})

	t.Run("commit clones then checks out", func(t *testing.T) {
		// This is the case the old update path got wrong: it ran the raw step
		// and ended up on the default branch's HEAD.
		steps := in.expandSteps(p, MethodCommit)
		if len(steps) != 3 {
			t.Fatalf("got %d steps, want 3 (clone, checkout, make): %v", len(steps), steps)
		}
		if steps[0] != `git clone 'https://example.com/tool.git' .` {
			t.Errorf("clone step = %q", steps[0])
		}
		if steps[1] != `git checkout 'abcdef1234567890'` {
			t.Errorf("checkout step = %q, want a checkout of the pinned commit", steps[1])
		}
	})

	t.Run("commit method without a commit falls back to the version", func(t *testing.T) {
		noCommit := &Package{
			Version: "v1.2.3",
			Install: Install{
				Source: Source{URL: "https://example.com/tool.git"},
				Steps:  []string{"git clone https://example.com/tool.git ."},
			},
		}
		steps := in.expandSteps(noCommit, MethodCommit)
		if len(steps) != 1 || !strings.Contains(steps[0], "--branch 'v1.2.3'") {
			t.Errorf("steps = %v, want a version clone", steps)
		}
	})

	t.Run("no version and no commit still shallow clones", func(t *testing.T) {
		bare := &Package{
			Install: Install{
				Source: Source{URL: "https://example.com/tool.git"},
				Steps:  []string{"git clone https://example.com/tool.git ."},
			},
		}
		steps := in.expandSteps(bare, MethodVersion)
		if len(steps) != 1 || steps[0] != `git clone --depth 1 'https://example.com/tool.git' .` {
			t.Errorf("steps = %v, want a plain shallow clone", steps)
		}
	})

	t.Run("unknown clone URL leaves the step alone", func(t *testing.T) {
		unknown := &Package{
			Version: "v1.0.0",
			Install: Install{Steps: []string{"git clone"}},
		}
		steps := in.expandSteps(unknown, MethodVersion)
		if len(steps) != 1 || steps[0] != "git clone" {
			t.Errorf("steps = %v, want the original step untouched", steps)
		}
	})
}

func TestRunCommandUsesShellAndDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shell syntax under test is POSIX")
	}

	config := testConfig(t)
	rec := &recorder{}
	in := NewInstaller(config, rec.report)

	dir := t.TempDir()

	// Quoting, && and redirection all have to survive: the old implementation
	// split on whitespace and exec'd argv[0] directly, so none of this worked.
	step := `mkdir -p 'a dir' && printf 'hello world' > 'a dir/file.txt' && cat 'a dir/file.txt'`
	if err := in.runCommand(step, dir, nil); err != nil {
		t.Fatalf("runCommand() error = %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "a dir", "file.txt"))
	if err != nil {
		t.Fatalf("the command did not run in dir: %v", err)
	}
	if string(got) != "hello world" {
		t.Errorf("file contents = %q, want %q", got, "hello world")
	}

	output := rec.texts(EventOutput)
	if len(output) == 0 || output[len(output)-1] != "hello world" {
		t.Errorf("stdout events = %v, want the command output to be streamed", output)
	}
}

func TestRunCommandCapturesStderrAndFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shell syntax under test is POSIX")
	}

	rec := &recorder{}
	in := NewInstaller(testConfig(t), rec.report)

	err := in.runCommand(`echo "to stderr" >&2; exit 3`, t.TempDir(), nil)
	if err == nil {
		t.Fatal("runCommand() error = nil, want a non-zero exit to be reported")
	}

	var sawStderr bool
	for _, line := range rec.texts(EventOutput) {
		if line == "to stderr" {
			sawStderr = true
		}
	}
	if !sawStderr {
		t.Errorf("stderr was not streamed; got %v", rec.texts(EventOutput))
	}
}

func TestRunCommandEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shell syntax under test is POSIX")
	}

	rec := &recorder{}
	in := NewInstaller(testConfig(t), rec.report)

	env := map[string]string{"CLIPACK_TEST_VAR": "from-package"}
	if err := in.runCommand(`printf '%s' "$CLIPACK_TEST_VAR"`, t.TempDir(), env); err != nil {
		t.Fatalf("runCommand() error = %v", err)
	}

	output := rec.texts(EventOutput)
	if len(output) != 1 || output[0] != "from-package" {
		t.Errorf("output = %v, want the package environment to be applied", output)
	}

	// The variable must not leak into the clipack process itself.
	if os.Getenv("CLIPACK_TEST_VAR") != "" {
		t.Error("the package environment leaked into the parent process")
	}
}

// buildablePackage returns a package that builds entirely from shell built-ins,
// so the installer can be exercised end to end without a network or a compiler.
func buildablePackage() *Package {
	return &Package{
		Name:        "demo",
		Version:     "v1.0.0",
		Commit:      "0123456789abcdef",
		Description: "A demo package",
		Install: Install{
			Steps: []string{
				"mkdir -p out man/man1",
				`printf '#!/bin/sh\necho demo\n' > out/demo`,
				`printf 'the man page' > man/man1/demo.1`,
				`printf 'built config' > out/demo.conf`,
			},
			Binaries: []string{"out/demo"},
			Configs:  []string{"out/demo.conf"},
			Man:      []string{"man/man1/demo.1"},
			AdditionalConfig: []AdditionalConfig{
				{Filename: "extra.toml", Content: "key = \"value\"\n"},
				{Filename: "nested/hook.sh", Content: "echo hook\n"},
			},
		},
		PostInstall: PostInstall{
			Scripts: []Script{{Filename: "demo-setup.sh", Content: "echo setup\n"}},
		},
	}
}

func TestInstallEndToEnd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shell syntax under test is POSIX")
	}

	config := testConfig(t)
	rec := &recorder{}
	in := NewInstaller(config, rec.report)

	p := buildablePackage()
	if err := in.Install(p, MethodVersion); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	binary := filepath.Join(config.Paths.Bin, "demo")
	if !exists(binary) {
		t.Error("the binary was not installed")
	}
	if info, err := os.Stat(binary); err == nil && info.Mode().Perm()&0o111 == 0 {
		t.Errorf("binary mode = %v, want it to be executable", info.Mode().Perm())
	}

	for _, want := range []string{
		filepath.Join(config.Paths.Configs, "demo", "demo.conf"),
		filepath.Join(config.Paths.Configs, "demo", "extra.toml"),
		filepath.Join(config.Paths.Configs, "demo", "nested", "hook.sh"),
		filepath.Join(config.Paths.Configs, "demo", "package.yaml"),
		filepath.Join(config.Paths.Man, "man1", "demo.1"),
		filepath.Join(config.Paths.Bin, "demo-setup.sh"),
	} {
		if !exists(want) {
			t.Errorf("missing after install: %s", want)
		}
	}

	// .sh additional-config files are made executable so they can be sourced
	// or run directly.
	if info, err := os.Stat(filepath.Join(config.Paths.Configs, "demo", "nested", "hook.sh")); err == nil {
		if info.Mode().Perm()&0o111 == 0 {
			t.Errorf("hook.sh mode = %v, want it to be executable", info.Mode().Perm())
		}
	}

	// cleanup_build is on, so the source tree must be gone.
	if exists(filepath.Join(config.Paths.Build, "demo")) {
		t.Error("the build directory survived a successful install despite cleanup_build")
	}

	// The manifest records the method actually used, which is what update and
	// remove read later.
	installed, err := InstalledMap(config)
	if err != nil {
		t.Fatal(err)
	}
	if installed["demo"] == nil {
		t.Fatal("the installed package was not recorded")
	}
	if installed["demo"].InstallMethod != MethodVersion {
		t.Errorf("recorded install_method = %q, want version", installed["demo"].InstallMethod)
	}

	// The step counter has to be complete and in order for the progress UI.
	steps := rec.kinds(EventStep)
	if len(steps) != 4 {
		t.Fatalf("got %d step events, want 4", len(steps))
	}
	for i, e := range steps {
		if e.Step != i+1 || e.Total != 4 {
			t.Errorf("step %d reported as %d/%d, want %d/4", i, e.Step, e.Total, i+1)
		}
	}

	if len(rec.kinds(EventDone)) != 1 {
		t.Errorf("got %d done events, want exactly 1", len(rec.kinds(EventDone)))
	}
}

func TestInstallKeepsBuildDirWhenCleanupDisabled(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shell syntax under test is POSIX")
	}

	config := testConfig(t)
	config.Options.CleanupBuild = false
	in := NewInstaller(config, nil)

	if err := in.Install(buildablePackage(), MethodVersion); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if !exists(filepath.Join(config.Paths.Build, "demo")) {
		t.Error("the build directory was removed despite cleanup_build: false")
	}
}

func TestInstallFailingStepReportsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shell syntax under test is POSIX")
	}

	config := testConfig(t)
	in := NewInstaller(config, nil)

	p := buildablePackage()
	p.Install.Steps = []string{"true", "exit 7", "mkdir -p out"}

	err := in.Install(p, MethodVersion)
	if err == nil {
		t.Fatal("Install() error = nil, want the failing step to be reported")
	}
	if !strings.Contains(err.Error(), "step 2/3") {
		t.Errorf("error = %v, want it to identify which step failed", err)
	}
	// The third step must not have run after the second failed.
	if exists(filepath.Join(config.Paths.Build, "demo", "out")) {
		t.Error("steps kept running after a failure")
	}
}

func TestInstallMissingArtifactIsAnError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shell syntax under test is POSIX")
	}

	config := testConfig(t)
	in := NewInstaller(config, nil)

	// The build succeeds but never produces the declared binary. The old code
	// only logged this and still printed "Successfully installed".
	p := &Package{
		Name:    "ghost",
		Version: "v1.0.0",
		Install: Install{
			Steps:    []string{"true"},
			Binaries: []string{"out/ghost"},
		},
	}

	if err := in.Install(p, MethodVersion); err == nil {
		t.Fatal("Install() error = nil, want a missing binary to fail the install")
	}
}

func TestRemove(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shell syntax under test is POSIX")
	}

	config := testConfig(t)
	in := NewInstaller(config, nil)

	p := buildablePackage()
	if err := in.Install(p, MethodVersion); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	installed, err := InstalledMap(config)
	if err != nil {
		t.Fatal(err)
	}

	if err := in.Remove(installed["demo"]); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	for _, gone := range []string{
		filepath.Join(config.Paths.Bin, "demo"),
		filepath.Join(config.Paths.Bin, "demo-setup.sh"),
		filepath.Join(config.Paths.Man, "man1", "demo.1"),
		filepath.Join(config.Paths.Configs, "demo"),
	} {
		if exists(gone) {
			t.Errorf("still present after remove: %s", gone)
		}
	}
}

func TestRemoveIsIdempotent(t *testing.T) {
	config := testConfig(t)
	rec := &recorder{}
	in := NewInstaller(config, rec.report)

	// Nothing was ever installed, so every artifact is already absent. That is
	// not a failure, and it must not be reported as a warning either.
	p := buildablePackage()
	if err := in.Remove(p); err != nil {
		t.Fatalf("Remove() on a package with no artifacts error = %v, want nil", err)
	}
	if warns := rec.texts(EventWarn); len(warns) != 0 {
		t.Errorf("got warnings for already-absent files: %v", warns)
	}
}

func TestRemoveWarnsOnUndeletableArtifact(t *testing.T) {
	config := testConfig(t)
	rec := &recorder{}
	in := NewInstaller(config, rec.report)

	// A non-empty directory where a binary is expected: os.Remove fails with
	// something other than "not found", which has to surface as a warning
	// rather than being swallowed or aborting the removal.
	blocked := filepath.Join(config.Paths.Bin, "demo")
	if err := os.MkdirAll(filepath.Join(blocked, "child"), 0o755); err != nil {
		t.Fatal(err)
	}

	p := &Package{Name: "demo", Install: Install{Binaries: []string{"out/demo"}}}
	if err := in.Remove(p); err != nil {
		t.Fatalf("Remove() error = %v, want the removal to continue", err)
	}

	warns := rec.texts(EventWarn)
	if len(warns) == 0 || !strings.Contains(warns[0], "demo") {
		t.Errorf("warnings = %v, want one about the binary that could not be removed", warns)
	}
	if len(rec.kinds(EventDone)) != 1 {
		t.Error("Remove() did not complete despite the warning")
	}
}

func TestUpdateRemovesRenamedArtifacts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shell syntax under test is POSIX")
	}

	config := testConfig(t)
	in := NewInstaller(config, nil)

	// v1 ships bin/old-name.
	v1 := &Package{
		Name:    "renamer",
		Version: "v1.0.0",
		Install: Install{
			Steps:    []string{"mkdir -p out", "printf x > out/old-name"},
			Binaries: []string{"out/old-name"},
		},
	}
	if err := in.Install(v1, MethodVersion); err != nil {
		t.Fatalf("installing v1: %v", err)
	}
	if !exists(filepath.Join(config.Paths.Bin, "old-name")) {
		t.Fatal("v1 binary was not installed")
	}

	// v2 renames it. The old binary has to go: this is the cleanup that the
	// previous implementation deleted the manifest before it could read.
	v2 := &Package{
		Name:    "renamer",
		Version: "v2.0.0",
		Install: Install{
			Steps:    []string{"mkdir -p out", "printf x > out/new-name"},
			Binaries: []string{"out/new-name"},
		},
	}
	if err := in.Update(v2, MethodVersion); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if exists(filepath.Join(config.Paths.Bin, "old-name")) {
		t.Error("the binary from the previous version was left behind")
	}
	if !exists(filepath.Join(config.Paths.Bin, "new-name")) {
		t.Error("the new binary was not installed")
	}

	installed, err := InstalledMap(config)
	if err != nil {
		t.Fatal(err)
	}
	if installed["renamer"].Version != "v2.0.0" {
		t.Errorf("recorded version = %q, want v2.0.0", installed["renamer"].Version)
	}
}

func TestInstallDoesNotChangeWorkingDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shell syntax under test is POSIX")
	}

	before, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	in := NewInstaller(testConfig(t), nil)
	if err := in.Install(buildablePackage(), MethodVersion); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	after, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// The old code called os.Chdir(buildDir) and never restored it, which broke
	// every relative path used afterwards in the same process.
	if before != after {
		t.Errorf("working directory changed from %q to %q", before, after)
	}
}

func TestStepsInheritTheParentEnvironment(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shell syntax under test is POSIX")
	}

	t.Setenv("CLIPACK_INHERITED", "from-the-shell")

	rec := &recorder{}
	in := NewInstaller(testConfig(t), rec.report)

	// A package that declares no environment of its own still runs with the
	// one clipack was started in. That is what lets a user work around a
	// registry entry that is missing a build flag, by exporting it before
	// launching clipack.
	if err := in.runCommand(`printf '%s' "$CLIPACK_INHERITED"`, t.TempDir(), nil); err != nil {
		t.Fatalf("runCommand() error = %v", err)
	}

	output := rec.texts(EventOutput)
	if len(output) != 1 || output[0] != "from-the-shell" {
		t.Errorf("output = %v, want the inherited value", output)
	}
}
