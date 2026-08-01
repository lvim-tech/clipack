package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/lvim-tech/clipack/pkg"
)

func TestShortCommit(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"3ae767327436c59fead00e99126c237a071a6c3e", "3ae767327436"},
		{"abc123", "abc123"},
		{"", ""},
		{"123456789012", "123456789012"}, // exactly the limit, untouched
	}

	for _, tt := range tests {
		if got := shortCommit(tt.in); got != tt.want {
			t.Errorf("shortCommit(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRenderDetail(t *testing.T) {
	entry := packageItem{pkg: &pkg.Package{
		Name:        "bat",
		Version:     "v0.25.0",
		Commit:      "b7f9662097d7e1476d68cd4035f8ce4602dd57e0",
		Description: "A cat clone with syntax highlighting",
		Category:    "cli",
		Maintainer:  "sharkdp",
		License:     "MIT",
		Homepage:    "https://github.com/sharkdp/bat",
		UpdatedAt:   time.Date(2025, 5, 6, 1, 27, 56, 0, time.UTC),
		Tags:        []string{"cat", "syntax-highlighting"},
		Install: pkg.Install{
			Steps:            []string{"git clone https://example.com/bat.git .", "cargo build --release"},
			Binaries:         []string{"target/release/bat"},
			AdditionalConfig: []pkg.AdditionalConfig{{Filename: "config.sh"}},
		},
	}}

	out := renderDetail(entry, 60, DefaultStyles())

	for _, want := range []string{
		"bat",
		"v0.25.0",
		"A cat clone with syntax highlighting",
		"cli",
		"sharkdp",
		"MIT",
		"github.com/sharkdp/bat",
		"b7f9662097d7", // the commit is shortened for display
		"2025-05-06",
		"cat, syntax-highlighting",
		"Build steps",
		"cargo build --release",
		"Binaries",
		"target/release/bat",
		"Config files",
		"config.sh",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("renderDetail() is missing %q", want)
		}
	}

	// The full SHA must not appear; only the shortened form.
	if strings.Contains(out, "b7f9662097d7e1476d68cd4035f8ce4602dd57e0") {
		t.Error("renderDetail() printed the full commit SHA")
	}
}

func TestRenderDetailOmitsEmptyFields(t *testing.T) {
	entry := packageItem{pkg: &pkg.Package{Name: "minimal"}}

	out := renderDetail(entry, 60, DefaultStyles())

	// A package with no metadata must not print empty rows or section headers.
	for _, unwanted := range []string{"License", "Homepage", "Maintainer", "Build steps", "Binaries"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("renderDetail() printed the %q row for an empty field", unwanted)
		}
	}
	if !strings.Contains(out, "minimal") {
		t.Error("renderDetail() is missing the package name")
	}
}

func TestRenderDetailWrapsToWidth(t *testing.T) {
	entry := packageItem{pkg: &pkg.Package{
		Name:        "verbose",
		Description: strings.Repeat("long description ", 20),
	}}

	const width = 50
	out := renderDetail(entry, width, DefaultStyles())

	for _, line := range strings.Split(out, "\n") {
		if got := len([]rune(line)); got > width {
			t.Errorf("line is %d cells wide, want at most %d: %q", got, width, line)
		}
	}
}

func TestRenderDetailClampsTinyWidth(t *testing.T) {
	entry := packageItem{pkg: &pkg.Package{Name: "bat", Description: "A cat clone"}}

	// A very narrow pane must not panic or produce negative-width styles.
	for _, width := range []int{-10, 0, 1, 5, 19} {
		if out := renderDetail(entry, width, DefaultStyles()); out == "" {
			t.Errorf("renderDetail(width=%d) returned nothing", width)
		}
	}
}

func TestRenderStatus(t *testing.T) {
	registry := &pkg.Package{Name: "bat", Version: "v2.0.0", Commit: "newcommit123456"}

	t.Run("not installed", func(t *testing.T) {
		out := renderStatus(packageItem{pkg: registry}, DefaultStyles())
		if !strings.Contains(out, "Not installed") {
			t.Errorf("renderStatus() = %q, want it to say the package is not installed", out)
		}
	})

	t.Run("installed and current", func(t *testing.T) {
		entry := packageItem{
			pkg:       registry,
			installed: &pkg.Package{Version: "v2.0.0", InstallMethod: pkg.MethodVersion},
		}
		out := renderStatus(entry, DefaultStyles())
		if !strings.Contains(out, "Installed") {
			t.Errorf("renderStatus() = %q, want it to say installed", out)
		}
		if strings.Contains(out, "Update available") {
			t.Errorf("renderStatus() = %q, want no update notice for a current package", out)
		}
	})

	t.Run("update available", func(t *testing.T) {
		entry := packageItem{
			pkg:       registry,
			installed: &pkg.Package{Version: "v1.0.0", InstallMethod: pkg.MethodVersion},
		}
		out := renderStatus(entry, DefaultStyles())
		if !strings.Contains(out, "Update available") {
			t.Errorf("renderStatus() = %q, want an update notice", out)
		}
		if !strings.Contains(out, "v2.0.0") {
			t.Errorf("renderStatus() = %q, want the available version", out)
		}
	})

	t.Run("commit method shortens both refs", func(t *testing.T) {
		entry := packageItem{
			pkg:       registry,
			installed: &pkg.Package{Commit: "oldcommit098765432", InstallMethod: pkg.MethodCommit},
		}
		out := renderStatus(entry, DefaultStyles())
		if !strings.Contains(out, "oldcommit098") {
			t.Errorf("renderStatus() = %q, want the shortened installed commit", out)
		}
		if strings.Contains(out, "oldcommit098765432") {
			t.Error("renderStatus() printed the full installed SHA")
		}
	})

	t.Run("missing method defaults to version", func(t *testing.T) {
		entry := packageItem{
			pkg:       registry,
			installed: &pkg.Package{Version: "v1.0.0"},
		}
		out := renderStatus(entry, DefaultStyles())
		if !strings.Contains(out, pkg.MethodVersion) {
			t.Errorf("renderStatus() = %q, want it to fall back to the version method", out)
		}
	})
}
