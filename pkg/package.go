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

// Install holds the installation steps and related data
type Install struct {
	Script           string             `yaml:"script,omitempty"`
	Commands         []string           `yaml:"commands,omitempty"`
	Files            []string           `yaml:"files,omitempty"`
	Deps             []string           `yaml:"deps,omitempty"`
	Environment      map[string]string  `yaml:"environment,omitempty"`
	Steps            []string           `yaml:"steps,omitempty"`
	Binaries         []string           `yaml:"binaries,omitempty"`
	Configs          []string           `yaml:"configs,omitempty"`
	Man              []string           `yaml:"man,omitempty"`
	AdditionalConfig []AdditionalConfig `yaml:"additional-config,omitempty"`
}

// AdditionalConfig holds additional configuration data
type AdditionalConfig struct {
	Filename string `yaml:"filename"`
	Content  string `yaml:"content"`
}

// PostInstall holds post-installation scripts
type PostInstall struct {
	Scripts []Script `yaml:"scripts,omitempty"`
}

// Script holds a script filename and its content
type Script struct {
	Filename string `yaml:"filename"`
	Content  string `yaml:"content"`
}

// Package holds the package data
type Package struct {
	Name        string      `yaml:"name"`
	Version     string      `yaml:"version"`
	Commit      string      `yaml:"commit"`
	Description string      `yaml:"description"`
	Maintainer  string      `yaml:"maintainer"`
	UpdatedAt   time.Time   `yaml:"updated_at"`
	Tags        []string    `yaml:"tags"`
	Category    string      `yaml:"-"`
	License     string      `yaml:"license"`
	Homepage    string      `yaml:"homepage"`
	Install     Install     `yaml:"install"`
	PostInstall PostInstall `yaml:"post-install,omitempty"`
	InstallMethod    string          `yaml:"install_method"`
}

// LoadAllPackagesFromDir loads all packages from a directory
func LoadAllPackagesFromDir(registryDir string) ([]*Package, error) {
	if _, err := os.Stat(registryDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("registry directory does not exist: %v", err)
	}

	entries, err := os.ReadDir(registryDir)
	if err != nil {
		return nil, fmt.Errorf("error reading registry directory: %v", err)
	}

	var packages []*Package
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yaml") {
			data, err := os.ReadFile(filepath.Join(registryDir, entry.Name()))
			if err != nil {
				fmt.Printf("Warning: error reading %s: %v\n", entry.Name(), err)
				continue
			}

			pkg, err := LoadPackageFromBytes(data)
			if err != nil {
				fmt.Printf("Warning: error parsing %s: %v\n", entry.Name(), err)
				continue
			}

			packages = append(packages, pkg)
		}
	}

	return packages, nil
}

// LoadPackageFromBytes loads a package from a byte array
func LoadPackageFromBytes(data []byte) (*Package, error) {
	var pkg Package
	if err := yaml.Unmarshal(data, &pkg); err != nil {
		return nil, err
	}
	return &pkg, nil
}

// LoadPackageFromReader loads a package from an io.Reader
func LoadPackageFromReader(r io.Reader) (*Package, error) {
	var pkg Package
	decoder := yaml.NewDecoder(r)
	if err := decoder.Decode(&pkg); err != nil {
		return nil, err
	}
	return &pkg, nil
}

// CopyFile copies a file from src to dst
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

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	if _, err := io.Copy(destination, source); err != nil {
		return err
	}

	return nil
}

// LoadInstalledPackages loads installed packages from the config directory
func LoadInstalledPackages(config *cnfg.Config) ([]*Package, error) {
	installedDir := config.Paths.Configs
	entries, err := os.ReadDir(installedDir)
	if err != nil {
		return nil, fmt.Errorf("error reading installed directory: %v", err)
	}

	var packages []*Package
	for _, entry := range entries {
		if entry.IsDir() {
			packageFile := filepath.Join(installedDir, entry.Name(), "package.yaml")
			data, err := os.ReadFile(packageFile)
			if err != nil {
				fmt.Printf("Warning: error reading %s: %v\n", packageFile, err)
				continue
			}

			pkg, err := LoadPackageFromBytes(data)
			if err != nil {
				fmt.Printf("Warning: error parsing %s: %v\n", packageFile, err)
				continue
			}

			packages = append(packages, pkg)
		}
	}

	return packages, nil
}
