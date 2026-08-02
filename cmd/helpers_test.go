package cmd

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/lvim-tech/clipack/cnfg"
	"github.com/lvim-tech/clipack/pkg"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// setupCmdTest points HOME at a temporary directory, writes a configuration
// there and returns it. Nothing touches the user's real installation.
func setupCmdTest(t *testing.T) *cnfg.Config {
	t.Helper()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("SHELL", "/bin/bash")

	config := cnfg.NewDefaultConfig(filepath.Join(t.TempDir(), "packages"))
	if err := config.Save(); err != nil {
		t.Fatalf("writing the test configuration: %v", err)
	}
	return config
}

// seedCache writes packages straight into the registry cache, so the commands
// resolve them without any network access.
func seedCache(t *testing.T, config *cnfg.Config, packages ...*pkg.Package) {
	t.Helper()

	if err := pkg.SaveToCache(packages, config); err != nil {
		t.Fatalf("seeding the registry cache: %v", err)
	}
}

// installManifest records a package as installed by writing its manifest.
func installManifest(t *testing.T, config *cnfg.Config, p *pkg.Package) {
	t.Helper()

	dir := filepath.Join(config.Paths.Configs, p.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	data, err := yaml.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	// The manifest alone is not an install: clipack now checks that what it
	// describes is on disk, so the fixture has to put the binaries there too.
	// Use installBrokenManifest for the case where they are deliberately absent.
	for _, bin := range p.Install.Binaries {
		target := filepath.Join(config.Paths.Bin, filepath.Base(bin))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// demoPackage is a package that "builds" from shell built-ins, so an install
// can be exercised without a compiler or a network.
func demoPackage() *pkg.Package {
	return &pkg.Package{
		Name:        "demo",
		Version:     "v1.0.0",
		Commit:      "0123456789abcdef",
		Description: "A demo package for tests",
		Category:    "cli",
		Maintainer:  "clipack",
		License:     "MIT",
		Install: pkg.Install{
			Steps:    []string{"mkdir -p out", `printf 'demo binary' > out/demo`},
			Binaries: []string{"out/demo"},
		},
	}
}

// otherPackage is a second registry entry, used to check filtering and counts.
func otherPackage() *pkg.Package {
	return &pkg.Package{
		Name:        "other",
		Version:     "v2.0.0",
		Description: "Another package\nwith a second line",
		Category:    "file_managers",
	}
}

// resetFlags restores every command flag to its default. Cobra binds flags to
// package-level variables that survive between Execute calls, so without this a
// flag set by one test would leak into the next.
func resetFlags() {
	installForceRefresh, installMethod, installYes = false, "", false
	updateForceRefresh, updateAll, updateYes = false, false, false
	removeYes = false
	listForceRefresh, listInstalled, listUpdates = false, false, false
	previewForceRefresh = false
	themeShowColors = false
}

// execute runs the root command with the given arguments and returns whatever
// it wrote to stdout and stderr along with the error.
func execute(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()

	resetFlags()
	t.Cleanup(resetFlags)

	rootCmd.SetArgs(args)
	t.Cleanup(func() { rootCmd.SetArgs(nil) })

	stdout, stderr = capture(t, func() {
		err = rootCmd.Execute()
	})
	return stdout, stderr, err
}

// capture redirects os.Stdout and os.Stderr for the duration of fn. Both pipes
// are drained concurrently, so output larger than the pipe buffer cannot
// deadlock the test.
func capture(t *testing.T, fn func()) (string, string) {
	t.Helper()

	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW

	outC := make(chan string, 1)
	errC := make(chan string, 1)
	go func() { b, _ := io.ReadAll(outR); outC <- string(b) }()
	go func() { b, _ := io.ReadAll(errR); errC <- string(b) }()

	defer func() {
		os.Stdout, os.Stderr = oldOut, oldErr
		outW.Close()
		errW.Close()
	}()

	fn()

	os.Stdout, os.Stderr = oldOut, oldErr
	outW.Close()
	errW.Close()

	return <-outC, <-errC
}

// withStdin replaces os.Stdin with a file holding the given input, so the
// confirmation prompts can be answered.
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

// findCommand returns a registered subcommand by name.
func findCommand(t *testing.T, name string) *cobra.Command {
	t.Helper()

	for _, c := range rootCmd.Commands() {
		if c.Name() == name {
			return c
		}
	}
	t.Fatalf("command %q is not registered", name)
	return nil
}

// exists reports whether a path is present.
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// installBrokenManifest records a package as installed without creating any of
// the artifacts it declares — a manifest that outlived its binaries, which is
// what a deleted or replaced binary leaves behind.
func installBrokenManifest(t *testing.T, config *cnfg.Config, p *pkg.Package) {
	t.Helper()

	installManifest(t, config, p)
	for _, bin := range p.Install.Binaries {
		if err := os.Remove(filepath.Join(config.Paths.Bin, filepath.Base(bin))); err != nil {
			t.Fatal(err)
		}
	}
}
