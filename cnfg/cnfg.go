// Package cnfg loads and writes clipack's configuration.
//
// The configuration lives at ~/.config/clipack/config.yaml and names three
// things: where the registry is, where packages are installed, and the defaults
// for install operations.
//
// There are two ways to create one. CreateDefaultConfig prompts on stdin and is
// used by the CLI; interactive front-ends call NewDefaultConfig followed by
// Save, so they can collect the install directory their own way.
package cnfg

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lvim-tech/clipack/utils"
	"gopkg.in/yaml.v3"
)

// Default registry endpoints used when a config is created from scratch.
const (
	DefaultRegistryURL     = "https://github.com/lvim-tech/clipack-registry.git"
	DefaultRegistryRepoURL = "https://api.github.com/repos/lvim-tech/clipack-registry/contents"
	DefaultBranch          = "main"
	DefaultUpdateInterval  = 24 * time.Hour
)

// RegistryConfig holds the configuration for the registry.
type RegistryConfig struct {
	URL             string        `yaml:"url"`
	RegistryRepoURL string        `yaml:"registryRepoURL"`
	Token           string        `yaml:"token,omitempty"`
	Branch          string        `yaml:"branch"`
	UpdateInterval  time.Duration `yaml:"update_interval"`
}

// PathsConfig holds the configuration for various paths used in the application.
type PathsConfig struct {
	Base     string `yaml:"base"`
	Registry string `yaml:"registry"`
	Bin      string `yaml:"bin"`
	Configs  string `yaml:"configs"`
	Build    string `yaml:"build"`
	Man      string `yaml:"man"`
}

// OptionsConfig holds the configuration for various options in the application.
type OptionsConfig struct {
	AutoSymlink   bool   `yaml:"auto_symlink"`
	BackupConfigs bool   `yaml:"backup_configs"`
	CleanupBuild  bool   `yaml:"cleanup_build"`
	InstallMethod string `yaml:"install_method"`
}

// Config holds the entire configuration structure.
type Config struct {
	Registry RegistryConfig `yaml:"registry"`
	Paths    PathsConfig    `yaml:"paths"`
	Options  OptionsConfig  `yaml:"options"`
	Theme    Theme          `yaml:"theme"`
}

// ConfigDir returns the directory holding clipack's own configuration.
func ConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not get home directory: %w", err)
	}
	return filepath.Join(home, ".config", "clipack"), nil
}

// ConfigPath returns the path to config.yaml.
func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.yaml"), nil
}

// Exists reports whether a configuration file is already present.
func Exists() bool {
	path, err := ConfigPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// DefaultInstallDir is the suggested base directory for packages.
func DefaultInstallDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "clipack"
	}
	return filepath.Join(home, "clipack")
}

// ExpandPath resolves ~ and makes the path absolute.
func ExpandPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	if strings.HasPrefix(path, "~") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

// NewDefaultConfig builds a configuration rooted at installDir.
func NewDefaultConfig(installDir string) *Config {
	installDir = ExpandPath(installDir)
	return &Config{
		Registry: RegistryConfig{
			URL:             DefaultRegistryURL,
			RegistryRepoURL: DefaultRegistryRepoURL,
			Branch:          DefaultBranch,
			UpdateInterval:  DefaultUpdateInterval,
		},
		Paths: PathsConfig{
			Base:     installDir,
			Registry: filepath.Join(installDir, "registry"),
			Bin:      filepath.Join(installDir, "bin"),
			Configs:  filepath.Join(installDir, "configs"),
			Build:    filepath.Join(installDir, "build"),
			Man:      filepath.Join(installDir, "man"),
		},
		Options: OptionsConfig{
			AutoSymlink:   true,
			BackupConfigs: true,
			CleanupBuild:  true,
			InstallMethod: "version",
		},
		// Written out explicitly so the knob is discoverable in the file.
		Theme: Theme{Name: DefaultThemeName},
	}
}

// Save writes the configuration and creates the directory tree it describes.
func (c *Config) Save() error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("could not create config directory: %w", err)
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("could not encode config: %w", err)
	}

	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, append([]byte("---\n"), data...), 0o644); err != nil {
		return fmt.Errorf("could not write config file: %w", err)
	}

	return c.EnsureDirs()
}

// EnsureDirs creates every directory referenced by the configuration.
func (c *Config) EnsureDirs() error {
	for _, dir := range c.Dirs() {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("could not create directory %s: %w", dir, err)
		}
	}
	return nil
}

// Dirs lists the managed directories.
func (c *Config) Dirs() []string {
	return []string{c.Paths.Registry, c.Paths.Bin, c.Paths.Configs, c.Paths.Build, c.Paths.Man}
}

// LoadConfig loads the configuration from the config file.
func LoadConfig() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("could not parse config file: %w", err)
	}

	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &config, nil
}

// validateConfig fills in defaults and rejects unusable configurations.
func validateConfig(config *Config) error {
	if config.Registry.URL == "" {
		return fmt.Errorf("registry URL is required")
	}
	if config.Registry.Branch == "" {
		config.Registry.Branch = DefaultBranch
	}
	if config.Registry.UpdateInterval == 0 {
		config.Registry.UpdateInterval = DefaultUpdateInterval
	}
	if config.Options.InstallMethod == "" {
		config.Options.InstallMethod = "version"
	}

	named := map[string]string{
		"base":     config.Paths.Base,
		"registry": config.Paths.Registry,
		"bin":      config.Paths.Bin,
		"configs":  config.Paths.Configs,
		"build":    config.Paths.Build,
		"man":      config.Paths.Man,
	}
	for name, path := range named {
		if path == "" {
			return fmt.Errorf("path %q is not set", name)
		}
		if !filepath.IsAbs(path) {
			return fmt.Errorf("path %q must be absolute, got %q", name, path)
		}
	}

	if err := validateTheme(&config.Theme); err != nil {
		return fmt.Errorf("theme: %w", err)
	}

	return nil
}

// CreateDefaultConfig creates the default configuration file if it does not
// exist, prompting on stdin. Interactive front-ends should use NewDefaultConfig
// and Save instead.
func CreateDefaultConfig() error {
	if Exists() {
		return nil
	}

	installDir, err := AskInstallDirectory()
	if err != nil {
		return fmt.Errorf("could not get installation directory: %w", err)
	}

	config := NewDefaultConfig(installDir)
	if err := config.Save(); err != nil {
		return err
	}

	path, _ := ConfigPath()
	fmt.Printf("\nConfiguration created at: %s\n", path)
	fmt.Printf("Installation directory: %s\n", config.Paths.Base)
	fmt.Printf("\nThe following directories have been created:\n")
	for _, dir := range config.Dirs() {
		fmt.Printf("- %s\n", dir)
	}

	if utils.AskForConfirmation("Do you want to add the bin and man paths to your shell configuration?") {
		if err := AddPathsToShellConfig(config.Paths.Bin, config.Paths.Man); err != nil {
			return fmt.Errorf("could not add paths to shell configuration: %w", err)
		}
	}

	return nil
}

// UpdateConfig repoints the installation directory while preserving settings
// the user customised. The previous version rewrote the file from a hardcoded
// template, silently discarding the registry token and every option.
func UpdateConfig() error {
	installDir, err := AskInstallDirectory()
	if err != nil {
		return fmt.Errorf("could not get installation directory: %w", err)
	}

	config := NewDefaultConfig(installDir)
	if existing, err := LoadConfig(); err == nil {
		config.Registry = existing.Registry
		config.Options = existing.Options
	}

	if err := config.Save(); err != nil {
		return err
	}

	path, _ := ConfigPath()
	fmt.Printf("Configuration updated at: %s\n", path)

	if utils.AskForConfirmation("Do you want to add the bin and man paths to your shell configuration?") {
		if err := AddPathsToShellConfig(config.Paths.Bin, config.Paths.Man); err != nil {
			return fmt.Errorf("could not add paths to shell configuration: %w", err)
		}
	}

	return nil
}

// AskInstallDirectory prompts the user to input the installation directory.
// It shares utils' stdin reader so a following confirmation prompt still sees
// its own answer when input is piped in.
func AskInstallDirectory() (string, error) {
	defaultDir := DefaultInstallDir()

	fmt.Printf("\nWhere would you like to install clipack packages?\n")
	fmt.Printf("Default: %s\n", defaultDir)
	fmt.Printf("Enter path (or press Enter for default): ")

	input, err := utils.ReadLine()
	if err != nil && input == "" {
		return "", fmt.Errorf("error reading input: %w", err)
	}

	installDir := strings.TrimSpace(input)
	if installDir == "" {
		installDir = defaultDir
	}

	return ExpandPath(installDir), nil
}
