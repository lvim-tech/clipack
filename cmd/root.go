package cmd

import (
	"clipack/config"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var cfg config.Config

var rootCmd = &cobra.Command{
	Use:   "clipack",
	Short: "Clipack is a tool for managing applications",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println("Error executing root command:", err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
}

func initConfig() {
	cfg = config.InitConfig()
}
