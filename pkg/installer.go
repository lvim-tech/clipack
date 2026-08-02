package pkg

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/lvim-tech/clipack/cnfg"
	"github.com/lvim-tech/clipack/utils"
	"gopkg.in/yaml.v3"
)

// EventKind classifies a progress event.
type EventKind int

const (
	EventInfo EventKind = iota
	EventStep
	EventOutput
	EventWarn
	EventError
	// EventHint carries an explanation of a failure and what to do about it.
	// Separate from EventWarn so the interfaces can put it where a user will
	// actually read it, rather than in the middle of the build log.
	EventHint
	EventDone
)

// Event is a single progress notification emitted during an operation.
type Event struct {
	Kind    EventKind
	Text    string
	Step    int
	Total   int
	Package string
}

// Reporter receives progress events. It must be safe to call from the
// goroutine running the operation; the TUI forwards events onto a channel.
type Reporter func(Event)

// DiscardReporter drops all events.
func DiscardReporter(Event) {}

// Installer performs package operations against a configuration.
// It reports progress through a Reporter instead of writing to stdout, so the
// same code drives both the TUI and the non-interactive CLI.
type Installer struct {
	Config *cnfg.Config
	Report Reporter

	mu sync.Mutex
	// tail holds the most recent output of the step being run, so a failure
	// can be explained without keeping the whole log.
	tail *outputTail
}

// NewInstaller builds an Installer, defaulting to a no-op reporter.
func NewInstaller(config *cnfg.Config, report Reporter) *Installer {
	if report == nil {
		report = DiscardReporter
	}
	return &Installer{Config: config, Report: report}
}

func (in *Installer) emit(e Event) {
	in.mu.Lock()
	defer in.mu.Unlock()
	in.Report(e)
}

func (in *Installer) infof(format string, args ...any) {
	in.emit(Event{Kind: EventInfo, Text: fmt.Sprintf(format, args...)})
}

func (in *Installer) warnf(format string, args ...any) {
	in.emit(Event{Kind: EventWarn, Text: fmt.Sprintf(format, args...)})
}

func (in *Installer) outputf(line string) {
	in.emit(Event{Kind: EventOutput, Text: line})
}

// record keeps a line for the failure explanation. Guarded by the same lock as
// reporting: both output pipes are drained concurrently.
func (in *Installer) record(line string) {
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.tail != nil {
		in.tail.add(line)
	}
}

// explain reports what the captured output says about a failure. Nothing is
// emitted when the output is not recognised — a guess dressed up as advice is
// worse than the raw error, which the user has already seen.
func (in *Installer) explain() {
	in.mu.Lock()
	var lines []string
	if in.tail != nil {
		lines = in.tail.all()
	}
	in.mu.Unlock()

	for _, d := range Diagnose(lines) {
		text := d.Cause
		if d.Fix != "" {
			text += "\n" + d.Fix
		}
		in.emit(Event{Kind: EventHint, Text: text})
	}
}

// Paths bundles the destination directories for a package.
type Paths struct {
	Base   string
	Bin    string
	Config string
	Build  string
	Man    string
}

func (in *Installer) pathsFor(name string) Paths {
	return Paths{
		Base:   in.Config.Paths.Base,
		Bin:    in.Config.Paths.Bin,
		Config: filepath.Join(in.Config.Paths.Configs, name),
		Build:  filepath.Join(in.Config.Paths.Build, name),
		Man:    in.Config.Paths.Man,
	}
}

// under joins rel onto root and refuses anything that escapes root.
//
// Registry files are the input here and the result reaches os.RemoveAll, so
// "../.." has to be rejected rather than cleaned into something plausible.
func under(root, rel string) (string, error) {
	if rel == "" {
		return "", fmt.Errorf("empty path")
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("%q must be relative", rel)
	}
	abs := filepath.Join(root, rel)
	inside, err := filepath.Rel(root, abs)
	if err != nil {
		return "", err
	}
	if inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%q points outside %s", rel, root)
	}
	if inside == "." {
		return "", fmt.Errorf("%q is the directory itself", rel)
	}
	return abs, nil
}

// overlaps reports whether either path contains the other, or they are equal.
func overlaps(a, b string) bool {
	a, b = filepath.Clean(a), filepath.Clean(b)
	if a == b {
		return true
	}
	return strings.HasPrefix(a, b+string(filepath.Separator)) ||
		strings.HasPrefix(b, a+string(filepath.Separator))
}

// resolveResource validates one resource declaration and returns the absolute
// source and destination.
//
// Three things are checked, all of which are ways a registry file could turn an
// uninstall into data loss:
//
//   - neither path may escape the directory it is relative to;
//   - the destination may not touch a directory clipack manages itself, since
//     removing the resource would then take other packages' binaries, configs
//     or man pages with it;
//   - the destination may not BE a top-level directory under base. "lib/kitty"
//     is one package's tree and removing it is safe; "lib" is shared, and the
//     first uninstall would empty it for everyone.
func (in *Installer) resolveResource(res Resource, paths Paths) (string, string, error) {
	src, err := under(paths.Build, res.Source)
	if err != nil {
		return "", "", fmt.Errorf("resource source: %w", err)
	}

	dst, err := under(paths.Base, res.Target)
	if err != nil {
		return "", "", fmt.Errorf("resource target: %w", err)
	}

	rel, err := filepath.Rel(paths.Base, dst)
	if err != nil {
		return "", "", err
	}
	if len(strings.Split(rel, string(filepath.Separator))) < 2 {
		return "", "", fmt.Errorf(
			"resource target %q is a top-level directory; use a path a single package owns, like %q",
			res.Target, res.Target+"/"+filepath.Base(res.Source))
	}

	for name, managed := range map[string]string{
		"bin":      in.Config.Paths.Bin,
		"configs":  in.Config.Paths.Configs,
		"build":    in.Config.Paths.Build,
		"man":      in.Config.Paths.Man,
		"registry": in.Config.Paths.Registry,
	} {
		if managed != "" && overlaps(dst, managed) {
			return "", "", fmt.Errorf("resource target %q overlaps the %s directory", res.Target, name)
		}
	}

	return src, dst, nil
}

// ResolveMethod falls back to the configured default install method.
func (in *Installer) ResolveMethod(method string) string {
	if method != "" {
		return method
	}
	if in.Config.Options.InstallMethod != "" {
		return in.Config.Options.InstallMethod
	}
	return MethodVersion
}

// Install builds and installs a package. Existing build directories are removed
// without prompting; callers confirm beforehand.
func (in *Installer) Install(p *Package, method string) error {
	return in.install(p, method, nil)
}

// install is the shared implementation. previous, when set, is the manifest of
// the version being replaced — and it is cleaned up only AFTER the build has
// succeeded. Cleaning first read better, but it meant a failed build left the
// package uninstalled: the working version's binaries were already gone by the
// time the compiler said no. A ghostty update died on a compiler-version check
// and took the installed ghostty with it; staging is what that cost.
func (in *Installer) install(p *Package, method string, previous *Package) error {
	method = in.ResolveMethod(method)
	paths := in.pathsFor(p.Name)

	in.emit(Event{Kind: EventInfo, Package: p.Name,
		Text: fmt.Sprintf("Installing %s (%s: %s)", p.Name, method, p.Ref(method))})

	if err := os.RemoveAll(paths.Build); err != nil {
		return fmt.Errorf("removing build directory: %w", err)
	}
	for _, dir := range []string{paths.Base, paths.Bin, paths.Config, paths.Build, paths.Man} {
		if err := utils.EnsureDirectoryExists(dir); err != nil {
			return fmt.Errorf("creating directory %s: %w", dir, err)
		}
	}

	if err := in.runSteps(p, method, paths.Build); err != nil {
		return err
	}

	// Only now, with a finished build in hand, does the previous version go.
	// The window where neither version is installed shrinks from the whole
	// build to the copy steps below.
	if previous != nil {
		in.removeArtifacts(previous, paths)
		if err := os.RemoveAll(paths.Config); err != nil {
			in.warnf("could not remove config directory: %v", err)
		}
		if err := utils.EnsureDirectoryExists(paths.Config); err != nil {
			return fmt.Errorf("recreating config directory: %w", err)
		}
	}

	var errs []error
	errs = append(errs, in.installBinaries(p, paths)...)
	errs = append(errs, in.installResources(p, paths)...)
	errs = append(errs, in.installDesktopEntries(p, paths)...)
	errs = append(errs, in.installConfigs(p, paths)...)
	errs = append(errs, in.installMan(p, paths)...)
	errs = append(errs, in.installAdditionalConfig(p, paths)...)

	p.InstallMethod = method
	if err := in.writeManifest(p, paths); err != nil {
		errs = append(errs, err)
	}
	errs = append(errs, in.installPostInstallScripts(p, paths)...)

	// After the manifest, so a setup script can read what was installed, and
	// after the config files it links to have been written.
	in.runSetup(p, paths)
	in.refreshShellIntegration()

	if in.Config.Options.CleanupBuild {
		if err := os.RemoveAll(paths.Build); err != nil {
			in.warnf("could not remove build directory: %v", err)
		}
	}

	if err := errors.Join(errs...); err != nil {
		return err
	}

	in.emit(Event{Kind: EventDone, Package: p.Name,
		Text: fmt.Sprintf("Successfully installed %s (%s)", p.Name, p.Ref(method))})
	return nil
}

// Update reinstalls a package, cleaning up the artifacts recorded in the
// previously installed manifest before rebuilding.
func (in *Installer) Update(p *Package, method string) error {
	method = in.ResolveMethod(method)
	paths := in.pathsFor(p.Name)

	// The old manifest is read here but acted on only after the new build
	// succeeds — install() does the cleanup at that point. Nothing of the
	// working version is touched before there is something to replace it with.
	previous, err := in.readManifest(paths)
	if err != nil {
		previous = nil
	}

	return in.install(p, method, previous)
}

// Remove uninstalls a package based on its installed manifest.
func (in *Installer) Remove(p *Package) error {
	paths := in.pathsFor(p.Name)

	in.emit(Event{Kind: EventInfo, Package: p.Name, Text: "Removing " + p.Name})
	in.removeArtifacts(p, paths)

	if err := os.RemoveAll(paths.Config); err != nil {
		return fmt.Errorf("removing config directory: %w", err)
	}
	in.infof("Removed config directory %s", paths.Config)

	if err := os.RemoveAll(paths.Build); err != nil {
		in.warnf("could not remove build directory: %v", err)
	}

	// The config directory is gone, so its integration has to leave the
	// aggregate with it — otherwise every shell would source a missing file.
	in.refreshShellIntegration()

	in.emit(Event{Kind: EventDone, Package: p.Name,
		Text: fmt.Sprintf("Successfully removed %s", p.Name)})
	return nil
}

// removeArtifacts deletes binaries, resource trees, man pages and post-install
// scripts — everything the install put outside the package's own config
// directory, which Remove deletes wholesale.
func (in *Installer) removeArtifacts(p *Package, paths Paths) {
	in.removeDesktopEntries(p)

	for _, res := range p.Install.Resources {
		_, dst, err := in.resolveResource(res, paths)
		if err != nil {
			// Refusing to delete beats guessing at what a malformed target meant.
			in.warnf("not removing resource %q: %v", res.Target, err)
			continue
		}
		if _, err := os.Stat(dst); os.IsNotExist(err) {
			continue
		}
		if err := os.RemoveAll(dst); err != nil {
			in.warnf("could not remove resources %s: %v", dst, err)
			continue
		}
		in.infof("Removed resources %s", dst)
		pruneEmptyParents(dst, paths.Base)
	}

	for _, binPath := range p.Install.Binaries {
		target := filepath.Join(paths.Bin, filepath.Base(binPath))
		if err := os.Remove(target); err == nil {
			in.infof("Removed binary %s", target)
		} else if !os.IsNotExist(err) {
			in.warnf("could not remove binary %s: %v", target, err)
		}
	}

	for _, manPage := range p.Install.Man {
		target, ok := manTarget(paths.Man, manPage)
		if !ok {
			continue
		}
		if err := os.Remove(target); err == nil {
			in.infof("Removed man page %s", target)
		} else if !os.IsNotExist(err) {
			in.warnf("could not remove man page %s: %v", target, err)
		}
	}

	for _, script := range p.PostInstall.Scripts {
		// install writes these with filepath.Base; remove must match.
		target := filepath.Join(paths.Bin, filepath.Base(script.Filename))
		if err := os.Remove(target); err == nil {
			in.infof("Removed post-install script %s", target)
		} else if !os.IsNotExist(err) {
			in.warnf("could not remove post-install script %s: %v", target, err)
		}
	}
}

// runSteps executes the install steps inside buildDir.
func (in *Installer) runSteps(p *Package, method, buildDir string) error {
	steps := in.expandSteps(p, method)
	total := len(steps)

	for i, step := range steps {
		in.emit(Event{Kind: EventStep, Package: p.Name, Step: i + 1, Total: total, Text: step})
		// Reset per step: the failure is explained from the output of the step
		// that failed, not from whatever a successful earlier step printed.
		in.mu.Lock()
		in.tail = newOutputTail(diagnosticTailLines)
		in.mu.Unlock()

		if err := in.runCommand(step, buildDir, p.Install.Environment); err != nil {
			in.explain()
			return fmt.Errorf("step %d/%d %q failed: %w", i+1, total, step, err)
		}
	}
	return nil
}

// diagnosticTailLines is how much of a step's output is kept for diagnosis.
// Compilers repeat themselves — a Rust workspace prints its toolchain
// requirement once per crate — so the window has to outlast that repetition
// while staying far below the size of a full build log.
const diagnosticTailLines = 400

// expandSteps rewrites the "git clone" step so the requested version or commit
// is actually checked out. The previous code only did this on install, never on
// update, so updates silently pulled the default branch HEAD.
func (in *Installer) expandSteps(p *Package, method string) []string {
	cloneURL := p.CloneURL()
	var steps []string

	for _, step := range p.Install.Steps {
		if !strings.Contains(step, "git clone") || cloneURL == "" {
			steps = append(steps, step)
			continue
		}

		if method == MethodCommit && p.Commit != "" {
			steps = append(steps,
				fmt.Sprintf("git clone %s .", shellQuote(cloneURL)),
				fmt.Sprintf("git checkout %s", shellQuote(p.Commit)),
			)
			continue
		}

		if p.Version != "" {
			steps = append(steps, fmt.Sprintf(
				"git clone --branch %s --single-branch --depth 1 %s .",
				shellQuote(p.Version), shellQuote(cloneURL)))
			continue
		}

		steps = append(steps, fmt.Sprintf("git clone --depth 1 %s .", shellQuote(cloneURL)))
	}

	return steps
}

// runCommand executes a step through a shell inside dir. Using a shell (rather
// than strings.Fields) preserves quoting, pipes and && in registry steps, and
// cmd.Dir replaces the old process-wide os.Chdir, which corrupted relative
// paths for everything else running in the process.
// buildEnv is the environment a step runs with: the process environment minus
// CONFIG_SITE, plus the entry's own variables — appended last, so an entry
// that genuinely needs the variable can still set it in install.environment.
//
// CONFIG_SITE is openSUSE's autoconf site file (/etc/profile.d/site.sh exports
// it), and it makes every configure default libdir to lib64. Nothing clipack
// builds installs libraries into the system, so the variable buys the builds
// nothing — and it broke one: a vendored jemalloc inside a cargo build honours
// it while the crate's build script does not, so tikv-jemalloc-sys installed
// libjemalloc_pic.a into out/lib64 and told rustc to search out/lib. yazi's
// main branch hit that live; fd, difftastic and qsv carry the same crate on
// linux-gnu and would have hit it on their first install.
func buildEnv(base []string, extra map[string]string) []string {
	out := make([]string, 0, len(base)+len(extra))
	for _, kv := range base {
		if strings.HasPrefix(kv, "CONFIG_SITE=") {
			continue
		}
		out = append(out, kv)
	}
	for k, v := range extra {
		out = append(out, k+"="+v)
	}
	return out
}

func (in *Installer) runCommand(step, dir string, env map[string]string) error {
	shell := "/bin/sh"
	args := []string{"-c", step}
	if runtime.GOOS == "windows" {
		shell = "cmd"
		args = []string{"/C", step}
	}

	cmd := exec.Command(shell, args...)
	cmd.Dir = dir
	cmd.Env = buildEnv(os.Environ(), env)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	// Both pipes must be fully drained before Wait, otherwise a build that
	// writes more than the pipe buffer deadlocks.
	var wg sync.WaitGroup
	scan := func(r io.Reader) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			in.record(line)
			in.outputf(line)
		}
	}
	wg.Add(2)
	go scan(stdout)
	go scan(stderr)
	wg.Wait()

	return cmd.Wait()
}

func (in *Installer) installBinaries(p *Package, paths Paths) []error {
	var errs []error
	for _, binPath := range p.Install.Binaries {
		src := filepath.Join(paths.Build, binPath)
		dst := filepath.Join(paths.Bin, filepath.Base(binPath))

		if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
			in.warnf("could not replace existing binary %s: %v", dst, err)
		}
		if err := CopyFile(src, dst); err != nil {
			errs = append(errs, fmt.Errorf("copying binary %s: %w", binPath, err))
			continue
		}
		if err := os.Chmod(dst, 0o755); err != nil {
			errs = append(errs, fmt.Errorf("chmod %s: %w", dst, err))
			continue
		}
		in.infof("Installed binary %s", dst)
	}
	return errs
}

// installDesktopEntries puts a package's menu entries in the user's application
// directory. Failures are warnings, not errors: a program that is installed and
// runnable but missing from the menu is a working install, and refusing the
// whole package over a .desktop file would be out of proportion.
func (in *Installer) installDesktopEntries(p *Package, paths Paths) []error {
	for _, entry := range p.Install.Desktop {
		src, err := under(paths.Build, entry.Source)
		if err != nil {
			in.warnf("desktop entry source: %v", err)
			continue
		}
		contents, err := os.ReadFile(src)
		if err != nil {
			in.warnf("desktop entry %s not found in build output", entry.Source)
			continue
		}

		dst, iconDir, err := desktopPaths(p.Name, entry.Source)
		if err != nil {
			in.warnf("desktop entry: %v", err)
			continue
		}

		icon := in.installDesktopIcon(p, entry, paths, iconDir)

		if err := utils.EnsureDirectoryExists(filepath.Dir(dst)); err != nil {
			in.warnf("could not create %s: %v", filepath.Dir(dst), err)
			continue
		}
		rewritten := rewriteDesktopEntry(contents, desktopRewrite{
			BinDir: paths.Bin,
			Name:   entry.Name,
			Icon:   icon,
			Env:    renderDesktopEnv(entry.Env, paths.Base),
		})
		if err := os.WriteFile(dst, rewritten, 0o644); err != nil {
			in.warnf("could not write desktop entry %s: %v", dst, err)
			continue
		}
		in.infof("Installed desktop entry %s", dst)
	}
	return nil
}

// renderDesktopEnv turns the entry's env map into sorted K=V pairs, expanding
// the ${base} placeholder. Sorted because map order is random and the entry
// file should not churn between installs of the same definition.
func renderDesktopEnv(env map[string]string, base string) []string {
	if len(env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, k+"="+strings.ReplaceAll(env[k], "${base}", base))
	}
	return pairs
}

// installDesktopIcon copies the icon an entry declares and returns its absolute
// path, or "" when there is none to install — in which case Icon= is left as the
// package wrote it and resolves through the icon theme.
func (in *Installer) installDesktopIcon(p *Package, entry DesktopEntry, paths Paths, iconDir string) string {
	if entry.Icon == "" {
		return ""
	}

	src, err := under(paths.Build, entry.Icon)
	if err != nil {
		in.warnf("desktop icon: %v", err)
		return ""
	}
	if _, err := os.Stat(src); err != nil {
		in.warnf("desktop icon %s not found in build output", entry.Icon)
		return ""
	}

	if err := utils.EnsureDirectoryExists(iconDir); err != nil {
		in.warnf("could not create %s: %v", iconDir, err)
		return ""
	}
	dst := filepath.Join(iconDir, filepath.Base(entry.Icon))
	if err := CopyFile(src, dst); err != nil {
		in.warnf("could not copy desktop icon %s: %v", entry.Icon, err)
		return ""
	}
	in.infof("Installed desktop icon %s", dst)
	return dst
}

// removeDesktopEntries deletes the menu entries and icons an install added.
func (in *Installer) removeDesktopEntries(p *Package) {
	for _, entry := range p.Install.Desktop {
		dst, iconDir, err := desktopPaths(p.Name, entry.Source)
		if err != nil {
			in.warnf("not removing desktop entry: %v", err)
			continue
		}

		// What keeps this from deleting a file the distribution owns is that the
		// name is derived rather than taken: desktopPaths prefixes it, so a
		// manifest naming "kitty.desktop" resolves to "clipack-kitty-kitty.desktop"
		// and the system entry is out of reach by construction.
		if err := os.Remove(dst); err == nil {
			in.infof("Removed desktop entry %s", dst)
		} else if !os.IsNotExist(err) {
			in.warnf("could not remove desktop entry %s: %v", dst, err)
		}

		if entry.Icon == "" {
			continue
		}
		if err := os.RemoveAll(iconDir); err != nil {
			in.warnf("could not remove desktop icons %s: %v", iconDir, err)
		}
	}
}

func (in *Installer) installResources(p *Package, paths Paths) []error {
	var errs []error
	for _, res := range p.Install.Resources {
		src, dst, err := in.resolveResource(res, paths)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		info, err := os.Stat(src)
		if err != nil {
			errs = append(errs, fmt.Errorf("resource %s not found in build output", res.Source))
			continue
		}
		if !info.IsDir() {
			errs = append(errs, fmt.Errorf("resource %s is not a directory", res.Source))
			continue
		}

		// Replace the tree rather than copy over it: files the previous version
		// shipped and this one dropped are exactly what a merge would keep alive,
		// and a stale .so or Python module is worse than a missing one.
		if err := os.RemoveAll(dst); err != nil {
			errs = append(errs, fmt.Errorf("replacing %s: %w", dst, err))
			continue
		}
		if err := CopyTree(src, dst); err != nil {
			errs = append(errs, fmt.Errorf("copying resource %s: %w", res.Source, err))
			continue
		}
		in.infof("Installed resources %s", dst)
	}
	return errs
}

// pruneEmptyParents removes the directories between dir and root that the
// removal just emptied, so uninstalling the only package that used <base>/lib
// does not leave the directory behind. os.Remove refuses a non-empty directory,
// which is precisely the stop condition wanted here.
func pruneEmptyParents(dir, root string) {
	for {
		dir = filepath.Dir(dir)
		if !overlaps(dir, root) || filepath.Clean(dir) == filepath.Clean(root) {
			return
		}
		if os.Remove(dir) != nil {
			return
		}
	}
}

func (in *Installer) installConfigs(p *Package, paths Paths) []error {
	var errs []error
	for _, confPath := range p.Install.Configs {
		src := filepath.Join(paths.Build, confPath)
		dst := filepath.Join(paths.Config, filepath.Base(confPath))

		if _, err := os.Stat(src); err != nil {
			in.warnf("config file %s not found in build output", confPath)
			continue
		}
		if err := CopyFile(src, dst); err != nil {
			errs = append(errs, fmt.Errorf("copying config %s: %w", confPath, err))
			continue
		}
		in.infof("Installed config %s", dst)
	}
	return errs
}

// manTarget resolves the destination path for a man page, e.g.
// "man/man1/bat.1" -> "<man>/man1/bat.1".
func manTarget(manDir, manPage string) (string, bool) {
	ext := filepath.Ext(manPage)
	if len(ext) < 2 {
		return "", false
	}
	return filepath.Join(manDir, "man"+ext[1:], filepath.Base(manPage)), true
}

func (in *Installer) installMan(p *Package, paths Paths) []error {
	var errs []error
	for _, manPage := range p.Install.Man {
		dst, ok := manTarget(paths.Man, manPage)
		if !ok {
			in.warnf("could not determine man section for %s", manPage)
			continue
		}

		src := filepath.Join(paths.Build, manPage)
		if _, err := os.Stat(src); err != nil {
			in.warnf("man page %s not found in build output", manPage)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			errs = append(errs, fmt.Errorf("creating man directory: %w", err))
			continue
		}
		if err := CopyFile(src, dst); err != nil {
			errs = append(errs, fmt.Errorf("copying man page %s: %w", manPage, err))
			continue
		}
		in.infof("Installed man page %s", dst)
	}
	return errs
}

func (in *Installer) installAdditionalConfig(p *Package, paths Paths) []error {
	var errs []error
	for _, ac := range p.Install.AdditionalConfig {
		dst := filepath.Join(paths.Config, ac.Filename)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			errs = append(errs, fmt.Errorf("creating directory for %s: %w", ac.Filename, err))
			continue
		}

		var content []byte
		if strings.HasPrefix(ac.Content, "http://") || strings.HasPrefix(ac.Content, "https://") {
			downloaded, err := utils.DownloadContent(ac.Content)
			if err != nil {
				errs = append(errs, fmt.Errorf("downloading %s: %w", ac.Filename, err))
				continue
			}
			content = downloaded
		} else {
			content = []byte(ac.Content)
		}

		mode := os.FileMode(0o644)
		if strings.HasSuffix(ac.Filename, ".sh") {
			mode = 0o755
		}
		if err := os.WriteFile(dst, content, mode); err != nil {
			errs = append(errs, fmt.Errorf("writing %s: %w", ac.Filename, err))
			continue
		}
		in.infof("Created %s", dst)
	}
	return errs
}

// installPostInstallScripts writes scripts straight into the bin directory.
// The previous code wrote them into the build directory and then os.Rename'd
// them, which fails outright when build and bin sit on different filesystems.
func (in *Installer) installPostInstallScripts(p *Package, paths Paths) []error {
	var errs []error
	for _, script := range p.PostInstall.Scripts {
		dst := filepath.Join(paths.Bin, filepath.Base(script.Filename))
		if err := os.WriteFile(dst, []byte(script.Content), 0o755); err != nil {
			errs = append(errs, fmt.Errorf("writing post-install script %s: %w", script.Filename, err))
			continue
		}
		in.infof("Installed post-install script %s", dst)
	}
	return errs
}

func (in *Installer) writeManifest(p *Package, paths Paths) error {
	data, err := yaml.Marshal(p)
	if err != nil {
		return fmt.Errorf("marshaling package manifest: %w", err)
	}
	manifestPath := filepath.Join(paths.Config, "package.yaml")
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		return fmt.Errorf("writing package manifest: %w", err)
	}
	return nil
}

func (in *Installer) readManifest(paths Paths) (*Package, error) {
	data, err := os.ReadFile(filepath.Join(paths.Config, "package.yaml"))
	if err != nil {
		return nil, err
	}
	return LoadPackageFromBytes(data)
}

// shellQuote wraps a value in single quotes for safe shell interpolation.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
