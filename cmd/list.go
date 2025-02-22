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

var forceRefreshInList bool

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List available packages",
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
		if forceRefreshInList {
			fmt.Println("Forcing refresh of the registry cache...")

			// Изтриваме кеш файловете
			cachePath := pkg.GetCacheFilePath()
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

		fmt.Println("\nAvailable packages:")
		fmt.Println("------------------")
		for _, p := range packages {
			tags := strings.Join(p.Tags, ", ")
			if tags == "" {
				tags = "-"
			}

			fmt.Printf("\nName: %s\n", p.Name)
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
		}
	},
}

func init() {
	listCmd.Flags().BoolVarP(&forceRefreshInList, "force-refresh", "f", false, "Force refresh of the registry cache")
	rootCmd.AddCommand(listCmd)
}
