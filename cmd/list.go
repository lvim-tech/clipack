package cmd

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/lvim-tech/clipack/cnfg"
	"github.com/lvim-tech/clipack/pkg"
	"github.com/spf13/cobra"
)

var forceRefreshInList bool

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List available packages",
	Run: func(cmd *cobra.Command, args []string) {
		if err := cnfg.CreateDefaultConfig(); err != nil {
			log.Fatalf("Error creating config file: %v", err)
		}

		config, err := cnfg.LoadConfig()
		if err != nil {
			log.Fatalf("Error loading config: %v", err)
		}

		var packages []*pkg.Package
		if forceRefreshInList {
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

		installedMap := make(map[string]*pkg.Package)
		for _, ip := range installedPackages {
			installedMap[ip.Name] = ip
		}

		fmt.Println("\nAvailable packages:")
		fmt.Println("------------------")
		for _, p := range packages {
			tags := strings.Join(p.Tags, ", ")
			if tags == "" {
				tags = "-"
			}

			fmt.Printf("\nName: %s\n", p.Name)
			fmt.Printf("Registry Version: %s\n", p.Version)
			fmt.Printf("Registry Commit: %s\n", p.Commit)
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
			if ip, ok := installedMap[p.Name]; ok {
				fmt.Printf("Install Method: %s\n", ip.InstallMethod)
				if ip.InstallMethod == "commit" {
					fmt.Printf("Installed Commit: %s\n", ip.Commit)
				} else {
					fmt.Printf("Installed Version: %s\n", ip.Version)
				}
			} else {
				fmt.Printf("Install Method: Not installed\n")
			}
		}
	},
}

func init() {
	listCmd.Flags().BoolVarP(&forceRefreshInList, "force-refresh", "f", false, "Force refresh of the registry cache")
	rootCmd.AddCommand(listCmd)
}
