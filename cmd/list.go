package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/lvim-tech/clipack/pkg"
	"github.com/spf13/cobra"
)

var (
	listForceRefresh bool
	listInstalled    bool
	listUpdates      bool
)

// listCmd prints the registry as a table. The output is one line per package so
// it stays greppable; use "clipack preview <name>" for the full record.
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List available packages",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := loadConfig()
		if err != nil {
			return err
		}

		packages, err := loadPackages(config, listForceRefresh)
		if err != nil {
			return err
		}

		installedMap, err := pkg.InstalledMap(config)
		if err != nil {
			return err
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tVERSION\tSTATUS\tCATEGORY\tDESCRIPTION")

		shown := 0
		for _, p := range packages {
			installed := installedMap[p.Name]

			status := "-"
			switch {
			case installed != nil && pkg.HasUpdate(p, installed):
				status = "update"
			case installed != nil && len(installed.MissingArtifacts(config)) > 0:
				// The manifest is there but what it describes is not, which is
				// worth saying plainly rather than reporting as installed.
				status = "broken"
			case installed != nil:
				status = "installed"
			}

			if listInstalled && installed == nil {
				continue
			}
			if listUpdates && status != "update" {
				continue
			}

			category := p.Category
			if category == "" {
				category = "-"
			}

			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				p.Name, p.Version, status, category, firstLine(p.Description))
			shown++
		}

		if err := w.Flush(); err != nil {
			return err
		}

		fmt.Printf("\n%d of %d package(s)\n", shown, len(packages))
		return nil
	},
}

// firstLine trims a description to its first line for table output.
func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}

func init() {
	listCmd.Flags().BoolVarP(&listForceRefresh, "force-refresh", "f", false, "Force refresh of the registry cache")
	listCmd.Flags().BoolVarP(&listInstalled, "installed", "i", false, "Only show installed packages")
	listCmd.Flags().BoolVarP(&listUpdates, "updates", "u", false, "Only show packages with updates")
	rootCmd.AddCommand(listCmd)
}
