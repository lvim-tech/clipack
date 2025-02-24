package cmd

import (
	"log"

	"github.com/lvim-tech/clipack/cnfg"
	"github.com/spf13/cobra"
)

var updateConfigCmd = &cobra.Command{
	Use:   "update-config",
	Short: "Update the configuration file",
	Long:  `Update the configuration file with the latest default configuration values.`,
	Run: func(cmd *cobra.Command, args []string) {
		if err := cnfg.UpdateConfig(); err != nil {
			log.Fatalf("Error updating config: %v", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(updateConfigCmd)
}
