package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "clipack",
	Short: "Clipack is a command-line package manager for installing and managing CLI tools and configurations.",
	Long: `Clipack is a command-line package manager for installing
and managing CLI tools and configurations.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
}

func initConfig() {
	// Here you can initialize your config if needed
}
