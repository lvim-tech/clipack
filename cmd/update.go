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

	"github.com/lvim-tech/clipack/cnfg"
	"github.com/lvim-tech/clipack/pkg"
	"github.com/lvim-tech/clipack/utils"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	forceRefreshInUpdate bool
	updateInstallMethod  string
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

		if updateInstallMethod == "" {
			updateInstallMethod = config.Options.InstallMethod
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

			for _, installed := range installedPackages {
				if installed.Name == packageName && installed.InstallMethod == updateInstallMethod {
					for _, p := range packages {
						if p.Name == installed.Name && ((updateInstallMethod == "version" && p.Version != installed.Version) || (updateInstallMethod == "commit" && p.Commit != installed.Commit)) {
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
			fmt.Printf("Installed Version/Commit: %s\n", installedPackages[0].Version)
			if updateInstallMethod == "version" {
				fmt.Printf("Available Version: %s\n", selectedPackage.Version)
			} else {
				fmt.Printf("Available Commit: %s\n", selectedPackage.Commit)
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
			fmt.Printf("Updated: %s\n\n", selectedPackage.UpdatedAt.Format("2006-01-02 15:04:05"))

			if !utils.AskForConfirmation("Proceed with update?") {
				fmt.Println("Update cancelled.")
				return
			}

			updatePackage(selectedPackage, config)
			return
		}

		fmt.Println("\nPackages with updates available:")
		fmt.Println("-------------------------------")

		var updatesAvailable []*pkg.Package
		for _, installed := range installedPackages {
			if installed.InstallMethod != updateInstallMethod {
				continue
			}
			for _, p := range packages {
				if p.Name == installed.Name && ((updateInstallMethod == "version" && p.Version != installed.Version) || (updateInstallMethod == "commit" && p.Commit != installed.Commit)) {
					tags := strings.Join(p.Tags, ", ")
					if tags == "" {
						tags = "-"
					}

					fmt.Printf("\n%d) Name: %s\n", len(updatesAvailable)+1, p.Name)
					fmt.Printf("Current Version/Commit: %s\n", installed.Version)
					if updateInstallMethod == "version" {
						fmt.Printf("Available Version: %s\n", p.Version)
					} else {
						fmt.Printf("Available Commit: %s\n", p.Commit)
					}
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
			fmt.Printf("Installed Version/Commit: %s\n", installedPackages[0].Version)
			if updateInstallMethod == "version" {
				fmt.Printf("Available Version: %s\n", p.Version)
			} else {
				fmt.Printf("Available Commit: %s\n", p.Commit)
			}
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

			if !utils.AskForConfirmation("Proceed with update?") {
				fmt.Println("Update cancelled.")
				continue
			}

			updatePackage(p, config)
			break
		}
	},
}

func updatePackage(selectedPackage *pkg.Package, config *cnfg.Config) {
	fmt.Println("\nUpdating package:", selectedPackage.Name)

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

	for k, v := range selectedPackage.Install.Environment {
		os.Setenv(k, v)
	}

	for _, step := range selectedPackage.Install.Steps {
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
		if _, err := os.Lstat(dstPath); err == nil {
			if err := os.Remove(dstPath); err != nil {
				log.Printf("Error removing existing binary %s: %v", dstPath, err)
			}
		}
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

		ext := filepath.Ext(manPage)
		if len(ext) < 2 {
			log.Printf("Warning: could not determine section for %s", manPage)
			continue
		}

		section := "man" + ext[1:]
		sectionDir := filepath.Join(manDir, section)

		if err := os.MkdirAll(sectionDir, 0755); err != nil {
			log.Printf("Error creating man section directory %s: %v", sectionDir, err)
			continue
		}

		dstPath := filepath.Join(sectionDir, filepath.Base(manPage))

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

	for _, additionalConfig := range selectedPackage.Install.AdditionalConfig {
		dstPath := filepath.Join(configDir, additionalConfig.Filename)

		dstDir := filepath.Dir(dstPath)
		if err := os.MkdirAll(dstDir, 0755); err != nil {
			log.Printf("Warning: could not create directory structure for %s: %v", dstPath, err)
			continue
		}

		var content []byte
		if strings.HasPrefix(additionalConfig.Content, "http://") || strings.HasPrefix(additionalConfig.Content, "https://") {
			var err error
			content, err = utils.DownloadContent(additionalConfig.Content)
			if err != nil {
				log.Printf("Warning: could not download content for %s: %v", additionalConfig.Filename, err)
				continue
			}
		} else {
			content = []byte(additionalConfig.Content)
		}

		if err := os.WriteFile(dstPath, content, 0644); err != nil {
			log.Printf("Warning: could not write additional config %s: %v", additionalConfig.Filename, err)
		} else {
			fmt.Printf("Created additional config %s\n", dstPath)
		}
	}

	packageConfigPath = filepath.Join(configDir, "package.yaml")
	packageData, err = yaml.Marshal(selectedPackage)
	if err != nil {
		log.Fatalf("Error marshaling package data: %v", err)
	}
	if err := os.WriteFile(packageConfigPath, packageData, 0644); err != nil {
		log.Fatalf("Error writing package config file: %v", err)
	}

	for _, script := range selectedPackage.PostInstall.Scripts {
		scriptPath := filepath.Join(buildDir, script.Filename)
		if err := os.WriteFile(scriptPath, []byte(script.Content), 0755); err != nil {
			log.Printf("Warning: could not write post-install script %s: %v", script.Filename, err)
		} else {
			fmt.Printf("Created post-install script %s\n", scriptPath)

			dstScriptPath := filepath.Join(binDir, filepath.Base(script.Filename))
			if err := os.Rename(scriptPath, dstScriptPath); err != nil {
				log.Printf("Error moving script %s: %v", scriptPath, err)
			}
			if err := os.Chmod(dstScriptPath, 0755); err != nil {
				log.Printf("Error making script executable %s: %v", dstScriptPath, err)
			}
		}
	}

	if config.Options.CleanupBuild {
		if err := os.RemoveAll(buildDir); err != nil {
			log.Printf("Warning: could not remove build directory: %v", err)
		}
	}

	fmt.Printf("\nSuccessfully updated %s to version %s\n", selectedPackage.Name, selectedPackage.Version)
	fmt.Printf("Binaries: %s\n", binDir)
	fmt.Printf("Configs: %s\n", configDir)
	fmt.Printf("Man pages: %s\n", manDir)

	if len(selectedPackage.Install.Binaries) > 0 {
		fmt.Printf("\nTo add the binaries to your PATH, add this line to your shell's RC file:\n")
		fmt.Printf("export PATH=\"%s:$PATH\"\n", binDir)
	}
}

func init() {
	updateCmd.Flags().BoolVarP(&forceRefreshInUpdate, "force-refresh", "f", false, "Force refresh of the registry cache")
	updateCmd.Flags().StringVarP(&updateInstallMethod, "install-method", "m", "", "Installation method: version or commit")
	rootCmd.AddCommand(updateCmd)
}
