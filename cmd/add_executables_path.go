package cmd

import (
	"fmt"

	"github.com/lvim-tech/clipack/cnfg"
	"github.com/spf13/cobra"
)

// addExecutablesPathCmd appends the managed bin and man directories to the
// user's shell rc file. Re-running it is safe: the helper skips the write when
// the path is already referenced.
var addExecutablesPathCmd = &cobra.Command{
	Use:     "add-executables-path",
	Aliases: []string{"path"},
	Short:   "Add the bin and man paths to your shell configuration",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := loadConfig()
		if err != nil {
			return err
		}

		shellConfigFilePath, err := cnfg.GetShellConfigFilePath()
		if err != nil {
			return err
		}

		fmt.Printf("These paths will be added to %s:\n", shellConfigFilePath)
		fmt.Printf("  bin : %s\n", config.Paths.Bin)
		fmt.Printf("  man : %s\n\n", config.Paths.Man)

		if !askYes("Proceed?") {
			fmt.Println("Cancelled.")
			return nil
		}

		return cnfg.AddPathsToShellConfig(config.Paths.Bin, config.Paths.Man)
	},
}

func init() {
	rootCmd.AddCommand(addExecutablesPathCmd)
}
