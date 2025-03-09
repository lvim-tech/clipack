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
	forceRefreshInInstall bool
	useLatestInInstall    bool
)

var installCmd = &cobra.Command{
	Use:   "install [package-name]",
	Short: "Install a package from registry",
	Run: func(cmd *cobra.Command, args []string) {
		if err := cnfg.CreateDefaultConfig(); err != nil {
			log.Fatalf("Error creating config file: %v", err)
		}

		config, err := cnfg.LoadConfig()
		if err != nil {
			log.Fatalf("Error loading config: %v", err)
		}

		var packages []*pkg.Package
		if forceRefreshInInstall {
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

		if len(args) > 0 {
			packageName := args[0]
			var selectedPackage *pkg.Package

			for _, p := range packages {
				if p.Name == packageName {
					selectedPackage = p
					break
				}
			}

			if selectedPackage == nil {
				log.Fatalf("Package %s not found in registry", packageName)
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

			if !utils.AskForConfirmation("Proceed with installation?") {
				fmt.Println("Installation cancelled.")
				return
			}

			fmt.Println("\nInstalling package:", selectedPackage.Name)
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
				if useLatestInInstall && strings.Contains(step, "git clone") {
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

			// Добавяне на installation информация
			selectedPackage.Installation = pkg.Installation{
				Method:        "specific",
				ActualVersion: selectedPackage.Version,
				InstalledAt:   time.Now().UTC(),
				InstalledBy:   utils.GetCurrentUser(),
			}

			if useLatestInInstall {
				// Извличане на текущия комит
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

			for _, additionalConfig := range selectedPackage.Install.AdditionalConfig {
				dstPath := filepath.Join(configDir, additionalConfig.Filename)

				dstDir := filepath.Dir(dstPath)
				if err := os.MkdirAll(dstDir, 0755); err != nil {
					log.Printf("Warning: could not create directory structure for %s: %v", dstPath, err)
					continue
				}

				if err := os.WriteFile(dstPath, []byte(additionalConfig.Content), 0644); err != nil {
					log.Printf("Warning: could not write additional config %s: %v", additionalConfig.Filename, err)
				} else {
					fmt.Printf("Created additional config %s\n", dstPath)
				}
			}

			// Запазване на package.yaml с installation информацията
			packageData, err := yaml.Marshal(selectedPackage)
			if err != nil {
				log.Fatalf("Error marshaling package data: %v", err)
			}
			packageConfigPath := filepath.Join(configDir, "package.yaml")
			if err := os.WriteFile(packageConfigPath, packageData, 0644); err != nil {
				log.Fatalf("Error writing package config file: %v", err)
			}

			if config.Options.CleanupBuild {
				if err := os.RemoveAll(buildDir); err != nil {
					log.Printf("Warning: could not remove build directory: %v", err)
				}
			}

			fmt.Printf("\nSuccessfully installed %s version %s\n", selectedPackage.Name, selectedPackage.Version)
			fmt.Printf("Installation method: %s\n", selectedPackage.Installation.Method)
			fmt.Printf("Installed by: %s\n", selectedPackage.Installation.InstalledBy)
			fmt.Printf("Installation time: %s\n", selectedPackage.Installation.InstalledAt.Format("2006-01-02 15:04:05"))
			fmt.Printf("Binaries: %s\n", binDir)
			fmt.Printf("Configs: %s\n", configDir)
			fmt.Printf("Man pages: %s\n", manDir)

			if len(selectedPackage.Install.Binaries) > 0 {
				fmt.Printf("\nTo add the binaries to your PATH, add this line to your shell's RC file:\n")
				fmt.Printf("export PATH=\"%s:$PATH\"\n", binDir)
			}

			return
		}

		fmt.Println("\nAvailable packages:")
		fmt.Println("-------------------")

		availablePackages := []*pkg.Package{}

		for _, p := range packages {
			tags := strings.Join(p.Tags, ", ")
			if tags == "" {
				tags = "-"
			}

			fmt.Printf("\n%d) Name: %s\n", len(availablePackages)+1, p.Name)
			fmt.Printf("Version: %s\n", p.Version)
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

			availablePackages = append(availablePackages, p)
		}

		reader := bufio.NewReader(os.Stdin)
		for {
			fmt.Print("\nEnter package number to install (or 'q' to quit): ")
			input, err := reader.ReadString('\n')
			if err != nil {
				log.Fatalf("Error reading input: %v", err)
			}

			input = strings.TrimSpace(input)
			if input == "q" {
				fmt.Println("Installation cancelled.")
				return
			}

			num, err := strconv.Atoi(input)
			if err != nil || num < 1 || num > len(availablePackages) {
				fmt.Printf("Please enter a number between 1 and %d\n", len(availablePackages))
				continue
			}

			selectedPackage := availablePackages[num-1]
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

			if !utils.AskForConfirmation("Proceed with installation?") {
				fmt.Println("Installation cancelled.")
				continue
			}

			fmt.Println("\nInstalling package:", selectedPackage.Name)

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
					continue
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

			// Добавяне на installation информация
			selectedPackage.Installation = pkg.Installation{
				Method:        "specific",
				ActualVersion: selectedPackage.Version,
				InstalledAt:   time.Now().UTC(),
				InstalledBy:   utils.GetCurrentUser(),
			}

			if useLatestInInstall {
				// Извличане на текущия комит
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

			// Изпълнение на инсталационните стъпки
			for _, step := range selectedPackage.Install.Steps {
				if useLatestInInstall && strings.Contains(step, "git clone") {
					step = strings.Replace(step, "--branch", "", 1)
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

			// Копиране на бинарните файлове
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

			// Запазване на package.yaml с installation информацията
			packageData, err := yaml.Marshal(selectedPackage)
			if err != nil {
				log.Fatalf("Error marshaling package data: %v", err)
			}
			packageConfigPath := filepath.Join(configDir, "package.yaml")
			if err := os.WriteFile(packageConfigPath, packageData, 0644); err != nil {
				log.Fatalf("Error writing package config file: %v", err)
			}

			if config.Options.CleanupBuild {
				if err := os.RemoveAll(buildDir); err != nil {
					log.Printf("Warning: could not remove build directory: %v", err)
				}
			}

			fmt.Printf("\nSuccessfully installed %s version %s\n", selectedPackage.Name, selectedPackage.Version)
			fmt.Printf("Installation method: %s\n", selectedPackage.Installation.Method)
			fmt.Printf("Installed by: %s\n", selectedPackage.Installation.InstalledBy)
			fmt.Printf("Installation time: %s\n", selectedPackage.Installation.InstalledAt.Format("2006-01-02 15:04:05"))
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
	installCmd.Flags().BoolVarP(&forceRefreshInInstall, "force-refresh", "f", false, "Force refresh of the registry cache")
	installCmd.Flags().BoolVarP(&useLatestInInstall, "latest", "l", false, "Install the latest version")
	rootCmd.AddCommand(installCmd)
}
