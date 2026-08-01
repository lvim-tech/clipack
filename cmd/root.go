// Package cmd wires clipack's command line interface.
//
// The commands are deliberately thin: every operation lives in pkg (registry
// access, install/update/remove) or tui (the interactive front-end), so the CLI
// and the TUI share exactly the same behaviour.
//
// Running clipack without a subcommand opens the Bubble Tea interface. The
// subcommands remain non-interactive so they can be used from scripts.
package cmd

import (
	"fmt"
	"os"

	"github.com/lvim-tech/clipack/tui"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "clipack",
	Short: "Clipack is a package manager for CLI tools and their configurations.",
	Long: `Clipack builds CLI tools from source and manages their binaries,
man pages and configuration files.

Run "clipack" with no arguments to open the interactive interface.`,
	SilenceUsage:  true,
	SilenceErrors: true,
	// Args are rejected here so a typo like "clipack instal bat" reports an
	// unknown command instead of silently opening the TUI.
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return tui.Run()
	},
}

// tuiCmd exposes the interface explicitly, which is handy in aliases and docs.
var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Open the interactive interface",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return tui.Run()
	},
}

// Execute runs the root command and converts an error into a non-zero exit.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "clipack:", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(tuiCmd)
}
