package pkg

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/lvim-tech/clipack/cnfg"
)

// exposablePackage is the buildable demo package with its binary exposed.
func exposablePackage() *Package {
	p := buildablePackage()
	p.Install.Expose = []string{"demo"}
	return p
}

// linkTarget reads a symlink, failing the test when the path is not one.
func linkTarget(t *testing.T, link string) string {
	t.Helper()

	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("%s does not exist: %v", link, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is not a symlink", link)
	}
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatalf("reading %s: %v", link, err)
	}
	return target
}

// exposeOnPath puts the configuration's expose directory on PATH, so the tests
// that are about the links themselves are not also about a warning that the
// directory nothing has heard of is not on PATH.
func exposeOnPath(t *testing.T, config *cnfg.Config) {
	t.Helper()
	t.Setenv("PATH", config.Paths.Expose)
}

// skipOnWindows guards the tests that build through a POSIX shell.
func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the shell syntax under test is POSIX")
	}
}

func TestExposeNames(t *testing.T) {
	p := &Package{
		Install: Install{
			Binaries: []string{"out/tmux", "out/tmate"},
			Expose:   []string{"tmux"},
		},
		Exposed:   []string{"tmate", "tmux"}, // tmux is already declared
		Unexposed: nil,
	}

	// The declared name comes first and the duplicate does not appear twice.
	if got := p.ExposeNames(); strings.Join(got, ",") != "tmux,tmate" {
		t.Errorf("ExposeNames() = %v, want [tmux tmate]", got)
	}

	// Unexposing subtracts from the declared set, which is what makes the
	// decision survive the next rebuild.
	p.Unexposed = []string{"tmux"}
	if got := p.ExposeNames(); strings.Join(got, ",") != "tmate" {
		t.Errorf("ExposeNames() with tmux unexposed = %v, want [tmate]", got)
	}
}

func TestBinaryNamesAndUnknownExpose(t *testing.T) {
	p := &Package{
		Install:     Install{Binaries: []string{"target/release/demo", "out/demo-helper"}},
		PostInstall: PostInstall{Scripts: []Script{{Filename: "demo-setup.sh"}}},
	}

	got := p.BinaryNames()
	want := []string{"demo", "demo-helper", "demo-setup.sh"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("BinaryNames() = %v, want %v", got, want)
	}

	// A name the package does not install is reported rather than skipped: it
	// is a typo, and skipping it silently is how it stays one.
	unknown := UnknownExpose(p, []string{"demo", "dmeo"})
	if len(unknown) != 1 || unknown[0] != "dmeo" {
		t.Errorf("UnknownExpose() = %v, want [dmeo]", unknown)
	}
}

func TestInstallExposesDeclaredBinary(t *testing.T) {
	skipOnWindows(t)

	config := testConfig(t)
	in := NewInstaller(config, nil)

	if err := in.Install(exposablePackage(), MethodVersion); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	link := filepath.Join(config.Paths.Expose, "demo")
	want := filepath.Join(config.Paths.Bin, "demo")
	if got := linkTarget(t, link); got != want {
		t.Errorf("%s points at %q, want %q", link, got, want)
	}

	// Nothing else the package installs is linked: exposing is per binary, and
	// the post-install script was not asked for.
	if _, err := os.Lstat(filepath.Join(config.Paths.Expose, "demo-setup.sh")); err == nil {
		t.Error("a binary that was not exposed was linked anyway")
	}
}

func TestInstallWithoutExposeLinksNothing(t *testing.T) {
	skipOnWindows(t)

	config := testConfig(t)
	in := NewInstaller(config, nil)

	// The whole point of the field being optional: a registry entry written
	// before it existed keeps behaving exactly as it did.
	if err := in.Install(buildablePackage(), MethodVersion); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if _, err := os.Stat(config.Paths.Expose); err == nil {
		t.Errorf("%s was created even though nothing is exposed", config.Paths.Expose)
	}
}

func TestExposeIsIdempotentAndAdoptsAManualLink(t *testing.T) {
	config := testConfig(t)
	exposeOnPath(t, config)
	in := NewInstaller(config, nil)
	paths := in.pathsFor("demo")

	link := filepath.Join(config.Paths.Expose, "demo")
	target := filepath.Join(config.Paths.Bin, "demo")

	// The link the user made by hand, before clipack knew how to make it.
	if err := os.MkdirAll(config.Paths.Expose, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	before, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}

	rec := &recorder{}
	in.Report = rec.report
	in.applyExpose(exposablePackage(), paths)

	if got := linkTarget(t, link); got != target {
		t.Errorf("the adopted link points at %q, want %q", got, target)
	}
	if len(rec.kinds(EventWarn)) != 0 {
		t.Errorf("adopting an identical link warned: %v", rec.texts(EventWarn))
	}
	if len(rec.kinds(EventInfo)) != 0 {
		t.Errorf("adopting an identical link was announced: %v", rec.texts(EventInfo))
	}

	after, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Error("an identical link was recreated instead of adopted")
	}

	// Applying it again changes nothing either.
	in.applyExpose(exposablePackage(), paths)
	if got := linkTarget(t, link); got != target {
		t.Errorf("after a second apply the link points at %q, want %q", got, target)
	}
}

func TestExposeRepairsAStaleLink(t *testing.T) {
	config := testConfig(t)
	exposeOnPath(t, config)
	in := NewInstaller(config, nil)
	rec := &recorder{}
	in.Report = rec.report

	link := filepath.Join(config.Paths.Expose, "demo")
	stale := filepath.Join(config.Paths.Bin, "demo-old")
	if err := os.MkdirAll(config.Paths.Expose, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(stale, link); err != nil {
		t.Fatal(err)
	}

	in.applyExpose(exposablePackage(), in.pathsFor("demo"))

	want := filepath.Join(config.Paths.Bin, "demo")
	if got := linkTarget(t, link); got != want {
		t.Errorf("the stale link points at %q, want it repaired to %q", got, want)
	}
	if len(rec.kinds(EventWarn)) != 0 {
		t.Errorf("repairing a link into clipack's own bin directory warned: %v", rec.texts(EventWarn))
	}
}

func TestExposeLeavesAForeignFileAlone(t *testing.T) {
	config := testConfig(t)
	in := NewInstaller(config, nil)
	rec := &recorder{}
	in.Report = rec.report

	link := filepath.Join(config.Paths.Expose, "demo")
	if err := os.MkdirAll(config.Paths.Expose, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(link, []byte("#!/bin/sh\necho not clipack\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	in.applyExpose(exposablePackage(), in.pathsFor("demo"))

	contents, err := os.ReadFile(link)
	if err != nil {
		t.Fatalf("the foreign file is gone: %v", err)
	}
	if !strings.Contains(string(contents), "not clipack") {
		t.Error("the foreign file was replaced by a link")
	}
	if len(rec.kinds(EventWarn)) == 0 {
		t.Error("refusing to expose over a foreign file said nothing")
	}
}

func TestExposeLeavesAForeignLinkAlone(t *testing.T) {
	config := testConfig(t)
	in := NewInstaller(config, nil)
	rec := &recorder{}
	in.Report = rec.report

	elsewhere := filepath.Join(t.TempDir(), "demo")
	if err := os.WriteFile(elsewhere, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(config.Paths.Expose, "demo")
	if err := os.MkdirAll(config.Paths.Expose, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, link); err != nil {
		t.Fatal(err)
	}

	in.applyExpose(exposablePackage(), in.pathsFor("demo"))

	if got := linkTarget(t, link); got != elsewhere {
		t.Errorf("a link to another program was repointed to %q", got)
	}
	if len(rec.kinds(EventWarn)) == 0 {
		t.Error("refusing to expose over somebody else's link said nothing")
	}
}

func TestExposeWarnsAboutABinaryThePackageDoesNotInstall(t *testing.T) {
	config := testConfig(t)
	in := NewInstaller(config, nil)
	rec := &recorder{}
	in.Report = rec.report

	p := buildablePackage()
	p.Install.Expose = []string{"dmeo"}
	in.applyExpose(p, in.pathsFor(p.Name))

	warnings := strings.Join(rec.texts(EventWarn), "\n")
	if !strings.Contains(warnings, "dmeo") {
		t.Errorf("warnings = %q, want the unknown name named", warnings)
	}
	if _, err := os.Lstat(filepath.Join(config.Paths.Expose, "dmeo")); err == nil {
		t.Error("a link was made for a binary the package does not install")
	}
}

func TestRemoveDropsOnlyItsOwnLinks(t *testing.T) {
	skipOnWindows(t)

	config := testConfig(t)
	in := NewInstaller(config, nil)

	p := exposablePackage()
	if err := in.Install(p, MethodVersion); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	// A second name in the same directory, owned by something else entirely.
	foreign := filepath.Join(config.Paths.Expose, "other")
	if err := os.WriteFile(foreign, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	installed, err := in.readManifest(in.pathsFor(p.Name))
	if err != nil {
		t.Fatalf("reading the manifest: %v", err)
	}
	if err := in.Remove(installed); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}

	if _, err := os.Lstat(filepath.Join(config.Paths.Expose, "demo")); err == nil {
		t.Error("the exposed link outlived the package")
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Errorf("an unrelated file in the expose directory was removed: %v", err)
	}
}

func TestRemoveKeepsAForeignFileOfTheSameName(t *testing.T) {
	config := testConfig(t)
	in := NewInstaller(config, nil)
	rec := &recorder{}
	in.Report = rec.report

	// The manifest says demo is exposed, but the name is held by something
	// clipack did not put there. Removing the package must not remove it.
	link := filepath.Join(config.Paths.Expose, "demo")
	if err := os.MkdirAll(config.Paths.Expose, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(link, []byte("distribution's demo\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	in.removeExposed(exposablePackage(), in.pathsFor("demo"))

	if _, err := os.Stat(link); err != nil {
		t.Errorf("a file clipack did not create was removed: %v", err)
	}
}

func TestExposeAdHocSurvivesAnUpdate(t *testing.T) {
	skipOnWindows(t)

	config := testConfig(t)
	in := NewInstaller(config, nil)

	// Installed without any expose at all — the ordinary case.
	if err := in.Install(buildablePackage(), MethodVersion); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	paths := in.pathsFor("demo")

	installed, err := in.readManifest(paths)
	if err != nil {
		t.Fatal(err)
	}
	if err := in.Expose(installed, []string{"demo"}); err != nil {
		t.Fatalf("Expose() error = %v", err)
	}

	link := filepath.Join(config.Paths.Expose, "demo")
	if got := linkTarget(t, link); got != filepath.Join(config.Paths.Bin, "demo") {
		t.Fatalf("Expose() left the link pointing at %q", got)
	}

	// The decision is state, so it has to be in the manifest — that is what a
	// rebuild reads.
	stored, err := in.readManifest(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.Exposed) != 1 || stored.Exposed[0] != "demo" {
		t.Fatalf("manifest exposed = %v, want [demo]", stored.Exposed)
	}

	// The registry entry knows nothing about it, and the rebuild must still
	// end up with the link.
	if err := in.Update(buildablePackage(), MethodVersion); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if got := linkTarget(t, link); got != filepath.Join(config.Paths.Bin, "demo") {
		t.Errorf("after the update the link points at %q, want the binary", got)
	}

	rebuilt, err := in.readManifest(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(rebuilt.Exposed) != 1 || rebuilt.Exposed[0] != "demo" {
		t.Errorf("after the update manifest exposed = %v, want [demo]", rebuilt.Exposed)
	}
}

func TestExposeRejectsABinaryThePackageDoesNotInstall(t *testing.T) {
	config := testConfig(t)
	in := NewInstaller(config, nil)

	p := buildablePackage()
	err := in.Expose(p, []string{"dmeo"})
	if err == nil {
		t.Fatal("Expose() error = nil, want a refusal")
	}
	// The message has to name both the typo and what could have been meant.
	if !strings.Contains(err.Error(), "dmeo") || !strings.Contains(err.Error(), "demo") {
		t.Errorf("error = %v, want it to name the unknown binary and the real ones", err)
	}
}

func TestExposeWithoutNamesTakesEveryBinary(t *testing.T) {
	skipOnWindows(t)

	config := testConfig(t)
	in := NewInstaller(config, nil)
	if err := in.Install(buildablePackage(), MethodVersion); err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	installed, err := in.readManifest(in.pathsFor("demo"))
	if err != nil {
		t.Fatal(err)
	}
	if err := in.Expose(installed, nil); err != nil {
		t.Fatalf("Expose() error = %v", err)
	}

	for _, name := range []string{"demo", "demo-setup.sh"} {
		if _, err := os.Lstat(filepath.Join(config.Paths.Expose, name)); err != nil {
			t.Errorf("%s was not exposed: %v", name, err)
		}
	}
}

func TestUnexposeOfADeclaredBinarySticks(t *testing.T) {
	skipOnWindows(t)

	config := testConfig(t)
	in := NewInstaller(config, nil)

	if err := in.Install(exposablePackage(), MethodVersion); err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	paths := in.pathsFor("demo")
	link := filepath.Join(config.Paths.Expose, "demo")

	installed, err := in.readManifest(paths)
	if err != nil {
		t.Fatal(err)
	}
	if err := in.Unexpose(installed, []string{"demo"}); err != nil {
		t.Fatalf("Unexpose() error = %v", err)
	}
	if _, err := os.Lstat(link); err == nil {
		t.Fatal("the link is still there after unexposing")
	}

	stored, err := in.readManifest(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.Unexposed) != 1 || stored.Unexposed[0] != "demo" {
		t.Fatalf("manifest unexposed = %v, want [demo]", stored.Unexposed)
	}

	// Without the record, the next rebuild would put the registry's own link
	// straight back, and unexposing would look broken.
	if err := in.Update(exposablePackage(), MethodVersion); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if _, err := os.Lstat(link); err == nil {
		t.Error("the rebuild restored a link that was unexposed by hand")
	}
}

func TestExposeStatusesReportTheLink(t *testing.T) {
	config := testConfig(t)
	p := exposablePackage()

	statuses := ExposeStatuses(config, p)
	if len(statuses) != 1 {
		t.Fatalf("ExposeStatuses() = %d entries, want 1", len(statuses))
	}

	st := statuses[0]
	if st.State != ExposeAbsent {
		t.Errorf("state = %v, want absent before anything is linked", st.State)
	}
	if !st.Declared || !st.Known {
		t.Errorf("declared = %v, known = %v, want both true", st.Declared, st.Known)
	}
	if st.Link != filepath.Join(config.Paths.Expose, "demo") {
		t.Errorf("link = %q", st.Link)
	}
	if st.Problem() != "not linked" {
		t.Errorf("Problem() = %q, want it to say the link is missing", st.Problem())
	}
}

func TestExposeStatusesReportShadowingAndPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH semantics under test are POSIX")
	}

	config := testConfig(t)
	in := NewInstaller(config, nil)
	if err := os.MkdirAll(config.Paths.Bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.Paths.Bin, "demo"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	in.applyExpose(exposablePackage(), in.pathsFor("demo"))

	// The expose directory is on PATH but something earlier answers to the same
	// name: the link is correct and still never the program that runs.
	earlier := t.TempDir()
	if err := os.WriteFile(filepath.Join(earlier, "demo"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", earlier+string(os.PathListSeparator)+config.Paths.Expose)

	st := ExposeStatuses(config, exposablePackage())[0]
	if st.State != ExposeLinked {
		t.Fatalf("state = %v, want linked", st.State)
	}
	if st.Shadow != earlier {
		t.Errorf("shadow = %q, want %q", st.Shadow, earlier)
	}
	if !strings.Contains(st.Problem(), "shadowed") {
		t.Errorf("Problem() = %q, want it to report the shadowing", st.Problem())
	}
	if st.OK() {
		t.Error("OK() = true for a shadowed link")
	}

	// The expose directory missing from PATH is the other way a link goes
	// unused, and it is worth a different sentence.
	t.Setenv("PATH", earlier)
	st = ExposeStatuses(config, exposablePackage())[0]
	if st.DirOnPath {
		t.Error("DirOnPath = true for a directory that is not on PATH")
	}
	if !strings.Contains(st.Problem(), "not on PATH") {
		t.Errorf("Problem() = %q, want it to report the missing PATH entry", st.Problem())
	}

	// And with the expose directory first, nothing is wrong.
	t.Setenv("PATH", config.Paths.Expose+string(os.PathListSeparator)+earlier)
	st = ExposeStatuses(config, exposablePackage())[0]
	if !st.OK() {
		t.Errorf("OK() = false with the link first on PATH: %s", st.Problem())
	}
}

func TestExposeWithoutAConfiguredDirectoryWarns(t *testing.T) {
	config := testConfig(t)
	config.Paths.Expose = ""

	in := NewInstaller(config, nil)
	rec := &recorder{}
	in.Report = rec.report

	in.applyExpose(exposablePackage(), in.pathsFor("demo"))
	if len(rec.kinds(EventWarn)) == 0 {
		t.Error("a package asking to be exposed with nowhere to link it said nothing")
	}

	if err := in.Expose(buildablePackage(), []string{"demo"}); err == nil {
		t.Error("Expose() error = nil, want a refusal when no directory is configured")
	}
}

func TestDefaultExposeDirIsUnderHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if got, want := cnfg.DefaultExposeDir(), filepath.Join(home, ".local", "bin"); got != want {
		t.Errorf("DefaultExposeDir() = %q, want %q", got, want)
	}
}
