package cmd

import (
	"fmt"

	"github.com/lvim-tech/clipack/pkg"
	"github.com/lvim-tech/clipack/tui"
	"github.com/spf13/cobra"
)

var removeYes bool

// removeCmd uninstalls packages. Removal is driven by the installed manifest
// (configs/<name>/package.yaml), which is the only accurate record of what was
// written to disk — the registry entry may have changed since installation.
var removeCmd = &cobra.Command{
	Use:     "remove [package...]",
	Aliases: []string{"uninstall", "rm"},
	Short:   "Remove installed packages",
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return tui.Run()
		}

		config, err := loadConfig()
		if err != nil {
			return err
		}

		installedMap, err := pkg.InstalledMap(config)
		if err != nil {
			return err
		}
		if len(installedMap) == 0 {
			fmt.Println("No packages are installed.")
			return nil
		}

		installer := newInstaller(config)

		for _, name := range args {
			installed, ok := installedMap[name]
			if !ok {
				return fmt.Errorf("package %q is not installed", name)
			}

			fmt.Printf("\n%s (%s)\n", installed.Name, installed.Ref(installed.InstallMethod))
			fmt.Printf("  %s\n\n", installed.Description)

			if !removeYes && !askYes("Proceed with removal?") {
				fmt.Println("Skipped", name)
				continue
			}

			if err := installer.Remove(installed); err != nil {
				return fmt.Errorf("removing %s: %w", name, err)
			}
		}

		return nil
	},
}

func init() {
	removeCmd.Flags().BoolVarP(&removeYes, "yes", "y", false, "Do not ask for confirmation")
	rootCmd.AddCommand(removeCmd)
}
