package pkg

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// desktopPackage builds a package that ships a menu entry and an icon, the way
// a graphical program does.
func desktopPackage() *Package {
	p := buildablePackage()
	p.Install.Steps = append(p.Install.Steps,
		"mkdir -p out/share/applications out/share/icons",
		`printf '[Desktop Entry]\nType=Application\nName=demo\nExec=demo --flag\nTryExec=demo\nIcon=demo\n' > out/share/applications/demo.desktop`,
		`printf 'PNG' > out/share/icons/demo.png`,
	)
	p.Install.Desktop = []DesktopEntry{{
		Source: "out/share/applications/demo.desktop",
		Icon:   "out/share/icons/demo.png",
	}}
	return p
}

func TestInstallWritesTheDesktopEntry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shell syntax under test is POSIX")
	}

	withDataHome(t)
	config := testConfig(t)
	in := NewInstaller(config, nil)

	if err := in.Install(desktopPackage(), MethodVersion); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	entry, iconDir, err := desktopPaths("demo", "out/share/applications/demo.desktop")
	if err != nil {
		t.Fatal(err)
	}

	contents, err := os.ReadFile(entry)
	if err != nil {
		t.Fatalf("the desktop entry was not installed: %v", err)
	}
	got := string(contents)

	// Exec has to name the binary clipack installed. Left as "demo" the entry
	// would launch whatever else on PATH answers to that name.
	wantExec := filepath.Join(config.Paths.Bin, "demo")
	if !strings.Contains(got, "Exec="+wantExec+" --flag") {
		t.Errorf("entry = %q, want Exec pinned to %q", got, wantExec)
	}
	if !strings.Contains(got, "Name=demo (clipack)") {
		t.Errorf("entry = %q, want the name distinguished", got)
	}

	icon := filepath.Join(iconDir, "demo.png")
	if _, err := os.Stat(icon); err != nil {
		t.Errorf("the icon was not installed: %v", err)
	}
	if !strings.Contains(got, "Icon="+icon) {
		t.Errorf("entry = %q, want Icon pointing at the installed file", got)
	}
}

func TestRemoveDeletesTheDesktopEntry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shell syntax under test is POSIX")
	}

	withDataHome(t)
	config := testConfig(t)
	in := NewInstaller(config, nil)

	p := desktopPackage()
	if err := in.Install(p, MethodVersion); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	entry, iconDir, err := desktopPaths("demo", "out/share/applications/demo.desktop")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(entry); err != nil {
		t.Fatalf("nothing to remove: %v", err)
	}

	if err := in.Remove(p); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	if _, err := os.Stat(entry); !os.IsNotExist(err) {
		t.Errorf("stat entry = %v, want the menu entry gone", err)
	}
	if _, err := os.Stat(iconDir); !os.IsNotExist(err) {
		t.Errorf("stat icon directory = %v, want it gone", err)
	}
}

// TestInstallSurvivesAMissingDesktopEntry pins the severity: a program that is
// installed and runnable but absent from the menu is a working install, so a
// desktop file the build did not produce is a warning and not a failure.
func TestInstallSurvivesAMissingDesktopEntry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the shell syntax under test is POSIX")
	}

	withDataHome(t)
	config := testConfig(t)
	rec := &recorder{}
	in := NewInstaller(config, rec.report)

	p := buildablePackage()
	p.Install.Desktop = []DesktopEntry{{Source: "out/share/applications/nonexistent.desktop"}}

	if err := in.Install(p, MethodVersion); err != nil {
		t.Errorf("Install() error = %v, want a missing desktop entry tolerated", err)
	}
	if !exists(filepath.Join(config.Paths.Bin, "demo")) {
		t.Error("the binary was not installed")
	}
}

// TestRemoveCannotReachAForeignEntry guards the one destructive edge: the
// application directory is shared with the distribution, and a manifest is data
// that can be wrong. The protection is that desktopPaths derives the file name
// instead of taking it — drop the prefix there and this test deletes a file
// clipack never wrote.
func TestRemoveCannotReachAForeignEntry(t *testing.T) {
	withDataHome(t)
	config := testConfig(t)
	in := NewInstaller(config, nil)

	base, err := dataHome()
	if err != nil {
		t.Fatal(err)
	}
	apps := filepath.Join(base, "applications")
	if err := os.MkdirAll(apps, 0o755); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(apps, "someone-elses.desktop")
	if err := os.WriteFile(foreign, []byte("[Desktop Entry]\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// A source that resolves onto the foreign file only if the prefix is ignored.
	p := buildablePackage()
	p.Install.Desktop = []DesktopEntry{{Source: "someone-elses.desktop"}}
	in.removeDesktopEntries(p)

	if _, err := os.Stat(foreign); err != nil {
		t.Errorf("stat foreign entry = %v, want it left alone", err)
	}
}
