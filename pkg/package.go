// Package pkg holds clipack's domain logic: the package format, registry
// access, the on-disk cache and the installer.
//
// Nothing here writes to stdout. Progress is reported through Installer's
// Reporter callback, which is what lets the CLI and the TUI share one
// implementation of install, update and remove.
//
// The files are split as:
//
//	package.go   - the Package type and helpers over it
//	registry.go  - fetching packages from the registry over HTTP
//	cache.go     - the local registry cache
//	installer.go - building and installing a package
package pkg

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lvim-tech/clipack/cnfg"
	"gopkg.in/yaml.v3"
)

// Install method identifiers.
const (
	MethodVersion = "version"
	MethodCommit  = "commit"
)

// Source is where the package sources come from: one URL, and nothing else.
//
// It carried two more fields, both parsed and never read, and both removed once
// that was checked field by field against the rest of the schema — which came
// out clean, they were the only two.
//
//   - `ref` named a branch, and 82 of 83 entries set one. What a build checks
//     out comes from `version` or `commit`, whichever the install method names,
//     and expandSteps is where that happens. The field read like the answer to
//     "which branch is built", which it never was.
//   - `type` said `git` in 81 entries and `script` in two. Nothing branched on
//     it — the steps say what to do, and every entry's first step is a clone
//     regardless of which word was here.
type Source struct {
	URL string `yaml:"url,omitempty"`
}

// Install holds the installation steps and related data.
type Install struct {
	Source           Source             `yaml:"source,omitempty"`
	Environment      map[string]string  `yaml:"environment,omitempty"`
	Steps            []string           `yaml:"steps,omitempty"`
	Binaries         []string           `yaml:"binaries,omitempty"`
	Resources        []Resource         `yaml:"resources,omitempty"`
	Desktop          []DesktopEntry     `yaml:"desktop,omitempty"`
	Configs          []string           `yaml:"configs,omitempty"`
	Man              []string           `yaml:"man,omitempty"`
	AdditionalConfig []AdditionalConfig `yaml:"additional-config,omitempty"`
	// Setup is a shell script clipack runs once, after everything is installed.
	//
	// It exists to separate two things that used to share one file. Linking a
	// theme into ~/.config is done once and is clipack's job; exporting a
	// variable or calling `eval "$(zoxide init zsh)"` has to happen inside the
	// user's shell, every time, and no process can do that for its parent. The
	// first kind belongs here; the second stays in a config.sh that gets sourced.
	Setup string `yaml:"setup,omitempty"`
}

// Resource is a directory tree a package needs installed alongside its
// binaries, and which uninstalling therefore has to remove.
//
// Not every program is a self-contained executable. kitty's launcher is
// compiled with KITTY_LIB_PATH="../lib/kitty" and loads its Python package from
// there; ghostty looks for its terminfo and shell integration in
// "../share/ghostty". Both resolve that against their own location, so with
// binaries in <base>/bin the trees have to land in <base>/lib and <base>/share.
//
// Declaring them here rather than copying them from an install step is what
// makes them removable: they go into the manifest, so Remove knows what to
// delete even after the registry entry has moved on.
type Resource struct {
	// Source is the directory in the build output, relative to the build dir.
	Source string `yaml:"source"`
	// Target is where it is installed, relative to the base directory.
	Target string `yaml:"target"`
}

// Requirements are what has to be on the machine before a build can start.
//
// Split in two because they are obtained differently. Distribution packages can
// be installed with one command, which clipack renders ready to paste; a
// toolchain is named with the version it has to satisfy and left to the user,
// who may keep it in mise, rustup or the distribution and would not thank
// clipack for guessing.
//
// Version and Commit add to the shared set rather than replacing it, because
// the two refs of one package are usually built the same way — and when they
// are not, that difference is the only thing worth writing down. ghostty is the
// case that earned this: its release tag demands zig 0.15.2 and its main branch
// 0.16.0, and a single list could only ever be wrong for one of them.
type Requirements struct {
	MethodRequirements `yaml:",inline"`
	Version            MethodRequirements `yaml:"version,omitempty"`
	Commit             MethodRequirements `yaml:"commit,omitempty"`
}

// MethodRequirements is one set of requirements, for a distribution and for
// toolchains.
type MethodRequirements struct {
	// OpenSUSE names packages as zypper knows them. Other distributions get
	// their own field when someone verifies the names on one — a guessed
	// package name is worse than none, because it fails at the point where the
	// user has already trusted it.
	OpenSUSE []string `yaml:"opensuse,omitempty"`
	// Toolchain entries carry their version constraint, e.g. "zig >= 0.16".
	Toolchain []string `yaml:"toolchain,omitempty"`
}

// Empty reports whether there is nothing to require.
func (r MethodRequirements) Empty() bool {
	return len(r.OpenSUSE) == 0 && len(r.Toolchain) == 0
}

// For returns what a build with the given method needs: the shared set plus
// whatever that method adds.
func (r Requirements) For(method string) MethodRequirements {
	extra := r.Version
	if method == MethodCommit {
		extra = r.Commit
	}
	return MethodRequirements{
		OpenSUSE:  mergeRequirements(r.OpenSUSE, extra.OpenSUSE),
		Toolchain: mergeRequirements(r.Toolchain, extra.Toolchain),
	}
}

// mergeRequirements concatenates two lists, dropping duplicates and keeping the
// order they were written in — the registry lists them in the order a reader
// would want to see them, not alphabetically.
func mergeRequirements(base, extra []string) []string {
	if len(extra) == 0 {
		return base
	}

	seen := make(map[string]bool, len(base)+len(extra))
	merged := make([]string, 0, len(base)+len(extra))
	for _, list := range [][]string{base, extra} {
		for _, item := range list {
			if item == "" || seen[item] {
				continue
			}
			seen[item] = true
			merged = append(merged, item)
		}
	}
	return merged
}

// ZypperCommand renders the command that installs the named packages on
// openSUSE, ready to be copied into a terminal. It returns "" for an empty list
// rather than a command that would do nothing.
func ZypperCommand(packages []string) string {
	if len(packages) == 0 {
		return ""
	}
	return "sudo zypper in " + strings.Join(packages, " ")
}

// DesktopEntry is a .desktop file a package ships, installed into the user's
// application directory so a graphical program appears in menus and launchers.
//
// Which packages have one is the registry's business, not clipack's: a terminal
// emulator ships an entry, a command-line tool does not, and the difference is
// visible in the build output rather than in anything clipack could infer.
//
// The entry is installed alongside the distribution's, not over it, and its Exec
// is repointed at the binary clipack installed — see rewriteDesktopEntry for why
// that rewrite is the part that matters.
type DesktopEntry struct {
	// Source is the .desktop file in the build output, relative to the build dir.
	Source string `yaml:"source"`
	// Icon is an optional image in the build output. Without it the entry falls
	// back to the icon theme, which only has one when a system package supplied
	// it — so a package that is not also installed system-wide wants this.
	Icon string `yaml:"icon,omitempty"`
	// Name replaces the displayed name. Empty appends " (clipack)" to whatever
	// the shipped file says.
	Name string `yaml:"name,omitempty"`
	// Env is written into Exec as an `env K=V …` prefix. It exists because a
	// menu entry runs with the session's environment, not the shell's: a
	// program configured through a variable that config.sh exports (yazi's
	// YAZI_CONFIG_HOME) looks themed in every terminal and unthemed when
	// launched from the menu. The literal ${base} in a value is replaced with
	// the installation's base directory at install time.
	Env map[string]string `yaml:"env,omitempty"`
}

// AdditionalConfig holds additional configuration data.
type AdditionalConfig struct {
	Filename string `yaml:"filename"`
	Content  string `yaml:"content"`
}

// PostInstall holds post-installation scripts.
type PostInstall struct {
	Scripts []Script `yaml:"scripts,omitempty"`
}

// Script holds a script filename and its content.
type Script struct {
	Filename string `yaml:"filename"`
	Content  string `yaml:"content"`
}

// Package holds the package data.
type Package struct {
	Name        string    `yaml:"name"`
	Version     string    `yaml:"version"`
	Commit      string    `yaml:"commit"`
	Description string    `yaml:"description"`
	Maintainer  string    `yaml:"maintainer"`
	UpdatedAt   time.Time `yaml:"updated_at"`
	Tags        []string  `yaml:"tags"`
	Category    string    `yaml:"category,omitempty"`
	License     string    `yaml:"license"`
	Homepage    string    `yaml:"homepage"`
	// Requirements is what the machine needs before the build can run. It sits
	// beside Install rather than inside it because it describes the machine, not
	// the steps: nothing in it is fetched, built or removed by clipack.
	Requirements  Requirements `yaml:"requirements,omitempty"`
	Install       Install      `yaml:"install"`
	PostInstall   PostInstall  `yaml:"post-install,omitempty"`
	InstallMethod string       `yaml:"install_method,omitempty"`
}

// Ref returns the identifier this package is pinned to for the given method.
func (p *Package) Ref(method string) string {
	if method == MethodCommit {
		return p.Commit
	}
	return p.Version
}

// CloneURL returns the git URL for the package, preferring the declared
// source over the URL embedded in the first "git clone" step.
func (p *Package) CloneURL() string {
	if p.Install.Source.URL != "" {
		return p.Install.Source.URL
	}
	for _, step := range p.Install.Steps {
		if url := cloneURLFromStep(step); url != "" {
			return url
		}
	}
	return ""
}

// cloneURLFromStep extracts the repository URL from a "git clone ..." step.
// It skips flags so "git clone --depth 1 <url> ." resolves correctly, unlike
// blindly taking the third field.
func cloneURLFromStep(step string) string {
	fields := strings.Fields(step)
	if len(fields) < 3 || fields[0] != "git" || fields[1] != "clone" {
		return ""
	}
	for i := 2; i < len(fields); i++ {
		f := fields[i]
		if strings.HasPrefix(f, "-") {
			// --depth 1 style flags consume the next field.
			if f == "--depth" || f == "-b" || f == "--branch" || f == "-o" || f == "--origin" {
				i++
			}
			continue
		}
		if f == "." {
			continue
		}
		return f
	}
	return ""
}

// Matches reports whether the package matches a free-text query.
func (p *Package) Matches(query string) bool {
	if query == "" {
		return true
	}
	q := strings.ToLower(query)
	if strings.Contains(strings.ToLower(p.Name), q) ||
		strings.Contains(strings.ToLower(p.Description), q) ||
		strings.Contains(strings.ToLower(p.Category), q) {
		return true
	}
	for _, tag := range p.Tags {
		if strings.Contains(strings.ToLower(tag), q) {
			return true
		}
	}
	return false
}

// FindByName returns the package with the given name, or nil.
func FindByName(packages []*Package, name string) *Package {
	for _, p := range packages {
		if p.Name == name {
			return p
		}
	}
	return nil
}

// LoadPackageFromBytes loads a package from a byte array.
func LoadPackageFromBytes(data []byte) (*Package, error) {
	var pkg Package
	if err := yaml.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}
	return &pkg, nil
}

// LoadPackageFromReader loads a package from an io.Reader.
func LoadPackageFromReader(r io.Reader) (*Package, error) {
	var pkg Package
	decoder := yaml.NewDecoder(r)
	if err := decoder.Decode(&pkg); err != nil {
		return nil, err
	}
	return &pkg, nil
}

// CopyFile copies a file from src to dst, preserving the source mode.
func CopyFile(src, dst string) error {
	sourceFileStat, err := os.Stat(src)
	if err != nil {
		return err
	}

	if !sourceFileStat.Mode().IsRegular() {
		return fmt.Errorf("%s is not a regular file", src)
	}

	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	destination, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, sourceFileStat.Mode().Perm())
	if err != nil {
		return err
	}
	defer destination.Close()

	if _, err := io.Copy(destination, source); err != nil {
		return err
	}

	return destination.Close()
}

// CopyTree recursively copies the directory src to dst.
//
// Symlinks are recreated as symlinks rather than followed: kitty's package tree
// contains them, and dereferencing turns one link into a full second copy — or
// into an error, when the link is relative and points outside the tree.
func CopyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		info, err := d.Info()
		if err != nil {
			return err
		}

		switch {
		case d.IsDir():
			return os.MkdirAll(target, info.Mode().Perm())
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			// A stale link from an earlier install would make Symlink fail.
			if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
				return err
			}
			return os.Symlink(link, target)
		case info.Mode().IsRegular():
			return CopyFile(path, target)
		default:
			// Sockets, devices and pipes have no business in a build output;
			// skipping beats failing the whole install over one of them.
			return nil
		}
	})
}

// MissingArtifacts lists the binaries and resource trees a package's manifest
// claims are installed but which are not on disk.
//
// A package counts as installed because its manifest exists under configs/,
// and the manifest outlives whatever it describes: a binary can be deleted,
// replaced by something that is not a program, or never written because the
// build produced a different path than the package declared. clipack then
// reports the package as installed, refuses to install it again, and offers an
// update for something that is not there.
//
// The check is by existence and, for binaries, by being a regular file —
// a stale unix socket sitting where a binary belongs looks installed to
// os.Stat but is not a program, and PATH skips it silently.
func (p *Package) MissingArtifacts(config *cnfg.Config) []string {
	var missing []string

	for _, bin := range p.Install.Binaries {
		path := filepath.Join(config.Paths.Bin, filepath.Base(bin))
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			missing = append(missing, path)
		}
	}

	for _, res := range p.Install.Resources {
		if res.Target == "" {
			continue
		}
		path := filepath.Join(config.Paths.Base, res.Target)
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
			missing = append(missing, path)
		}
	}

	return missing
}

// IsIntact reports whether everything the manifest describes is still present.
func (p *Package) IsIntact(config *cnfg.Config) bool {
	return len(p.MissingArtifacts(config)) == 0
}

// LoadInstalledPackages loads installed packages from the config directory.
// A missing configs directory means "nothing installed yet", not an error.
func LoadInstalledPackages(config *cnfg.Config) ([]*Package, error) {
	installedDir := config.Paths.Configs
	entries, err := os.ReadDir(installedDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("error reading installed directory: %w", err)
	}

	var packages []*Package
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		packageFile := filepath.Join(installedDir, entry.Name(), "package.yaml")
		data, err := os.ReadFile(packageFile)
		if err != nil {
			continue
		}

		pkg, err := LoadPackageFromBytes(data)
		if err != nil {
			continue
		}

		packages = append(packages, pkg)
	}

	return packages, nil
}

// InstalledMap indexes installed packages by name.
func InstalledMap(config *cnfg.Config) (map[string]*Package, error) {
	installed, err := LoadInstalledPackages(config)
	if err != nil {
		return nil, err
	}
	m := make(map[string]*Package, len(installed))
	for _, p := range installed {
		m[p.Name] = p
	}
	return m, nil
}

// HasUpdate reports whether the registry package differs from the installed one
// under the installed package's own method.
func HasUpdate(registry, installed *Package) bool {
	if registry == nil || installed == nil {
		return false
	}
	method := installed.InstallMethod
	if method == "" {
		method = MethodVersion
	}
	return registry.Ref(method) != installed.Ref(method)
}
