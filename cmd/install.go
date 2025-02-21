package cmd

import (
	"clipack/internal"
	"fmt"

	"github.com/spf13/cobra"
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install a new application",
	Run: func(cmd *cobra.Command, args []string) {
		db := internal.InitDB()
		defer db.Close()
		// Тук ще добавим кода за инсталиране на ново приложение
		fmt.Println("Installing a new application...")
	},
}

func init() {
	rootCmd.AddCommand(installCmd)
}
