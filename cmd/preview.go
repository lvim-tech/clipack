package cmd

import (
	"fmt"

	"github.com/lvim-tech/clipack/pkg"
	"github.com/lvim-tech/clipack/tui"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var previewForceRefresh bool

// previewCmd dumps a package's registry record as YAML, plus what is currently
// installed. Unlike the previous implementation it bootstraps the config first,
// so it works on a fresh installation instead of failing to read config.yaml.
var previewCmd = &cobra.Command{
	Use:   "preview [package]",
	Short: "Show the full registry record for a package",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return tui.Run()
		}

		config, err := loadConfig()
		if err != nil {
			return err
		}

		if previewForceRefresh {
			if err := pkg.ClearCache(config); err != nil {
				return err
			}
		}

		name := args[0]
		packageInfo, err := pkg.LoadPackageFromRegistry(name, config)
		if err != nil {
			return err
		}

		yamlData, err := yaml.Marshal(packageInfo)
		if err != nil {
			return fmt.Errorf("encoding package: %w", err)
		}

		fmt.Println(string(yamlData))

		installedMap, err := pkg.InstalledMap(config)
		if err != nil {
			return err
		}

		installed, ok := installedMap[packageInfo.Name]
		if !ok {
			fmt.Println("status: not installed")
			return nil
		}

		method := installed.InstallMethod
		if method == "" {
			method = pkg.MethodVersion
		}

		fmt.Println("status: installed")
		fmt.Printf("install_method: %s\n", method)
		fmt.Printf("installed_ref: %s\n", installed.Ref(method))
		if pkg.HasUpdate(packageInfo, installed) {
			fmt.Printf("available_ref: %s\n", packageInfo.Ref(method))
		}

		return nil
	},
}

func init() {
	previewCmd.Flags().BoolVarP(&previewForceRefresh, "force-refresh", "f", false, "Force refresh of the registry cache")
	rootCmd.AddCommand(previewCmd)
}
