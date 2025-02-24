package cmd

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/lvim-tech/clipack/pkg"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var forceRefreshInPreview bool

var previewCmd = &cobra.Command{
	Use:   "preview [package-name]",
	Short: "Preview the registry packages",
	Run: func(cmd *cobra.Command, args []string) {
		// Зареждаме конфигурацията
		config, err := pkg.LoadConfig()
		if err != nil {
			log.Fatalf("Error loading config: %v", err)
		}

		if forceRefreshInPreview {
			fmt.Println("Forcing refresh of the registry cache...")

			// Изтриваме кеш файловете
			cachePath := pkg.GetCacheFilePath(config)
			os.Remove(cachePath)
			timestampPath := pkg.GetCacheTimestampFilePath(config)
			os.Remove(timestampPath)
		}

		if len(args) > 0 {
			packageName := args[0]
			packageInfo, err := pkg.LoadPackageFromRegistry(packageName, config)
			if err != nil {
				log.Fatalf("Error loading package: %v", err)
			}

			printFullPackageInfo(packageInfo)
			return
		}

		packages, err := pkg.LoadAllPackagesFromRegistry(config)
		if err != nil {
			log.Fatalf("Error loading packages from registry: %v", err)
		}

		fmt.Println("Registry Preview:")
		for i, pkg := range packages {
			fmt.Printf("%d) Name: %s, Version: %s, Description: %s\n", i+1, pkg.Name, pkg.Version, pkg.Description)
		}

		reader := bufio.NewReader(os.Stdin)
		for {
			fmt.Print("\nEnter package number to view details (or 'q' to quit): ")
			input, err := reader.ReadString('\n')
			if err != nil {
				log.Fatalf("Error reading input: %v", err)
			}

			input = strings.TrimSpace(input)
			if input == "q" {
				fmt.Println("Preview cancelled.")
				return
			}

			num, err := strconv.Atoi(input)
			if err != nil || num < 1 || num > len(packages) {
				fmt.Printf("Please enter a number between 1 and %d\n", len(packages))
				continue
			}

			selectedPackage := packages[num-1]
			printFullPackageInfo(selectedPackage)
			break
		}
	},
}

func printFullPackageInfo(pkg *pkg.Package) {
	fmt.Println("\nPackage Details:")
	fmt.Println("----------------")
	yamlData, err := yaml.Marshal(pkg)
	if err != nil {
		log.Fatalf("Error marshalling package info: %v", err)
	}
	fmt.Println(string(yamlData))
}

func init() {
	previewCmd.Flags().BoolVarP(&forceRefreshInPreview, "force-refresh", "f", false, "Force refresh of the registry cache")
	rootCmd.AddCommand(previewCmd)
}
