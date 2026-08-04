package tui

import (
	"errors"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lvim-tech/clipack/cnfg"
)

// Run starts the interactive TUI. When no configuration exists yet the model
// opens the first-run wizard instead of the package browser, so clipack is
// usable straight after installation without touching a config file by hand.
//
// The alternate screen buffer is used so the user's scrollback survives, and
// mouse cell motion is enabled for wheel scrolling in the list and log panes.
func Run() error {
	var config *cnfg.Config

	if cnfg.Exists() {
		loaded, err := cnfg.LoadConfig()
		if err != nil {
			// Printed rather than wrapped: an unfinished setup needs the
			// instructions, and inside an alt-screen TUI they would be lost.
			if errors.Is(err, cnfg.ErrNoRegistry) {
				return errors.New(cnfg.RegistryHelp())
			}
			return fmt.Errorf("could not load configuration: %w", err)
		}
		// Directories may have been removed since the config was written.
		if err := loaded.EnsureDirs(); err != nil {
			return err
		}
		config = loaded
	}

	program := tea.NewProgram(
		New(config),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)

	_, err := program.Run()
	return err
}
