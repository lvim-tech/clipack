package cmd

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lvim-tech/clipack/pkg"
	"github.com/spf13/cobra"
)

var forceRefreshInUpdate bool

var updateCmd = &cobra.Command{
	Use:   "update [package-name]",
	Short: "Check for updates to installed packages",
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

		var packages []*pkg.Package
		if forceRefreshInUpdate {
			fmt.Println("Forcing refresh of the registry cache...")

			// Изтриваме кеш файловете
			cachePath := pkg.GetCacheFilePath(config)
			os.Remove(cachePath)
			timestampPath := filepath.Join(config.Paths.Registry, "cache_timestamp.gob")
			os.Remove(timestampPath)

			packages, err = pkg.LoadAllPackagesFromRegistry(config)
			if err != nil {
				log.Fatalf("Error loading packages from registry: %v", err)
			}
			fmt.Println("Packages loaded from registry:", packages)

			// Запазваме в кеша
			if err := pkg.SaveToCache(packages, config); err != nil {
				log.Printf("Warning: could not cache packages: %v", err)
			} else {
				fmt.Println("Packages saved to cache successfully.")
			}
		} else {
			// Първо се опитваме да заредим от кеша
			packages, err = pkg.LoadFromCache(config)
			if err != nil {
				// Ако няма кеш или е изтекъл, зареждаме от GitHub
				fmt.Println("Fetching packages from registry...")
				packages, err = pkg.LoadAllPackagesFromRegistry(config)
				if err != nil {
					log.Fatalf("Error loading packages from registry: %v", err)
				}

				// Запазваме в кеша
				if err := pkg.SaveToCache(packages, config); err != nil {
					log.Printf("Warning: could not cache packages: %v", err)
				}
			}
		}

		if len(packages) == 0 {
			log.Fatalf("No packages found in registry")
		}

		installedPackages, err := pkg.LoadInstalledPackages(config)
		if err != nil {
			log.Fatalf("Error loading installed packages: %v", err)
		}

		if len(args) > 0 {
			packageName := args[0]
			var selectedPackage *pkg.Package

			for _, installed := range installedPackages {
				if installed.Name == packageName {
					for _, p := range packages {
						if p.Name == installed.Name && p.Version != installed.Version {
							selectedPackage = p
							break
						}
					}
					break
				}
			}

			if selectedPackage == nil {
				fmt.Printf("No updates available for package %s\n", packageName)
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

			if !pkg.AskForConfirmation("Proceed with update?") {
				fmt.Println("Update cancelled.")
				return
			}

			fmt.Println("\nUpdating package:", selectedPackage.Name)

			// Премахваме съществуващата инсталация
			existingConfigDir := filepath.Join(config.Paths.Configs, selectedPackage.Name)
			if err := os.RemoveAll(existingConfigDir); err != nil {
				log.Printf("Warning: could not remove existing config directory %s: %v", existingConfigDir, err)
			}

			for _, binPath := range selectedPackage.Install.Binaries {
				existingBinFile := filepath.Join(config.Paths.Bin, filepath.Base(binPath))
				if err := os.Remove(existingBinFile); err != nil {
					log.Printf("Warning: could not remove existing binary %s: %v", existingBinFile, err)
				}
			}

			// Инсталираме новата версия
			installCmd := exec.Command("clipack", "install", selectedPackage.Name)
			installCmd.Stdout = os.Stdout
			installCmd.Stderr = os.Stderr
			if err := installCmd.Run(); err != nil {
				log.Fatalf("Error installing package %s: %v", selectedPackage.Name, err)
			}

			return
		}

		fmt.Println("\nPackages with updates available:")
		fmt.Println("-------------------------------")

		var updatesAvailable []*pkg.Package
		for _, installed := range installedPackages {
			for _, p := range packages {
				if p.Name == installed.Name && p.Version != installed.Version {
					tags := strings.Join(p.Tags, ", ")
					if tags == "" {
						tags = "-"
					}

					fmt.Printf("\n%d) Name: %s\n", len(updatesAvailable)+1, p.Name)
					fmt.Printf("Current Version: %s\n", installed.Version)
					fmt.Printf("Available Version: %s\n", p.Version)
					fmt.Printf("Description: %s\n", p.Description)
					fmt.Printf("Maintainer: %s\n", p.Maintainer)
					if p.License != "" {
						fmt.Printf("License: %s\n", p.License)
					}
					if p.Homepage != "" {
						fmt.Printf("Homepage: %s\n", p.Homepage)
					}
					fmt.Printf("Tags: %s\n", tags)
					fmt.Printf("Updated: %s\n", p.UpdatedAt.Format("2006-01-02 15:04:05"))

					updatesAvailable = append(updatesAvailable, p)
				}
			}
		}

		if len(updatesAvailable) == 0 {
			fmt.Println("All packages are up to date.")
			return
		}

		reader := bufio.NewReader(os.Stdin)
		for {
			fmt.Print("\nEnter package number to update (or 'q' to quit): ")
			input, err := reader.ReadString('\n')
			if err != nil {
				log.Fatalf("Error reading input: %v", err)
			}

			input = strings.TrimSpace(input)
			if input == "q" {
				fmt.Println("Update cancelled.")
				return
			}

			num, err := strconv.Atoi(input)
			if err != nil || num < 1 || num > len(updatesAvailable) {
				fmt.Printf("Please enter a number between 1 and %d\n", len(updatesAvailable))
				continue
			}

			p := updatesAvailable[num-1]
			fmt.Println("\nSelected package details:")
			fmt.Println("------------------------")
			fmt.Printf("Name: %s\n", p.Name)
			fmt.Printf("Version: %s\n", p.Version)
			fmt.Printf("Description: %s\n", p.Description)
			fmt.Printf("Maintainer: %s\n", p.Maintainer)
			if p.License != "" {
				fmt.Printf("License: %s\n", p.License)
			}
			if p.Homepage != "" {
				fmt.Printf("Homepage: %s\n", p.Homepage)
			}
			fmt.Printf("Tags: %s\n", strings.Join(p.Tags, ", "))
			fmt.Printf("Updated: %s\n\n", p.UpdatedAt.Format("2006-01-02 15:04:05"))

			if !pkg.AskForConfirmation("Proceed with update?") {
				fmt.Println("Update cancelled.")
				continue
			}

			fmt.Println("\nUpdating package:", p.Name)

			// Премахваме съществуващата инсталация
			existingConfigDir := filepath.Join(config.Paths.Configs, p.Name)
			if err := os.RemoveAll(existingConfigDir); err != nil {
				log.Printf("Warning: could not remove existing config directory %s: %v", existingConfigDir, err)
			}

			for _, binPath := range p.Install.Binaries {
				existingBinFile := filepath.Join(config.Paths.Bin, filepath.Base(binPath))
				if err := os.Remove(existingBinFile); err != nil {
					log.Printf("Warning: could not remove existing binary %s: %v", existingBinFile, err)
				}
			}

			// Инсталираме новата версия
			installCmd := exec.Command("clipack", "install", p.Name)
			installCmd.Stdout = os.Stdout
			installCmd.Stderr = os.Stderr
			if err := installCmd.Run(); err != nil {
				log.Fatalf("Error installing package %s: %v", p.Name, err)
			}

			break
		}
	},
}

func init() {
	updateCmd.Flags().BoolVarP(&forceRefreshInUpdate, "force-refresh", "f", false, "Force refresh of the registry cache")
	rootCmd.AddCommand(updateCmd)
}
