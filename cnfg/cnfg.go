package cnfg

import (
	"bufio"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/lvim-tech/clipack/utils"
	"gopkg.in/yaml.v3"
	"github.com/spf13/viper"
)

// ConfigInit holds the initial configuration setup
type ConfigInit struct {
	InstallPath string
}

// RegistryConfig holds the configuration for the registry
type RegistryConfig struct {
	URL             string        `yaml:"url"`
	RegistryRepoURL string        `yaml:"registryRepoURL"`
	Token           string        `yaml:"token,omitempty"`
	Branch          string        `yaml:"branch"`
	UpdateInterval  time.Duration `yaml:"update_interval"`
}

// PathsConfig holds the configuration for various paths used in the application
type PathsConfig struct {
	Base     string `yaml:"base"`
	Registry string `yaml:"registry"`
	Bin      string `yaml:"bin"`
	Configs  string `yaml:"configs"`
	Build    string `yaml:"build"`
	Man      string `yaml:"man"`
}

// OptionsConfig holds the configuration for various options in the application
type OptionsConfig struct {
	AutoSymlink   bool   `yaml:"auto_symlink"`
	BackupConfigs bool   `yaml:"backup_configs"`
	CleanupBuild  bool   `yaml:"cleanup_build"`
	InstallMethod string `yaml:"install_method"` // New field for install method
}

// Config holds the entire configuration structure
type Config struct {
	Registry RegistryConfig `yaml:"registry"`
	Paths    PathsConfig    `yaml:"paths"`
	Options  OptionsConfig  `yaml:"options"`
}

// InitConfig initializes the configuration
func InitConfig() ConfigInit {
	home, err := os.UserHomeDir()
	if err != nil {
		panic(fmt.Errorf("failed to get user home directory: %w", err))
	}

	configDir := filepath.Join(home, ".config", "clipack")
	if err := os.MkdirAll(configDir, os.ModePerm); err != nil {
		panic(fmt.Errorf("failed to create config directory: %w", err))
	}

	configFile := filepath.Join(configDir, "config.yaml")
	viper.SetConfigFile(configFile)
	viper.SetConfigType("yaml")

	viper.SetDefault("install_path", filepath.Join(home, "clipack_apps"))
	viper.SetDefault("options.install_method", "version") // Set default value for install_method

	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		if err := viper.WriteConfigAs(configFile); err != nil {
			panic(fmt.Errorf("failed to write config file: %w", err))
		}
	} else {
		if err := viper.ReadInConfig(); err != nil {
			panic(fmt.Errorf("failed to read config file: %w", err))
		}
	}

	return ConfigInit{
		InstallPath: viper.GetString("install_path"),
	}
}

// LoadConfig loads the configuration from the config file
func LoadConfig() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("could not get home directory: %v", err)
	}

	configPath := filepath.Join(home, ".config", "clipack", "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("could not read config file: %v", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("could not parse config file: %v", err)
	}

	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %v", err)
	}

	return &config, nil
}

// validateConfig validates the configuration fields
func validateConfig(config *Config) error {
	if config.Registry.URL == "" {
		return fmt.Errorf("registry URL is required")
	}

	if config.Registry.Branch == "" {
		config.Registry.Branch = "main"
	}

	if config.Registry.UpdateInterval == 0 {
		config.Registry.UpdateInterval = 24 * time.Hour
	}

	paths := []string{config.Paths.Base, config.Paths.Registry, config.Paths.Bin, config.Paths.Configs, config.Paths.Build, config.Paths.Man}
	for _, path := range paths {
		if !filepath.IsAbs(path) {
			return fmt.Errorf("all paths must be absolute")
		}
	}

	return nil
}

// CreateDefaultConfig creates the default configuration file if it does not exist
func CreateDefaultConfig() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not get home directory: %v", err)
	}

	configDir := filepath.Join(home, ".config", "clipack")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("could not create config directory: %v", err)
	}

	configPath := filepath.Join(configDir, "config.yaml")

	if _, err := os.Stat(configPath); err == nil {
		return nil
	}

	installDir, err := AskInstallDirectory()
	if err != nil {
		return fmt.Errorf("could not get installation directory: %v", err)
	}

	config := fmt.Sprintf(`---
registry:
  url: https://github.com/lvim-tech/clipack-registry.git
  registryRepoURL: https://api.github.com/repos/lvim-tech/clipack-registry/contents
  branch: main
  update_interval: 24h
  # token: your-github-token # Optional: Add your GitHub token here

paths:
  base: %s
  registry: %s
  bin: %s
  configs: %s
  build: %s
  man: %s

options:
  auto_symlink: true
  backup_configs: true
  cleanup_build: true
  install_method: version
`,
		installDir,
		filepath.Join(installDir, "registry"),
		filepath.Join(installDir, "bin"),
		filepath.Join(installDir, "configs"),
		filepath.Join(installDir, "build"),
		filepath.Join(installDir, "man"))

	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		return fmt.Errorf("could not write config file: %v", err)
	}

	dirs := []string{
		filepath.Join(installDir, "registry"),
		filepath.Join(installDir, "bin"),
		filepath.Join(installDir, "configs"),
		filepath.Join(installDir, "build"),
		filepath.Join(installDir, "man"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("could not create directory %s: %v", dir, err)
		}
	}

	fmt.Printf("\nConfiguration created at: %s\n", configPath)
	fmt.Printf("Installation directory: %s\n", installDir)
	fmt.Printf("\nThe following directories have been created:\n")
	for _, dir := range dirs {
		fmt.Printf("- %s\n", dir)
	}

	if utils.AskForConfirmation("Do you want to add the bin and man paths to your shell configuration?") {
		if err := AddPathsToShellConfig(filepath.Join(installDir, "bin"), filepath.Join(installDir, "man")); err != nil {
			return fmt.Errorf("could not add paths to shell configuration: %v", err)
		}
	}

	return nil
}

// UpdateConfig updates the configuration file
func UpdateConfig() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not get home directory: %v", err)
	}

	configDir := filepath.Join(home, ".config", "clipack")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("could not create config directory: %v", err)
	}

	configPath := filepath.Join(configDir, "config.yaml")

	installDir, err := AskInstallDirectory()
	if err != nil {
		return fmt.Errorf("could not get installation directory: %v", err)
	}

	config := fmt.Sprintf(`---
registry:
  url: https://github.com/lvim-tech/clipack-registry.git
  registryRepoURL: https://api.github.com/repos/lvim-tech/clipack-registry/contents
  branch: main
  update_interval: 24h
  # token: your-github-token # Optional: Add your GitHub token here

paths:
  base: %s
  registry: %s
  bin: %s
  configs: %s
  build: %s
  man: %s

options:
  auto_symlink: true
  backup_configs: true
  cleanup_build: true
  install_method: version
`,
		installDir,
		filepath.Join(installDir, "registry"),
		filepath.Join(installDir, "bin"),
		filepath.Join(installDir, "configs"),
		filepath.Join(installDir, "build"),
		filepath.Join(installDir, "man"))

	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		return fmt.Errorf("could not write config file: %v", err)
	}

	fmt.Printf("Configuration updated at: %s\n", configPath)

	if utils.AskForConfirmation("Do you want to add the bin and man paths to your shell configuration?") {
		if err := AddPathsToShellConfig(filepath.Join(installDir, "bin"), filepath.Join(installDir, "man")); err != nil {
			return fmt.Errorf("could not add paths to shell configuration: %v", err)
		}
	}

	return nil
}

// GetCurrentUserAndTime returns the current user and time
func GetCurrentUserAndTime() (string, string) {
	currentTime := "2025-03-02 18:16:07"
	currentUser := "bojanbb"
	return currentUser, currentTime
}

// AskInstallDirectory prompts the user to input the installation directory
func AskInstallDirectory() (string, error) {
	reader := bufio.NewReader(os.Stdin)

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	defaultDir := filepath.Join(home, "clipack")

	fmt.Printf("\nWhere would you like to install clipack packages?\n")
	fmt.Printf("Default: %s\n", defaultDir)
	fmt.Printf("Enter path (or press Enter for default): ")

	input, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("error reading input: %v", err)
	}

	installDir := strings.TrimSpace(input)

	if installDir == "" {
		installDir = defaultDir
	}

	if strings.HasPrefix(installDir, "~/") {
		installDir = filepath.Join(home, installDir[2:])
	}

	return installDir, nil
}

// AddPathsToShellConfig adds the bin and man paths to the shell configuration file
func AddPathsToShellConfig(binPath, manPath string) error {
	configFilePath, err := GetShellConfigFilePath()
	if err != nil {
		return fmt.Errorf("could not determine shell config file path: %v", err)
	}

	configFile, err := os.OpenFile(configFilePath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("could not open shell config file: %v", err)
	}
	defer configFile.Close()

	shell := filepath.Base(os.Getenv("SHELL"))

	switch shell {
	case "bash", "zsh":
		if _, err := configFile.WriteString(fmt.Sprintf("\nexport PATH=\"%s:$PATH\"\nexport MANPATH=\"%s:$MANPATH\"\n", binPath, manPath)); err != nil {
			return fmt.Errorf("could not write to shell config file: %v", err)
		}
	case "fish":
		if _, err := configFile.WriteString(fmt.Sprintf("\nset -x PATH %s $PATH\nset -x MANPATH %s $MANPATH\n", binPath, manPath)); err != nil {
			return fmt.Errorf("could not write to shell config file: %v", err)
		}
	default:
		return fmt.Errorf("unsupported shell: %s", shell)
	}

	fmt.Printf("Paths added to %s\n", configFilePath)
	return nil
}

// GetShellConfigFilePath returns the path to the shell configuration file
func GetShellConfigFilePath() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("could not get current user: %v", err)
	}

	shell := filepath.Base(os.Getenv("SHELL"))

	var configFilePath string
	switch shell {
	case "bash":
		configFilePath = filepath.Join(usr.HomeDir, ".bashrc")
	case "zsh":
		configFilePath = filepath.Join(usr.HomeDir, ".zshrc")
	case "fish":
		configFilePath = filepath.Join(usr.HomeDir, ".config", "fish", "config.fish")
	default:
		return "", fmt.Errorf("unsupported shell: %s", shell)
	}

	return configFilePath, nil
}
