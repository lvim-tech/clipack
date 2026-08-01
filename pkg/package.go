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

// Source describes where the package sources come from.
type Source struct {
	Type string `yaml:"type,omitempty"`
	URL  string `yaml:"url,omitempty"`
	Ref  string `yaml:"ref,omitempty"`
}

// Install holds the installation steps and related data.
type Install struct {
	Source           Source             `yaml:"source,omitempty"`
	Environment      map[string]string  `yaml:"environment,omitempty"`
	Steps            []string           `yaml:"steps,omitempty"`
	Binaries         []string           `yaml:"binaries,omitempty"`
	Configs          []string           `yaml:"configs,omitempty"`
	Man              []string           `yaml:"man,omitempty"`
	AdditionalConfig []AdditionalConfig `yaml:"additional-config,omitempty"`
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
	Name          string      `yaml:"name"`
	Version       string      `yaml:"version"`
	Commit        string      `yaml:"commit"`
	Description   string      `yaml:"description"`
	Maintainer    string      `yaml:"maintainer"`
	UpdatedAt     time.Time   `yaml:"updated_at"`
	Tags          []string    `yaml:"tags"`
	Category      string      `yaml:"category,omitempty"`
	License       string      `yaml:"license"`
	Homepage      string      `yaml:"homepage"`
	Install       Install     `yaml:"install"`
	PostInstall   PostInstall `yaml:"post-install,omitempty"`
	InstallMethod string      `yaml:"install_method,omitempty"`
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
