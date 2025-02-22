package pkg

import (
	"bufio"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type RegistryConfig struct {
	URL            string        `yaml:"url"`
	Branch         string        `yaml:"branch"`
	UpdateInterval time.Duration `yaml:"update_interval"`
}

type PathsConfig struct {
	Base     string `yaml:"base"`
	Registry string `yaml:"registry"`
	Bin      string `yaml:"bin"`
	Configs  string `yaml:"configs"`
	Build    string `yaml:"build"`
	Man      string `yaml:"man"`
}

type OptionsConfig struct {
	AutoSymlink   bool `yaml:"auto_symlink"`
	BackupConfigs bool `yaml:"backup_configs"`
	CleanupBuild  bool `yaml:"cleanup_build"`
}

type Config struct {
	Registry RegistryConfig `yaml:"registry"`
	Paths    PathsConfig    `yaml:"paths"`
	Options  OptionsConfig  `yaml:"options"`
}

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

	validateConfig(&config)
	return &config, nil
}

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

	// Проверяваме дали конфигурационният файл вече съществува
	if _, err := os.Stat(configPath); err == nil {
		return nil // Файлът вече съществува
	}

	installDir, err := AskInstallDirectory()
	if err != nil {
		return fmt.Errorf("could not get installation directory: %v", err)
	}

	currentUser, currentTime := GetCurrentUserAndTime()

	config := fmt.Sprintf(`# Clipack configuration file
# Created: %s
# User: %s

registry:
  url: https://github.com/lvim-tech/clipack-registry.git
  branch: main
  update_interval: 24h

paths:
  # Base installation directory
  base: %s
  # Other paths as absolute paths
  registry: %s
  bin: %s
  configs: %s
  build: %s
  man: %s

options:
  auto_symlink: true
  backup_configs: true
  cleanup_build: true
`,
		currentTime,
		currentUser,
		installDir,
		filepath.Join(installDir, "registry"),
		filepath.Join(installDir, "bin"),
		filepath.Join(installDir, "configs"),
		filepath.Join(installDir, "build"),
		filepath.Join(installDir, "man"))

	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		return fmt.Errorf("could not write config file: %v", err)
	}

	// Създаваме необходимите директории
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

	if AskForConfirmation("Do you want to add the bin and man paths to your shell configuration?") {
		if err := AddPathsToShellConfig(filepath.Join(installDir, "bin"), filepath.Join(installDir, "man")); err != nil {
			return fmt.Errorf("could not add paths to shell configuration: %v", err)
		}
	}

	return nil
}

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

	currentUser, currentTime := GetCurrentUserAndTime()

	config := fmt.Sprintf(`# Clipack configuration file
# Updated: %s
# User: %s

registry:
  url: https://github.com/lvim-tech/clipack-registry.git
  branch: main
  update_interval: 24h

paths:
  # Base installation directory
  base: %s
  # Other paths as absolute paths
  registry: %s
  bin: %s
  configs: %s
  build: %s
  man: %s

options:
  auto_symlink: true
  backup_configs: true
  cleanup_build: true
`,
		currentTime,
		currentUser,
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

	if AskForConfirmation("Do you want to add the bin and man paths to your shell configuration?") {
		if err := AddPathsToShellConfig(filepath.Join(installDir, "bin"), filepath.Join(installDir, "man")); err != nil {
			return fmt.Errorf("could not add paths to shell configuration: %v", err)
		}
	}

	return nil
}

func GetCurrentUserAndTime() (string, string) {
	currentTime := time.Now().UTC().Format("2006-01-02 15:04:05")
	currentUser := "lvim-tech" // Specified username
	return currentUser, currentTime
}

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
	}

	fmt.Printf("Paths added to %s\n", configFilePath)
	return nil
}

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
