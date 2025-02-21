package cmd

import (
	"bufio"
	"clipack/internal"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var instructionsCmd = &cobra.Command{
	Use:   "instructions",
	Short: "Manage installation instructions for applications",
}

var addInstructionCmd = &cobra.Command{
	Use:   "add",
	Short: "Add new installation instructions",
	Run: func(cmd *cobra.Command, args []string) {
		db := internal.InitDB()
		defer db.Close()

		reader := bufio.NewReader(os.Stdin)

		fmt.Print("Enter application name: ")
		name, _ := reader.ReadString('\n')
		name = strings.TrimSpace(name)

		fmt.Print("Enter download URL: ")
		downloadURL, _ := reader.ReadString('\n')
		downloadURL = strings.TrimSpace(downloadURL)

		fmt.Print("Enter install command: ")
		installCommand, _ := reader.ReadString('\n')
		installCommand = strings.TrimSpace(installCommand)

		fmt.Print("Enter version: ")
		version, _ := reader.ReadString('\n')
		version = strings.TrimSpace(version)

		fmt.Print("Enter additional config: ")
		config, _ := reader.ReadString('\n')
		config = strings.TrimSpace(config)

		internal.AddInstruction(db, name, downloadURL, installCommand, version, config)
		fmt.Println("Installation instructions added successfully.")
	},
}

var listInstructionsCmd = &cobra.Command{
	Use:   "list",
	Short: "List all installation instructions",
	Run: func(cmd *cobra.Command, args []string) {
		db := internal.InitDB()
		defer db.Close()

		fmt.Println("Listing all installation instructions...")
		internal.ListInstructions(db)
	},
}

var editInstructionCmd = &cobra.Command{
	Use:   "edit",
	Short: "Edit existing installation instructions",
	Run: func(cmd *cobra.Command, args []string) {
		db := internal.InitDB()
		defer db.Close()

		reader := bufio.NewReader(os.Stdin)

		fmt.Print("Enter the ID of the instruction to edit: ")
		idStr, _ := reader.ReadString('\n')
		idStr = strings.TrimSpace(idStr)
		id, err := strconv.Atoi(idStr)
		if err != nil {
			fmt.Println("Invalid ID")
			return
		}

		fmt.Print("Enter new application name (leave blank to keep current): ")
		name, _ := reader.ReadString('\n')
		name = strings.TrimSpace(name)

		fmt.Print("Enter new download URL (leave blank to keep current): ")
		downloadURL, _ := reader.ReadString('\n')
		downloadURL = strings.TrimSpace(downloadURL)

		fmt.Print("Enter new install command (leave blank to keep current): ")
		installCommand, _ := reader.ReadString('\n')
		installCommand = strings.TrimSpace(installCommand)

		fmt.Print("Enter new version (leave blank to keep current): ")
		version, _ := reader.ReadString('\n')
		version = strings.TrimSpace(version)

		fmt.Print("Enter new additional config (leave blank to keep current): ")
		config, _ := reader.ReadString('\n')
		config = strings.TrimSpace(config)

		internal.EditInstruction(db, id, name, downloadURL, installCommand, version, config)
		fmt.Println("Installation instructions edited successfully.")
	},
}

func init() {
	rootCmd.AddCommand(instructionsCmd)
	instructionsCmd.AddCommand(addInstructionCmd)
	instructionsCmd.AddCommand(listInstructionsCmd)
	instructionsCmd.AddCommand(editInstructionCmd)
}
