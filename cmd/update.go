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
	"time"

	"github.com/lvim-tech/clipack/cnfg"
	"github.com/lvim-tech/clipack/pkg"
	"github.com/lvim-tech/clipack/utils"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	forceRefreshInUpdate bool
	useLatestInUpdate    bool
)

var updateCmd = &cobra.Command{
	Use:   "update [package-name]",
	Short: "Check for updates to installed packages",
	Run: func(cmd *cobra.Command, args []string) {
		if err := cnfg.CreateDefaultConfig(); err != nil {
			log.Fatalf("Error creating config file: %v", err)
		}

		config, err := cnfg.LoadConfig()
		if err != nil {
			log.Fatalf("Error loading config: %v", err)
		}

		var packages []*pkg.Package
		if forceRefreshInUpdate {
			fmt.Println("Forcing refresh of the registry cache...")

			cachePath := pkg.GetCacheFilePath(config)
			os.Remove(cachePath)
			timestampPath := filepath.Join(config.Paths.Registry, "cache_timestamp.gob")
			os.Remove(timestampPath)

			packages, err = pkg.LoadAllPackagesFromRegistry(config)
			if err != nil {
				log.Fatalf("Error loading packages: %v", err)
			}

			if err := pkg.SaveToCache(packages, config); err != nil {
				log.Printf("Warning: could not cache packages: %v", err)
			} else {
				fmt.Println("Packages saved to cache successfully.")
			}
		} else {
			cachePath := pkg.GetCacheFilePath(config)
			if _, err := os.Stat(cachePath); os.IsNotExist(err) {
				fmt.Println("Cache not found. Fetching packages from registry...")
				packages, err = pkg.LoadAllPackagesFromRegistry(config)
				if err != nil {
					log.Fatalf("Error loading packages: %v", err)
				}

				if err := pkg.SaveToCache(packages, config); err != nil {
					log.Printf("Warning: could not cache packages: %v", err)
				}
			} else {
				packages, err = pkg.LoadFromCache(config)
				if err != nil {
					log.Fatalf("Error loading packages from cache: %v", err)
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
			var installedPackage *pkg.Package

			// Намиране на инсталирания пакет
			for _, installed := range installedPackages {
				if installed.Name == packageName {
					installedPackage = installed
					break
				}
			}

			if installedPackage == nil {
				fmt.Printf("Package %s is not installed\n", packageName)
				return
			}

			// Намиране на последната версия
			for _, p := range packages {
				if p.Name == packageName {
					selectedPackage = p
					break
				}
			}

			if selectedPackage == nil {
				fmt.Printf("Package %s not found in registry\n", packageName)
				return
			}

			// Проверка дали е нужен ъпдейт
			shouldUpdate := false
			updateReason := ""
			latestCommit := ""

			if installedPackage.Installation.Method == "latest" {
				// Получаване на последния комит от онлайн хостването на пакета
				cmd := exec.Command("git", "ls-remote", selectedPackage.Homepage, "HEAD")
				out, err := cmd.Output()
				if err != nil {
					log.Printf("Error getting latest commit for %s: %v", selectedPackage.Name, err)
				}
				latestCommit = strings.Fields(string(out))[0]

				if latestCommit != installedPackage.Installation.ActualVersion {
					shouldUpdate = true
					updateReason = fmt.Sprintf("new commit available (%s -> %s)", installedPackage.Installation.ActualVersion, latestCommit)
				}
			} else {
				if selectedPackage.Version != installedPackage.Installation.ActualVersion {
					shouldUpdate = true
					updateReason = fmt.Sprintf("new version available (%s -> %s)", installedPackage.Installation.ActualVersion, selectedPackage.Version)
				}
			}

			// Поправка за показване на текущата версия като "latest"
			currentVersion := installedPackage.Installation.ActualVersion
			if installedPackage.Installation.Method == "latest" {
				currentVersion = "latest"
			}

			if !shouldUpdate {
				fmt.Printf("%s is up to date (version %s)\n", packageName, currentVersion)
				return
			}

			fmt.Println("\nSelected package details:")
			fmt.Println("------------------------")
			fmt.Printf("Name: %s\n", selectedPackage.Name)
			fmt.Printf("Current Version: %s\n", currentVersion)
			if installedPackage.Installation.Method == "latest" {
				fmt.Printf("New Commit: %s\n", latestCommit)
			} else {
				fmt.Printf("New Version: %s\n", selectedPackage.Version)
			}
			fmt.Printf("Description: %s\n", selectedPackage.Description)
			fmt.Printf("Maintainer: %s\n", selectedPackage.Maintainer)
			if selectedPackage.License != "" {
				fmt.Printf("License: %s\n", selectedPackage.License)
			}
			if selectedPackage.Homepage != "" {
				fmt.Printf("Homepage: %s\n", selectedPackage.Homepage)
			}
			fmt.Printf("Tags: %s\n", strings.Join(selectedPackage.Tags, ", "))
			fmt.Printf("Updated: %s\n", selectedPackage.UpdatedAt.Format("2006-01-02 15:04:05"))
			fmt.Printf("Update Reason: %s\n\n", updateReason)

			if !utils.AskForConfirmation("Proceed with update?") {
				fmt.Println("Update cancelled.")
				return
			}

			fmt.Println("\nUpdating package:", selectedPackage.Name)

			// Почистване на съществуващите файлове
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

			// Четене и почистване на предишната инсталация
			packageConfigPath := filepath.Join(config.Paths.Configs, selectedPackage.Name, "package.yaml")
			packageData, err := os.ReadFile(packageConfigPath)
			if err == nil {
				var previousPackage pkg.Package
				if err := yaml.Unmarshal(packageData, &previousPackage); err == nil {
					for _, binPath := range previousPackage.Install.Binaries {
						existingBinFile := filepath.Join(config.Paths.Bin, filepath.Base(binPath))
						if err := os.Remove(existingBinFile); err != nil {
							log.Printf("Warning: could not remove existing binary %s: %v", existingBinFile, err)
						}
					}

					for _, confPath := range previousPackage.Install.Configs {
						existingConfFile := filepath.Join(config.Paths.Configs, previousPackage.Name, filepath.Base(confPath))
						if err := os.Remove(existingConfFile); err != nil {
							log.Printf("Warning: could not remove existing config %s: %v", existingConfFile, err)
						}
					}

					for _, manPage := range previousPackage.Install.Man {
						existingManFile := filepath.Join(config.Paths.Man, filepath.Base(manPage))
						if err := os.Remove(existingManFile); err != nil {
							log.Printf("Warning: could not remove existing man page %s: %v", existingManFile, err)
						}
					}
				}
			}

			binDir := config.Paths.Bin
			configDir := filepath.Join(config.Paths.Configs, selectedPackage.Name)
			buildDir := filepath.Join(config.Paths.Build, selectedPackage.Name)
			manDir := config.Paths.Man

			if _, err := os.Stat(buildDir); err == nil {
				if utils.AskForConfirmation(fmt.Sprintf("Build directory %s exists. Remove it?", buildDir)) {
					if err := os.RemoveAll(buildDir); err != nil {
						log.Fatalf("Error removing build directory: %v", err)
					}
				} else {
					fmt.Println("Installation cancelled.")
					return
				}
			}

			for _, dir := range []string{binDir, configDir, buildDir, manDir} {
				if err := utils.EnsureDirectoryExists(dir); err != nil {
					log.Fatalf("Error creating directory %s: %v", dir, err)
				}
			}

			if err := os.Chdir(buildDir); err != nil {
				log.Fatalf("Error changing to directory %s: %v", buildDir, err)
			}

			// Обновяване на installation информацията
			if installedPackage.Installation.Method == "latest" {
				cmd := exec.Command("git", "rev-parse", "HEAD")
				cmd.Dir = buildDir
				out, err := cmd.Output()
				if err != nil {
					log.Fatalf("Error getting current commit: %v", err)
				}
				commit := strings.TrimSpace(string(out))
				selectedPackage.Installation.Method = "latest"
				selectedPackage.Installation.ActualVersion = commit
			}

			for k, v := range selectedPackage.Install.Environment {
				os.Setenv(k, v)
			}

			for _, step := range selectedPackage.Install.Steps {
				if installedPackage.Installation.Method == "latest" && strings.Contains(step, "git clone") {
					step = strings.Replace(step, "--branch v", "", 1)
					step = strings.Replace(step, "0.8.1", "", 1)
					step = strings.Replace(step, "--single-branch", "", 1)
				}
				fmt.Printf("Executing: %s\n", step)
				cmdParts := strings.Fields(step)
				cmd := exec.Command(cmdParts[0], cmdParts[1:]...)
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				if err := cmd.Run(); err != nil {
					log.Fatalf("Error executing step '%s': %v", step, err)
				}
			}

			for _, binPath := range selectedPackage.Install.Binaries {
				srcPath := filepath.Join(buildDir, binPath)
				dstPath := filepath.Join(binDir, filepath.Base(binPath))
				fmt.Printf("Copying binary %s to %s\n", binPath, dstPath)
				if err := pkg.CopyFile(srcPath, dstPath); err != nil {
					log.Printf("Error copying binary %s: %v", binPath, err)
				}
				if err := os.Chmod(dstPath, 0755); err != nil {
					log.Printf("Error making binary executable %s: %v", dstPath, err)
				}
			}

			for _, confPath := range selectedPackage.Install.Configs {
				srcPath := filepath.Join(buildDir, confPath)
				dstPath := filepath.Join(configDir, filepath.Base(confPath))

				if _, err := os.Stat(srcPath); os.IsNotExist(err) {
					log.Printf("Warning: config file %s does not exist", srcPath)
					continue
				}

				if err := pkg.CopyFile(srcPath, dstPath); err != nil {
					log.Printf("Warning: could not copy config %s: %v", confPath, err)
				} else {
					fmt.Printf("Copied config %s to %s\n", confPath, dstPath)
				}
			}

			for _, manPage := range selectedPackage.Install.Man {
				srcPath := filepath.Join(buildDir, manPage)
				dstPath := filepath.Join(manDir, filepath.Base(manPage))

				if _, err := os.Stat(srcPath); os.IsNotExist(err) {
					log.Printf("Warning: man page %s does not exist", srcPath)
					continue
				}

				if err := pkg.CopyFile(srcPath, dstPath); err != nil {
					log.Printf("Warning: could not copy man page %s: %v", manPage, err)
				} else {
					fmt.Printf("Copied man page %s to %s\n", manPage, dstPath)
				}
			}

			packageData, err = yaml.Marshal(selectedPackage)
			if err != nil {
				log.Fatalf("Error marshaling package data: %v", err)
			}
			if err := os.WriteFile(packageConfigPath, packageData, 0644); err != nil {
				log.Fatalf("Error writing package config file: %v", err)
			}

			if config.Options.CleanupBuild {
				if err := os.RemoveAll(buildDir); err != nil {
					log.Printf("Warning: could not remove build directory: %v", err)
				}
			}

			fmt.Printf("\nSuccessfully updated %s\n", selectedPackage.Name)
			if installedPackage.Installation.Method == "latest" {
				fmt.Printf("New Commit: %s\n", selectedPackage.Installation.ActualVersion)
			} else {
				fmt.Printf("New Version: %s\n", selectedPackage.Installation.ActualVersion)
			}
			fmt.Printf("Installation method: %s\n", selectedPackage.Installation.Method)
			fmt.Printf("Updated by: %s\n", selectedPackage.Installation.InstalledBy)
			fmt.Printf("Update time: %s\n", selectedPackage.Installation.InstalledAt.Format("2006-01-02 15:04:05"))
			fmt.Printf("Binaries: %s\n", binDir)
			fmt.Printf("Configs: %s\n", configDir)
			fmt.Printf("Man pages: %s\n", manDir)

			if len(selectedPackage.Install.Binaries) > 0 {
				fmt.Printf("\nTo add the binaries to your PATH, add this line to your shell's RC file:\n")
				fmt.Printf("export PATH=\"%s:$PATH\"\n", binDir)
			}

			return
		}

		fmt.Println("\nPackages with updates available:")
		fmt.Println("-------------------------------")

		updatesAvailable := []*pkg.Package{}

		for _, installedPackage := range installedPackages {
			for _, p := range packages {
				if p.Name == installedPackage.Name && installedPackage.Installation.Method != "latest" && p.Version != installedPackage.Installation.ActualVersion {
					fmt.Printf("\n%d) Name: %s\n", len(updatesAvailable)+1, p.Name)
					fmt.Printf("Current Version: %s\n", installedPackage.Installation.ActualVersion)
					fmt.Printf("Available Version: %s\n", p.Version)
					fmt.Printf("Description: %s\n", p.Description)

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

			selectedPackage := updatesAvailable[num-1]
			fmt.Println("\nUpdating package:", selectedPackage.Name)

			// Почистване на съществуващите файлове
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

			// Четене и почистване на предишната инсталация
			packageConfigPath := filepath.Join(config.Paths.Configs, selectedPackage.Name, "package.yaml")
			packageData, err := os.ReadFile(packageConfigPath)
			if err == nil {
				var previousPackage pkg.Package
				if err := yaml.Unmarshal(packageData, &previousPackage); err == nil {
					for _, binPath := range previousPackage.Install.Binaries {
						existingBinFile := filepath.Join(config.Paths.Bin, filepath.Base(binPath))
						if err := os.Remove(existingBinFile); err != nil {
							log.Printf("Warning: could not remove existing binary %s: %v", existingBinFile, err)
						}
					}

					for _, confPath := range previousPackage.Install.Configs {
						existingConfFile := filepath.Join(config.Paths.Configs, previousPackage.Name, filepath.Base(confPath))
						if err := os.Remove(existingConfFile); err != nil {
							log.Printf("Warning: could not remove existing config %s: %v", existingConfFile, err)
						}
					}

					for _, manPage := range previousPackage.Install.Man {
						existingManFile := filepath.Join(config.Paths.Man, filepath.Base(manPage))
						if err := os.Remove(existingManFile); err != nil {
							log.Printf("Warning: could not remove existing man page %s: %v", existingManFile, err)
						}
					}
				}
			}

			binDir := config.Paths.Bin
			configDir := filepath.Join(config.Paths.Configs, selectedPackage.Name)
			buildDir := filepath.Join(config.Paths.Build, selectedPackage.Name)
			manDir := config.Paths.Man

			if _, err := os.Stat(buildDir); err == nil {
				if utils.AskForConfirmation(fmt.Sprintf("Build directory %s exists. Remove it?", buildDir)) {
					if err := os.RemoveAll(buildDir); err != nil {
						log.Fatalf("Error removing build directory: %v", err)
					}
				} else {
					fmt.Println("Installation cancelled.")
					return
				}
			}

			for _, dir := range []string{binDir, configDir, buildDir, manDir} {
				if err := utils.EnsureDirectoryExists(dir); err != nil {
					log.Fatalf("Error creating directory %s: %v", dir, err)
				}
			}

			if err := os.Chdir(buildDir); err != nil {
				log.Fatalf("Error changing to directory %s: %v", buildDir, err)
			}

			// Обновяване на installation информацията
			selectedPackage.Installation = pkg.Installation{
				Method:        "specific",
				ActualVersion: selectedPackage.Version,
				InstalledAt:   time.Now().UTC(),
				InstalledBy:   utils.GetCurrentUser(),
			}

			if selectedPackage.Installation.Method == "latest" {
				cmd := exec.Command("git", "rev-parse", "HEAD")
				cmd.Dir = buildDir
				out, err := cmd.Output()
				if err != nil {
					log.Fatalf("Error getting current commit: %v", err)
				}
				commit := strings.TrimSpace(string(out))
				selectedPackage.Installation.Method = "latest"
				selectedPackage.Installation.ActualVersion = commit
			}

			for k, v := range selectedPackage.Install.Environment {
				os.Setenv(k, v)
			}

			for _, step := range selectedPackage.Install.Steps {
				if selectedPackage.Installation.Method == "latest" && strings.Contains(step, "git clone") {
					step = strings.Replace(step, "--branch v", "", 1)
					step = strings.Replace(step, "0.8.1", "", 1)
					step = strings.Replace(step, "--single-branch", "", 1)
				}
				fmt.Printf("Executing: %s\n", step)
				cmdParts := strings.Fields(step)
				cmd := exec.Command(cmdParts[0], cmdParts[1:]...)
				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				if err := cmd.Run(); err != nil {
					log.Fatalf("Error executing step '%s': %v", step, err)
				}
			}

			for _, binPath := range selectedPackage.Install.Binaries {
				srcPath := filepath.Join(buildDir, binPath)
				dstPath := filepath.Join(binDir, filepath.Base(binPath))
				fmt.Printf("Copying binary %s to %s\n", binPath, dstPath)
				if err := pkg.CopyFile(srcPath, dstPath); err != nil {
					log.Printf("Error copying binary %s: %v", binPath, err)
				}
				if err := os.Chmod(dstPath, 0755); err != nil {
					log.Printf("Error making binary executable %s: %v", dstPath, err)
				}
			}

			for _, confPath := range selectedPackage.Install.Configs {
				srcPath := filepath.Join(buildDir, confPath)
				dstPath := filepath.Join(configDir, filepath.Base(confPath))

				if _, err := os.Stat(srcPath); os.IsNotExist(err) {
					log.Printf("Warning: config file %s does not exist", srcPath)
					continue
				}

				if err := pkg.CopyFile(srcPath, dstPath); err != nil {
					log.Printf("Warning: could not copy config %s: %v", confPath, err)
				} else {
					fmt.Printf("Copied config %s to %s\n", confPath, dstPath)
				}
			}

			for _, manPage := range selectedPackage.Install.Man {
				srcPath := filepath.Join(buildDir, manPage)
				dstPath := filepath.Join(manDir, filepath.Base(manPage))

				if _, err := os.Stat(srcPath); os.IsNotExist(err) {
					log.Printf("Warning: man page %s does not exist", srcPath)
					continue
				}

				if err := pkg.CopyFile(srcPath, dstPath); err != nil {
					log.Printf("Warning: could not copy man page %s: %v", manPage, err)
				} else {
					fmt.Printf("Copied man page %s to %s\n", manPage, dstPath)
				}
			}

			packageData, err = yaml.Marshal(selectedPackage)
			if err != nil {
				log.Fatalf("Error marshaling package data: %v", err)
			}
			if err := os.WriteFile(packageConfigPath, packageData, 0644); err != nil {
				log.Fatalf("Error writing package config file: %v", err)
			}

			if config.Options.CleanupBuild {
				if err := os.RemoveAll(buildDir); err != nil {
					log.Printf("Warning: could not remove build directory: %v", err)
				}
			}

			fmt.Printf("\nSuccessfully updated %s to version %s\n", selectedPackage.Name, selectedPackage.Version)
			fmt.Printf("Installation method: %s\n", selectedPackage.Installation.Method)
			fmt.Printf("Updated by: %s\n", selectedPackage.Installation.InstalledBy)
			fmt.Printf("Update time: %s\n", selectedPackage.Installation.InstalledAt.Format("2006-01-02 15:04:05"))
			fmt.Printf("Binaries: %s\n", binDir)
			fmt.Printf("Configs: %s\n", configDir)
			fmt.Printf("Man pages: %s\n", manDir)

			if len(selectedPackage.Install.Binaries) > 0 {
				fmt.Printf("\nTo add the binaries to your PATH, add this line to your shell's RC file:\n")
				fmt.Printf("export PATH=\"%s:$PATH\"\n", binDir)
			}
		}
	},
}

func init() {
	updateCmd.Flags().BoolVarP(&forceRefreshInUpdate, "force-refresh", "f", false, "Force refresh of the registry cache")
	updateCmd.Flags().BoolVarP(&useLatestInUpdate, "latest", "l", false, "Update to the latest commit")
	rootCmd.AddCommand(updateCmd)
}
