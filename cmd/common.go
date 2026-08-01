package cmd

import (
	"fmt"
	"os"

	"github.com/lvim-tech/clipack/cnfg"
	"github.com/lvim-tech/clipack/pkg"
	"github.com/lvim-tech/clipack/utils"
)

// loadConfig makes sure a configuration exists and returns it.
// Every subcommand starts here, so a fresh installation is bootstrapped no
// matter which command the user reaches for first.
func loadConfig() (*cnfg.Config, error) {
	if err := cnfg.CreateDefaultConfig(); err != nil {
		return nil, fmt.Errorf("creating config: %w", err)
	}

	config, err := cnfg.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	if err := config.EnsureDirs(); err != nil {
		return nil, err
	}

	return config, nil
}

// loadPackages returns the registry contents, optionally forcing a refresh.
func loadPackages(config *cnfg.Config, forceRefresh bool) ([]*pkg.Package, error) {
	if forceRefresh {
		if err := pkg.ClearCache(config); err != nil {
			return nil, fmt.Errorf("clearing cache: %w", err)
		}
		fmt.Fprintln(os.Stderr, "Refreshing registry…")
		packages, err := pkg.RefreshRegistry(config)
		if len(packages) == 0 {
			return nil, err
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "warning:", err)
		}
		return packages, nil
	}

	packages, err := pkg.LoadAllPackagesFromRegistry(config)
	if len(packages) == 0 {
		return nil, err
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning:", err)
	}
	return packages, nil
}

// cliReporter prints installer events to the terminal. It is the plain-text
// counterpart of the TUI's event log.
func cliReporter(e pkg.Event) {
	switch e.Kind {
	case pkg.EventStep:
		fmt.Printf("▶ [%d/%d] %s\n", e.Step, e.Total, e.Text)
	case pkg.EventOutput:
		fmt.Println("  │ " + e.Text)
	case pkg.EventWarn:
		fmt.Fprintln(os.Stderr, "! "+e.Text)
	case pkg.EventError:
		fmt.Fprintln(os.Stderr, "✗ "+e.Text)
	case pkg.EventDone:
		fmt.Println("✓ " + e.Text)
	default:
		fmt.Println(e.Text)
	}
}

// newInstaller builds an installer wired to the CLI reporter.
func newInstaller(config *cnfg.Config) *pkg.Installer {
	return pkg.NewInstaller(config, cliReporter)
}

// askYes prompts for confirmation on stdin.
func askYes(question string) bool {
	return utils.AskForConfirmation(question)
}
