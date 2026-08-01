package cmd

import (
	"fmt"

	"github.com/lvim-tech/clipack/pkg"
	"github.com/spf13/cobra"
)

var (
	updateForceRefresh bool
	updateAll          bool
	updateYes          bool
)

// updateCmd rebuilds installed packages whose registry entry has moved on.
// Each package is updated using the method it was installed with, so a package
// pinned to a commit is not silently switched to a version tag.
var updateCmd = &cobra.Command{
	Use:   "update [package...]",
	Short: "Update installed packages",
	Long: `Update installed packages to the version or commit currently listed in
the registry.

Without arguments the available updates are listed. Use --all to apply them, or
name the packages to update explicitly.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := loadConfig()
		if err != nil {
			return err
		}

		packages, err := loadPackages(config, updateForceRefresh)
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

		// Collect the outdated packages once; both the listing and the update
		// path work from the same set.
		type candidate struct {
			registry  *pkg.Package
			installed *pkg.Package
		}
		var outdated []candidate
		for _, p := range packages {
			installed, ok := installedMap[p.Name]
			if !ok || !pkg.HasUpdate(p, installed) {
				continue
			}
			outdated = append(outdated, candidate{registry: p, installed: installed})
		}

		// Named packages take precedence over the outdated set.
		if len(args) > 0 {
			installer := newInstaller(config)
			for _, name := range args {
				registry := pkg.FindByName(packages, name)
				installed := installedMap[name]
				if registry == nil {
					return fmt.Errorf("package %q not found in registry", name)
				}
				if installed == nil {
					return fmt.Errorf("package %q is not installed", name)
				}
				if !pkg.HasUpdate(registry, installed) {
					fmt.Printf("%s is already up to date\n", name)
					continue
				}

				method := installed.InstallMethod
				if method == "" {
					method = pkg.MethodVersion
				}

				fmt.Printf("\n%s: %s → %s\n", name, installed.Ref(method), registry.Ref(method))
				if !updateYes && !askYes("Proceed with update?") {
					continue
				}

				candidatePkg := *registry
				if err := installer.Update(&candidatePkg, method); err != nil {
					return fmt.Errorf("updating %s: %w", name, err)
				}
			}
			return nil
		}

		if len(outdated) == 0 {
			fmt.Println("All packages are up to date.")
			return nil
		}

		fmt.Printf("\n%d update(s) available:\n\n", len(outdated))
		for _, c := range outdated {
			method := c.installed.InstallMethod
			if method == "" {
				method = pkg.MethodVersion
			}
			// The previous implementation printed installedPackages[0].Version
			// here, i.e. the version of an unrelated package.
			fmt.Printf("  %-16s %s → %s\n", c.registry.Name, c.installed.Ref(method), c.registry.Ref(method))
		}
		fmt.Println()

		if !updateAll {
			fmt.Println("Run 'clipack update --all' to apply, or 'clipack update <name>' for one package.")
			return nil
		}

		installer := newInstaller(config)
		for _, c := range outdated {
			method := c.installed.InstallMethod
			if method == "" {
				method = pkg.MethodVersion
			}
			if !updateYes && !askYes(fmt.Sprintf("Update %s?", c.registry.Name)) {
				continue
			}
			candidatePkg := *c.registry
			if err := installer.Update(&candidatePkg, method); err != nil {
				return fmt.Errorf("updating %s: %w", c.registry.Name, err)
			}
		}

		return nil
	},
}

func init() {
	updateCmd.Flags().BoolVarP(&updateForceRefresh, "force-refresh", "f", false, "Force refresh of the registry cache")
	updateCmd.Flags().BoolVarP(&updateAll, "all", "a", false, "Update every outdated package")
	updateCmd.Flags().BoolVarP(&updateYes, "yes", "y", false, "Do not ask for confirmation")
	rootCmd.AddCommand(updateCmd)
}
