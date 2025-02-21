package cmd

import (
	"clipack/internal"
	"fmt"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all installed applications",
	Run: func(cmd *cobra.Command, args []string) {
		db := internal.InitDB()
		defer db.Close()
		fmt.Println("Listing all applications...")
		internal.ListApplications(db)
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
