package cmd

import (
	"fmt"
	"log"

	"github.com/lvim-tech/clipack/pkg"
	"github.com/spf13/cobra"
)

var addExecutablesPathCmd = &cobra.Command{
	Use:   "add-executables-path",
	Short: "Add executables and man paths to your shell configuration",
	Run: func(cmd *cobra.Command, args []string) {
		// Създаваме конфигурационния файл, ако не съществува
		if err := pkg.CreateDefaultConfig(); err != nil {
			log.Fatalf("Error creating config file: %v", err)
		}

		// Зареждаме конфигурацията
		config, err := pkg.LoadConfig()
		if err != nil {
			log.Fatalf("Error loading config: %v", err)
		}

		binPath := config.Paths.Bin
		manPath := config.Paths.Man

		fmt.Printf("The following paths will be added to your shell configuration:\n")
		fmt.Printf("Executables (bin): %s\n", binPath)
		fmt.Printf("Man pages: %s\n", manPath)

		shellConfigFilePath, err := pkg.GetShellConfigFilePath()
		if err != nil {
			log.Fatalf("Error determining shell config file path: %v", err)
		}
		fmt.Printf("These paths will be added to: %s\n", shellConfigFilePath)

		if !pkg.AskForConfirmation("Do you want to proceed with adding these paths?") {
			fmt.Println("Operation cancelled.")
			return
		}

		if err := pkg.AddPathsToShellConfig(binPath, manPath); err != nil {
			log.Fatalf("Error adding paths to shell configuration: %v", err)
		}

		fmt.Println("Executables and man paths have been added to your shell configuration.")
	},
}

func init() {
	rootCmd.AddCommand(addExecutablesPathCmd)
}
