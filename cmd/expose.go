package cmd

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/lvim-tech/clipack/cnfg"
	"github.com/lvim-tech/clipack/pkg"
	"github.com/spf13/cobra"
)

// exposeCmd links chosen binaries into the user's own bin directory.
//
// The bin directory clipack installs into is not on PATH by design, so a
// package being installed does not put its commands into the environment.
// Exposing is how the few that should be there get there — and it is per
// binary, not per package, because a package that installs six programs
// usually earns the exception for one of them.
var exposeCmd = &cobra.Command{
	Use:   "expose [package] [binary...]",
	Short: "Link a package's binaries into your own bin directory",
	Long: `Link binaries of an installed package into the expose directory
(paths.expose, ~/.local/bin by default), which is on PATH.

Without a binary name every binary the package installs is linked. The choice is
recorded in the package's manifest, so a later update relinks it.

Called with no arguments it prints what is exposed right now.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := loadConfig()
		if err != nil {
			return err
		}

		installedMap, err := pkg.InstalledMap(config)
		if err != nil {
			return err
		}

		if len(args) == 0 {
			return printExposeInventory(config, installedMap)
		}

		name := args[0]
		installed, ok := installedMap[name]
		if !ok {
			return fmt.Errorf("package %q is not installed", name)
		}

		if err := newInstaller(config).Expose(installed, args[1:]); err != nil {
			return err
		}

		printExposeStatus(config, installed)
		return nil
	},
}

// unexposeCmd removes links made for a package. A binary the registry entry
// declares stays unexposed across rebuilds, which is what the manifest's
// unexposed list is for.
var unexposeCmd = &cobra.Command{
	Use:   "unexpose <package> [binary...]",
	Short: "Remove a package's links from your own bin directory",
	Long: `Remove links made in the expose directory for an installed package.

Without a binary name every link the package has is removed. Only links that
point at this package's binaries are touched: a file of the same name that
belongs to something else is left where it is.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := loadConfig()
		if err != nil {
			return err
		}

		installedMap, err := pkg.InstalledMap(config)
		if err != nil {
			return err
		}

		name := args[0]
		installed, ok := installedMap[name]
		if !ok {
			return fmt.Errorf("package %q is not installed", name)
		}

		if err := newInstaller(config).Unexpose(installed, args[1:]); err != nil {
			return err
		}

		printExposeStatus(config, installed)
		return nil
	},
}

// printExposeStatus prints what one package's links look like now.
func printExposeStatus(config *cnfg.Config, p *pkg.Package) {
	statuses := pkg.ExposeStatuses(config, p)
	if len(statuses) == 0 {
		fmt.Printf("%s exposes nothing.\n", p.Name)
		return
	}

	fmt.Printf("\n%s\n", p.Name)
	for _, st := range statuses {
		// The arrow is only drawn for a link that is actually there: printing
		// it for one that was refused would claim the opposite of what the
		// line below then explains.
		if st.State == pkg.ExposeLinked {
			fmt.Printf("  %s → %s\n", st.Link, st.Target)
		} else {
			fmt.Printf("  %s\n", st.Link)
		}
		if problem := st.Problem(); problem != "" {
			fmt.Printf("      %s\n", problem)
		}
	}
}

// printExposeInventory lists every exposed binary of every installed package,
// which is the answer to "what did I actually put on PATH".
func printExposeInventory(config *cnfg.Config, installed map[string]*pkg.Package) error {
	names := make([]string, 0, len(installed))
	for name := range installed {
		names = append(names, name)
	}
	sort.Strings(names)

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "PACKAGE\tBINARY\tLINK\tSTATUS")

	shown := 0
	for _, name := range names {
		for _, st := range pkg.ExposeStatuses(config, installed[name]) {
			status := "ok"
			if problem := st.Problem(); problem != "" {
				status = problem
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", name, st.Name, st.Link, status)
			shown++
		}
	}

	if err := w.Flush(); err != nil {
		return err
	}
	if shown == 0 {
		fmt.Printf("Nothing is exposed. Run 'clipack expose <package>' to link one.\n")
		return nil
	}

	fmt.Printf("\n%d exposed in %s\n", shown, config.Paths.Expose)
	if cnfg.DirOnPath(config.Paths.Expose) < 0 {
		fmt.Printf("warning: %s is not on PATH\n", config.Paths.Expose)
	}
	return nil
}

func init() {
	rootCmd.AddCommand(exposeCmd)
	rootCmd.AddCommand(unexposeCmd)
}
