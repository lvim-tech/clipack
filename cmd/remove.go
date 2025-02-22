package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/lvim-tech/clipack/pkg"
	"github.com/spf13/cobra"
)

var removeCmd = &cobra.Command{
	Use:   "remove [package-name]",
	Short: "Remove an installed package",
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

		installedPackages, err := pkg.LoadInstalledPackages(config)
		if err != nil {
			log.Fatalf("Error loading installed packages: %v", err)
		}

		if len(args) == 0 {
			// Ако няма предоставено име на пакет, листваме всички инсталирани пакети
			fmt.Println("\nInstalled packages:")
			fmt.Println("-------------------")
			for i, p := range installedPackages {
				fmt.Printf("%d) %s (%s)\n", i+1, p.Name, p.Version)
			}
			return
		}

		packageName := args[0]
		var selectedPackage *pkg.Package
		for _, installed := range installedPackages {
			if installed.Name == packageName {
				selectedPackage = installed
				break
			}
		}

		if selectedPackage == nil {
			fmt.Printf("Package %s is not installed\n", packageName)
			return
		}

		fmt.Println("\nSelected package details:")
		fmt.Println("------------------------")
		fmt.Printf("Name: %s\n", selectedPackage.Name)
		fmt.Printf("Version: %s\n", selectedPackage.Version)
		fmt.Printf("Description: %s\n", selectedPackage.Description)
		fmt.Printf("Maintainer: %s\n", selectedPackage.Maintainer)
		if selectedPackage.License != "" {
			fmt.Printf("License: %s\n", selectedPackage.License)
		}
		if selectedPackage.Homepage != "" {
			fmt.Printf("Homepage: %s\n", selectedPackage.Homepage)
		}
		fmt.Printf("Tags: %s\n", strings.Join(selectedPackage.Tags, ", "))
		fmt.Printf("Updated: %s\n\n", selectedPackage.UpdatedAt.Format("2006-01-02 15:04:05"))

		if !pkg.AskForConfirmation("Proceed with removal?") {
			fmt.Println("Removal cancelled.")
			return
		}

		fmt.Println("\nRemoving package:", selectedPackage.Name)

		// Премахваме конфигурационните файлове
		existingConfigDir := filepath.Join(config.Paths.Configs, selectedPackage.Name)
		if err := os.RemoveAll(existingConfigDir); err != nil {
			log.Printf("Warning: could not remove existing config directory %s: %v", existingConfigDir, err)
		} else {
			fmt.Printf("Removed config directory %s\n", existingConfigDir)
		}

		// Премахваме бинарните файлове
		for _, binPath := range selectedPackage.Install.Binaries {
			existingBinFile := filepath.Join(config.Paths.Bin, filepath.Base(binPath))
			if err := os.Remove(existingBinFile); err != nil {
				log.Printf("Warning: could not remove existing binary %s: %v", existingBinFile, err)
			} else {
				fmt.Printf("Removed binary %s\n", existingBinFile)
			}
		}

		// Премахваме man страниците
		for _, manPage := range selectedPackage.Install.Man {
			existingManFile := filepath.Join(config.Paths.Man, filepath.Base(manPage))
			if err := os.Remove(existingManFile); err != nil {
				log.Printf("Warning: could not remove existing man page %s: %v", existingManFile, err)
			} else {
				fmt.Printf("Removed man page %s\n", existingManFile)
			}
		}

		// Премахваме файловете от post-install скриптовете
		for _, script := range selectedPackage.PostInstall.Scripts {
			scriptPath := filepath.Join(config.Paths.Bin, script.Filename)
			if err := os.Remove(scriptPath); err != nil {
				log.Printf("Warning: could not remove post-install script %s: %v", scriptPath, err)
			} else {
				fmt.Printf("Removed post-install script %s\n", scriptPath)
			}
		}

		fmt.Printf("\nSuccessfully removed %s\n", selectedPackage.Name)
	},
}

func init() {
	rootCmd.AddCommand(removeCmd)
}
