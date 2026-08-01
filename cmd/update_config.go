package cmd

import (
	"fmt"

	"github.com/lvim-tech/clipack/cnfg"
	"github.com/spf13/cobra"
)

// updateConfigCmd repoints the installation directory. The registry section and
// the options block are carried over from the existing file, so a configured
// token or a non-default install method survives the rewrite.
var updateConfigCmd = &cobra.Command{
	Use:   "update-config",
	Short: "Update the configuration file",
	Long: `Update the configuration file, keeping the registry settings and
options from the current file and only repointing the installation paths.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := cnfg.UpdateConfig(); err != nil {
			return fmt.Errorf("updating config: %w", err)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(updateConfigCmd)
}
