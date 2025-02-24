package cmd

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lvim-tech/clipack/cnfg"
	"github.com/lvim-tech/clipack/pkg"
	"github.com/lvim-tech/clipack/utils"
	"github.com/spf13/cobra"
)

var removeCmd = &cobra.Command{
	Use:   "remove [package-name]",
	Short: "Remove an installed package",
	Run: func(cmd *cobra.Command, args []string) {
		if err := cnfg.CreateDefaultConfig(); err != nil {
			log.Fatalf("Error creating config file: %v", err)
		}

		config, err := cnfg.LoadConfig()
		if err != nil {
			log.Fatalf("Error loading config: %v", err)
		}

		installedPackages, err := pkg.LoadInstalledPackages(config)
		if err != nil {
			log.Fatalf("Error loading installed packages: %v", err)
		}

		if len(installedPackages) == 0 {
			fmt.Println("No packages are installed.")
			return
		}

		if len(args) == 0 {
			fmt.Println("\nInstalled packages:")
			fmt.Println("-------------------")
			for i, p := range installedPackages {
				fmt.Printf("%d) %s (%s)\n", i+1, p.Name, p.Version)
			}

			reader := bufio.NewReader(os.Stdin)
			for {
				fmt.Print("\nEnter package number to remove (or 'q' to quit): ")
				input, err := reader.ReadString('\n')
				if err != nil {
					log.Fatalf("Error reading input: %v", err)
				}

				input = strings.TrimSpace(input)
				if input == "q" {
					fmt.Println("Removal cancelled.")
					return
				}

				num, err := strconv.Atoi(input)
				if err != nil || num < 1 || num > len(installedPackages) {
					fmt.Printf("Please enter a number between 1 and %d\n", len(installedPackages))
					continue
				}

				selectedPackage := installedPackages[num-1]
				removePackage(selectedPackage, config)
				break
			}
		} else {
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

			removePackage(selectedPackage, config)
		}
	},
}

func removePackage(selectedPackage *pkg.Package, config *cnfg.Config) {
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

	if !utils.AskForConfirmation("Proceed with removal?") {
		fmt.Println("Removal cancelled.")
		return
	}

	fmt.Println("\nRemoving package:", selectedPackage.Name)

	existingConfigDir := filepath.Join(config.Paths.Configs, selectedPackage.Name)
	if err := os.RemoveAll(existingConfigDir); err != nil {
		log.Printf("Warning: could not remove existing config directory %s: %v", existingConfigDir, err)
	} else {
		fmt.Printf("Removed config directory %s\n", existingConfigDir)
	}

	for _, binPath := range selectedPackage.Install.Binaries {
		existingBinFile := filepath.Join(config.Paths.Bin, filepath.Base(binPath))
		if err := os.Remove(existingBinFile); err != nil {
			log.Printf("Warning: could not remove existing binary %s: %v", existingBinFile, err)
		} else {
			fmt.Printf("Removed binary %s\n", existingBinFile)
		}
	}

	for _, manPage := range selectedPackage.Install.Man {
		ext := filepath.Ext(manPage)
		if len(ext) < 2 {
			log.Printf("Warning: could not determine section for %s", manPage)
			continue
		}

		section := "man" + ext[1:]
		sectionDir := filepath.Join(config.Paths.Man, section)
		existingManFile := filepath.Join(sectionDir, filepath.Base(manPage))

		if err := os.Remove(existingManFile); err != nil {
			log.Printf("Warning: could not remove existing man page %s: %v", existingManFile, err)
		} else {
			fmt.Printf("Removed man page %s\n", existingManFile)
		}
	}

	for _, script := range selectedPackage.PostInstall.Scripts {
		scriptPath := filepath.Join(config.Paths.Bin, script.Filename)
		if err := os.Remove(scriptPath); err != nil {
			log.Printf("Warning: could not remove post-install script %s: %v", scriptPath, err)
		} else {
			fmt.Printf("Removed post-install script %s\n", scriptPath)
		}
	}

	fmt.Printf("\nSuccessfully removed %s\n", selectedPackage.Name)
}

func init() {
	rootCmd.AddCommand(removeCmd)
}
