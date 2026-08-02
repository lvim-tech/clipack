package cmd

import (
	"fmt"
	"strings"

	"github.com/lvim-tech/clipack/pkg"
	"github.com/lvim-tech/clipack/tui"
	"github.com/spf13/cobra"
)

var (
	installForceRefresh bool
	installMethod       string
	installYes          bool
)

// installCmd installs one or more packages by name. Without arguments it hands
// over to the TUI rather than reimplementing a numbered-menu prompt.
var installCmd = &cobra.Command{
	Use:   "install [package...]",
	Short: "Install packages from the registry",
	Long: `Install one or more packages by name.

Called without arguments it opens the interactive interface, where packages can
be browsed, filtered and installed.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return tui.Run()
		}

		config, err := loadConfig()
		if err != nil {
			return err
		}

		packages, err := loadPackages(config, installForceRefresh)
		if err != nil {
			return err
		}

		installed, err := pkg.InstalledMap(config)
		if err != nil {
			return err
		}

		installer := newInstaller(config)
		method := installer.ResolveMethod(installMethod)

		for _, name := range args {
			selected := pkg.FindByName(packages, name)
			if selected == nil {
				return fmt.Errorf("package %q not found in registry", name)
			}

			// Installing over an existing install would leave the previous
			// version's binaries, resource trees and man pages behind: only
			// update knows what they were, because it reads the manifest first.
			//
			// Unless there is nothing to leave behind. A manifest outlives what
			// it describes, and a package whose binary is gone is not installed
			// in any sense the user cares about — refusing to install it, and
			// pointing at update instead, is the least useful thing to say.
			if prev := installed[name]; prev != nil {
				if missing := prev.MissingArtifacts(config); len(missing) == 0 {
					return fmt.Errorf("%s is already installed: use 'clipack update %s', or 'clipack remove %s' first", name, name, name)
				} else {
					fmt.Printf("%s is recorded as installed but %s is missing; reinstalling\n",
						name, strings.Join(missing, ", "))
				}
			}

			if !installYes && !confirmInstall(selected, method, config.Paths.Build) {
				fmt.Println("Skipped", name)
				continue
			}

			// Copy so the cached registry entry is not mutated by the installer.
			candidate := *selected
			if err := installer.Install(&candidate, method); err != nil {
				return fmt.Errorf("installing %s: %w", name, err)
			}
		}

		return nil
	},
}

// confirmInstall prints a summary and asks for confirmation.
func confirmInstall(p *pkg.Package, method, buildDir string) bool {
	fmt.Printf("\n%s\n", p.Name)
	fmt.Printf("  description : %s\n", p.Description)
	fmt.Printf("  %-11s : %s\n", method, p.Ref(method))
	fmt.Printf("  maintainer  : %s\n", p.Maintainer)
	if p.License != "" {
		fmt.Printf("  license     : %s\n", p.License)
	}
	if p.Homepage != "" {
		fmt.Printf("  homepage    : %s\n", p.Homepage)
	}
	fmt.Printf("  build dir   : %s\n\n", buildDir)

	return askYes("Proceed with installation?")
}

func init() {
	installCmd.Flags().BoolVarP(&installForceRefresh, "force-refresh", "f", false, "Force refresh of the registry cache")
	installCmd.Flags().StringVarP(&installMethod, "install-method", "m", "", "Installation method: version or commit")
	installCmd.Flags().BoolVarP(&installYes, "yes", "y", false, "Do not ask for confirmation")
	rootCmd.AddCommand(installCmd)
}
