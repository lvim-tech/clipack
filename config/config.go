package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

type Config struct {
	InstallPath string
}

func InitConfig() Config {
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

	// Set the default values
	viper.SetDefault("install_path", filepath.Join(home, "clipack_apps"))

	// Check if the config file exists; if not, create it
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		if err := viper.WriteConfigAs(configFile); err != nil {
			panic(fmt.Errorf("failed to write config file: %w", err))
		}
	} else {
		// File exists, so read it.
		if err := viper.ReadInConfig(); err != nil {
			panic(fmt.Errorf("failed to read config file: %w", err))
		}
	}

	return Config{
		InstallPath: viper.GetString("install_path"),
	}
}
