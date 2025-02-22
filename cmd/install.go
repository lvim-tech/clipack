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

var forceRefresh bool

var installCmd = &cobra.Command{
	Use:   "install [package-name]",
	Short: "Install a package from registry",
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

		var packageName string
		if len(args) == 0 {
			var packages []*pkg.Package

			if forceRefresh {
				fmt.Println("Forcing refresh of the registry cache...")

				// Изтриваме кеш файловете
				cachePath := pkg.GetCacheFilePath(config)
				os.Remove(cachePath)
				timestampPath := filepath.Join(config.Paths.Registry, "cache_timestamp.gob")
				os.Remove(timestampPath)

				packages, err = pkg.LoadAllPackagesFromRegistry(config)
				if err != nil {
					log.Fatalf("Error loading packages: %v", err)
				}
				fmt.Println(packages)

				// Запазваме в кеша
				if err := pkg.SaveToCache(packages, config); err != nil {
					log.Printf("Warning: could not cache packages: %v", err)
				} else {
					fmt.Println("Packages saved to cache successfully.")
				}
			} else {
				// Първо се опитваме да заредим от кеша
				cachePath := pkg.GetCacheFilePath(config)
				if _, err := os.Stat(cachePath); os.IsNotExist(err) {
					fmt.Println("Cache not found. Fetching packages from registry...")
					packages, err = pkg.LoadAllPackagesFromRegistry(config)
					if err != nil {
						log.Fatalf("Error loading packages: %v", err)
					}

					// Запазваме в кеша
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

			// Показваме интерактивен избор
			fmt.Println("\nAvailable packages:")
			fmt.Println("------------------")
			for i, p := range packages {
				tags := strings.Join(p.Tags, ", ")
				if tags == "" {
					tags = "-"
				}

				fmt.Printf("%d) %s (%s)\n", i+1, p.Name, p.Version)
				fmt.Printf("   Description: %s\n", p.Description)
				fmt.Printf("   Tags: %s\n", tags)
				fmt.Printf("   Updated: %s\n", p.UpdatedAt.Format("2006-01-02"))
				fmt.Println()
			}

			reader := bufio.NewReader(os.Stdin)
			for {
				fmt.Print("Enter package number to install (or 'q' to quit): ")
				input, err := reader.ReadString('\n')
				if err != nil {
					log.Fatalf("Error reading input: %v", err)
				}

				input = strings.TrimSpace(input)
				if input == "q" {
					fmt.Println("Installation cancelled.")
					os.Exit(0)
				}

				num, err := strconv.Atoi(input)
				if err != nil || num < 1 || num > len(packages) {
					fmt.Printf("Please enter a number between 1 and %d\n", len(packages))
					continue
				}

				p := packages[num-1]
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

				if pkg.AskForConfirmation("Proceed with installation?") {
					packageName = p.Name
					break
				}
				fmt.Println("Installation cancelled. Select another package or 'q' to quit.")
			}
		} else {
			packageName = args[0]
		}

		// Зареждаме пакета от кеша първо
		var selectedPackage *pkg.Package
		if cachedPackages, err := pkg.LoadFromCache(config); err == nil {
			for _, p := range cachedPackages {
				if p.Name == packageName {
					selectedPackage = p
					break
				}
			}
		}

		// Ако пакета не е в кеша, зареждаме го от регистъра
		if selectedPackage == nil {
			var err error
			selectedPackage, err = pkg.LoadPackageFromRegistry(packageName, config)
			if err != nil {
				log.Fatalf("Error: %v", err)
			}
		}

		// Подготвяме директориите
		binDir := config.Paths.Bin
		configDir := filepath.Join(config.Paths.Configs, selectedPackage.Name)
		buildDir := filepath.Join(config.Paths.Build, selectedPackage.Name)
		manDir := config.Paths.Man

		// Проверяваме дали buildDir съществува
		if _, err := os.Stat(buildDir); err == nil {
			if pkg.AskForConfirmation(fmt.Sprintf("Build directory %s exists. Remove it?", buildDir)) {
				if err := os.RemoveAll(buildDir); err != nil {
					log.Fatalf("Error removing build directory: %v", err)
				}
			} else {
				fmt.Println("Installation cancelled.")
				return
			}
		}

		// Създаваме необходимите директории
		for _, dir := range []string{binDir, configDir, buildDir, manDir} {
			if err := pkg.EnsureDirectoryExists(dir); err != nil {
				log.Fatalf("Error creating directory %s: %v", dir, err)
			}
		}

		// Превключваме към build директорията
		if err := os.Chdir(buildDir); err != nil {
			log.Fatalf("Error changing to directory %s: %v", buildDir, err)
		}

		// Задаваме environment променливите
		for k, v := range selectedPackage.Install.Environment {
			os.Setenv(k, v)
		}

		// Изпълняваме инсталационните стъпки
		for _, step := range selectedPackage.Install.Steps {
			// Проверяваме дали стъпката включва клониране на git репозиторий и добавяме версията с префикс "v", ако е необходимо
			if strings.Contains(step, "git clone") && !strings.Contains(step, " --branch v") {
				step = strings.Replace(step, " --branch ", " --branch v", 1)
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

		// Премахваме съществуващите файлове и копираме новите бинарни файлове
		for _, binPath := range selectedPackage.Install.Binaries {
			srcPath := filepath.Join(buildDir, binPath)
			dstPath := filepath.Join(binDir, filepath.Base(binPath))
			// Проверяваме дали файлът съществува и го трием, ако е необходимо
			if _, err := os.Lstat(dstPath); err == nil {
				if err := os.Remove(dstPath); err != nil {
					log.Printf("Error removing existing binary %s: %v", dstPath, err)
				}
			}
			fmt.Printf("Copying binary %s to %s\n", binPath, dstPath)
			if err := pkg.CopyFile(srcPath, dstPath); err != nil {
				log.Printf("Error copying binary %s: %v", binPath, err)
			}
			// Правим файла изпълним
			if err := os.Chmod(dstPath, 0755); err != nil {
				log.Printf("Error making binary executable %s: %v", dstPath, err)
			}
		}

		// Проверяваме и копираме конфигурационните файлове
		for _, confPath := range selectedPackage.Install.Configs {
			srcPath := filepath.Join(buildDir, confPath)
			dstPath := filepath.Join(configDir, filepath.Base(confPath))

			// Проверяваме дали конфигурационният файл съществува
			if _, err := os.Stat(srcPath); os.IsNotExist(err) {
				log.Printf("Warning: config file %s does not exist", srcPath)
				continue
			}

			// Копираме конфигурационния файл
			if err := pkg.CopyFile(srcPath, dstPath); err != nil {
				log.Printf("Warning: could not copy config %s: %v", confPath, err)
			} else {
				fmt.Printf("Copied config %s to %s\n", confPath, dstPath)
			}
		}

		// Копираме man страниците, ако има такива
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

		// Копираме допълнителните конфигурации, ако има такива
		for _, additionalConfig := range selectedPackage.Install.AdditionalConfig {
			dstPath := filepath.Join(configDir, additionalConfig.Filename)

			if err := os.WriteFile(dstPath, []byte(additionalConfig.Content), 0644); err != nil {
				log.Printf("Warning: could not write additional config %s: %v", additionalConfig.Filename, err)
			} else {
				fmt.Printf("Created additional config %s\n", dstPath)
			}
		}

		// Изпълняваме post-install скриптове
		for _, script := range selectedPackage.PostInstall.Scripts {
			scriptPath := filepath.Join(buildDir, script.Filename)
			if err := os.WriteFile(scriptPath, []byte(script.Content), 0755); err != nil {
				log.Printf("Warning: could not write post-install script %s: %v", script.Filename, err)
			} else {
				fmt.Printf("Created post-install script %s\n", scriptPath)

				// Преместваме скрипта в bin директорията
				dstScriptPath := filepath.Join(binDir, filepath.Base(script.Filename))
				if err := os.Rename(scriptPath, dstScriptPath); err != nil {
					log.Printf("Error moving script %s: %v", scriptPath, err)
				}
				// Правим файла изпълним
				if err := os.Chmod(dstScriptPath, 0755); err != nil {
					log.Printf("Error making script executable %s: %v", dstScriptPath, err)
				}
			}
		}

		// Почистваме
		if config.Options.CleanupBuild {
			if err := os.RemoveAll(buildDir); err != nil {
				log.Printf("Warning: could not remove build directory: %v", err)
			}
		}

		fmt.Printf("\nSuccessfully installed %s %s\n", selectedPackage.Name, selectedPackage.Version)
		fmt.Printf("Binaries: %s\n", binDir)
		fmt.Printf("Configs: %s\n", configDir)
		fmt.Printf("Man pages: %s\n", manDir)

		// Извеждаме инструкции за обновяване на PATH
		if len(selectedPackage.Install.Binaries) > 0 {
			fmt.Printf("\nTo add the binaries to your PATH, add this line to your shell's RC file:\n")
			fmt.Printf("export PATH=\"%s:$PATH\"\n", binDir)
		}
	},
}

func init() {
	installCmd.Flags().BoolVarP(&forceRefresh, "force-refresh", "f", false, "Force refresh of the registry cache")
	rootCmd.AddCommand(installCmd)
}
