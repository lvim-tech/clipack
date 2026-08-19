package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExposeCommandLinksABinary(t *testing.T) {
	config := setupCmdTest(t)
	installManifest(t, config, demoPackage())

	stdout, _, err := execute(t, "expose", "demo")
	if err != nil {
		t.Fatalf("expose demo error = %v", err)
	}

	link := filepath.Join(config.Paths.Expose, "demo")
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("reading %s: %v", link, err)
	}
	if want := filepath.Join(config.Paths.Bin, "demo"); target != want {
		t.Errorf("the link points at %q, want %q", target, want)
	}
	if !strings.Contains(stdout, link) {
		t.Errorf("output does not say where the link is:\n%s", stdout)
	}

	// Running it again is not an error and does not change anything.
	if _, _, err := execute(t, "expose", "demo"); err != nil {
		t.Errorf("exposing an already exposed binary error = %v", err)
	}
}

func TestExposeCommandRejectsAnUnknownBinary(t *testing.T) {
	config := setupCmdTest(t)
	installManifest(t, config, demoPackage())

	_, _, err := execute(t, "expose", "demo", "dmeo")
	if err == nil {
		t.Fatal("expose demo dmeo error = nil, want a refusal")
	}
	if !strings.Contains(err.Error(), "dmeo") {
		t.Errorf("error = %v, want it to name the binary that does not exist", err)
	}
	if _, lerr := os.Lstat(filepath.Join(config.Paths.Expose, "dmeo")); lerr == nil {
		t.Error("a link was made for a binary the package does not install")
	}
}

func TestExposeCommandRejectsAPackageThatIsNotInstalled(t *testing.T) {
	setupCmdTest(t)

	_, _, err := execute(t, "expose", "demo")
	if err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Errorf("error = %v, want it to say the package is not installed", err)
	}
}

func TestUnexposeCommandRemovesTheLink(t *testing.T) {
	config := setupCmdTest(t)
	installManifest(t, config, demoPackage())

	if _, _, err := execute(t, "expose", "demo"); err != nil {
		t.Fatalf("expose demo error = %v", err)
	}
	if _, _, err := execute(t, "unexpose", "demo"); err != nil {
		t.Fatalf("unexpose demo error = %v", err)
	}

	if _, err := os.Lstat(filepath.Join(config.Paths.Expose, "demo")); err == nil {
		t.Error("the link outlived the unexpose")
	}
}

func TestExposeCommandListsWhatIsExposed(t *testing.T) {
	config := setupCmdTest(t)
	installManifest(t, config, demoPackage())

	stdout, _, err := execute(t, "expose")
	if err != nil {
		t.Fatalf("expose error = %v", err)
	}
	if !strings.Contains(stdout, "Nothing is exposed") {
		t.Errorf("the empty inventory does not say so:\n%s", stdout)
	}

	if _, _, err := execute(t, "expose", "demo"); err != nil {
		t.Fatalf("expose demo error = %v", err)
	}

	stdout, _, err = execute(t, "expose")
	if err != nil {
		t.Fatalf("expose error = %v", err)
	}
	for _, want := range []string{"PACKAGE", "demo", filepath.Join(config.Paths.Expose, "demo")} {
		if !strings.Contains(stdout, want) {
			t.Errorf("the inventory is missing %q:\n%s", want, stdout)
		}
	}
}
