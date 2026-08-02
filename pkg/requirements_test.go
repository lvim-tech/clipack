package pkg

import (
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestRequirementsForAddsTheMethodToTheSharedSet is the shape the registry is
// written in: what both refs need is stated once, and only the difference is
// repeated. ghostty is the package that earned it — its tag and its main branch
// want different compilers.
func TestRequirementsForAddsTheMethodToTheSharedSet(t *testing.T) {
	r := Requirements{
		MethodRequirements: MethodRequirements{
			OpenSUSE:  []string{"gtk4-devel", "libadwaita-devel"},
			Toolchain: []string{"pkg-config"},
		},
		Version: MethodRequirements{Toolchain: []string{"zig == 0.15.2"}},
		Commit:  MethodRequirements{Toolchain: []string{"zig >= 0.16"}},
	}

	version := r.For(MethodVersion)
	if want := []string{"pkg-config", "zig == 0.15.2"}; !reflect.DeepEqual(version.Toolchain, want) {
		t.Errorf("For(version).Toolchain = %v, want %v", version.Toolchain, want)
	}
	if want := []string{"gtk4-devel", "libadwaita-devel"}; !reflect.DeepEqual(version.OpenSUSE, want) {
		t.Errorf("For(version).OpenSUSE = %v, want the shared list", version.OpenSUSE)
	}

	commit := r.For(MethodCommit)
	if want := []string{"pkg-config", "zig >= 0.16"}; !reflect.DeepEqual(commit.Toolchain, want) {
		t.Errorf("For(commit).Toolchain = %v, want %v", commit.Toolchain, want)
	}
}

func TestRequirementsForDropsDuplicates(t *testing.T) {
	r := Requirements{
		MethodRequirements: MethodRequirements{OpenSUSE: []string{"cmake", "pkg-config"}},
		Commit:             MethodRequirements{OpenSUSE: []string{"pkg-config", "ninja"}},
	}

	got := r.For(MethodCommit).OpenSUSE
	if want := []string{"cmake", "pkg-config", "ninja"}; !reflect.DeepEqual(got, want) {
		t.Errorf("For(commit).OpenSUSE = %v, want %v with the repeat dropped", got, want)
	}
}

func TestRequirementsEmpty(t *testing.T) {
	var r Requirements
	if !r.For(MethodVersion).Empty() {
		t.Error("a package with no requirements reported some")
	}
	if (MethodRequirements{Toolchain: []string{"go"}}).Empty() {
		t.Error("Empty() = true with a toolchain listed")
	}
}

// TestZypperCommandIsReadyToPaste is the whole point of rendering it rather than
// listing names: it has to work when copied into a terminal unchanged.
func TestZypperCommandIsReadyToPaste(t *testing.T) {
	got := ZypperCommand([]string{"gtk4-devel", "libadwaita-devel", "blueprint-compiler"})

	if want := "sudo zypper in gtk4-devel libadwaita-devel blueprint-compiler"; got != want {
		t.Errorf("ZypperCommand() = %q, want %q", got, want)
	}
	if strings.Contains(got, "  ") {
		t.Errorf("ZypperCommand() = %q, want no double spaces", got)
	}
	// An empty list must not produce a command that installs nothing.
	if got := ZypperCommand(nil); got != "" {
		t.Errorf("ZypperCommand(nil) = %q, want an empty string", got)
	}
}

// TestRequirementsRoundTripThroughYAML pins the registry syntax: the shared set
// is inline and the two methods are nested under their own keys.
func TestRequirementsRoundTripThroughYAML(t *testing.T) {
	const source = `
requirements:
  opensuse:
    - gtk4-devel
  toolchain:
    - "pkg-config"
  commit:
    toolchain:
      - "zig >= 0.16"
install:
  steps:
    - zig build
`

	var p Package
	if err := yaml.Unmarshal([]byte(source), &p); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if want := []string{"gtk4-devel"}; !reflect.DeepEqual(p.Requirements.OpenSUSE, want) {
		t.Errorf("OpenSUSE = %v, want %v", p.Requirements.OpenSUSE, want)
	}
	if want := []string{"pkg-config", "zig >= 0.16"}; !reflect.DeepEqual(p.Requirements.For(MethodCommit).Toolchain, want) {
		t.Errorf("For(commit).Toolchain = %v, want %v", p.Requirements.For(MethodCommit).Toolchain, want)
	}
	if got := p.Requirements.For(MethodVersion).Toolchain; !reflect.DeepEqual(got, []string{"pkg-config"}) {
		t.Errorf("For(version).Toolchain = %v, want only the shared entry", got)
	}
}

// TestRequirementsAreOptional guards every entry written before the field
// existed: they have to keep parsing, and report nothing rather than crash.
func TestRequirementsAreOptional(t *testing.T) {
	var p Package
	if err := yaml.Unmarshal([]byte("name: bat\ninstall:\n  steps:\n    - cargo build\n"), &p); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if !p.Requirements.For(MethodVersion).Empty() {
		t.Error("a package without the field reported requirements")
	}
}
