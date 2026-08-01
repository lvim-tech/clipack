package cmd

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCommandsAreRegistered(t *testing.T) {
	want := []string{
		"add-executables-path",
		"install",
		"list",
		"preview",
		"remove",
		"tui",
		"update",
		"update-config",
	}

	registered := make(map[string]bool)
	for _, c := range rootCmd.Commands() {
		registered[c.Name()] = true
	}

	for _, name := range want {
		if !registered[name] {
			t.Errorf("command %q is not registered", name)
		}
	}
}

func TestCommandAliases(t *testing.T) {
	tests := map[string][]string{
		"remove":               {"uninstall", "rm"},
		"add-executables-path": {"path"},
	}

	for name, aliases := range tests {
		cmd := findCommand(t, name)
		for _, alias := range aliases {
			var found bool
			for _, got := range cmd.Aliases {
				if got == alias {
					found = true
				}
			}
			if !found {
				t.Errorf("%s is missing the alias %q (has %v)", name, alias, cmd.Aliases)
			}
		}
	}
}

func TestFlagsAreDeclared(t *testing.T) {
	tests := []struct {
		command   string
		flag      string
		shorthand string
	}{
		{"install", "force-refresh", "f"},
		{"install", "install-method", "m"},
		{"install", "yes", "y"},
		{"update", "force-refresh", "f"},
		{"update", "all", "a"},
		{"update", "yes", "y"},
		{"remove", "yes", "y"},
		{"list", "force-refresh", "f"},
		{"list", "installed", "i"},
		{"list", "updates", "u"},
		{"preview", "force-refresh", "f"},
	}

	for _, tt := range tests {
		t.Run(tt.command+"/"+tt.flag, func(t *testing.T) {
			flag := findCommand(t, tt.command).Flags().Lookup(tt.flag)
			if flag == nil {
				t.Fatalf("%s has no --%s flag", tt.command, tt.flag)
			}
			if flag.Shorthand != tt.shorthand {
				t.Errorf("--%s shorthand = %q, want %q", tt.flag, flag.Shorthand, tt.shorthand)
			}
		})
	}
}

func TestArgumentValidators(t *testing.T) {
	// These commands take no positional arguments, so a typo is reported rather
	// than silently ignored.
	for _, name := range []string{"list", "tui", "update-config", "add-executables-path"} {
		t.Run(name, func(t *testing.T) {
			cmd := findCommand(t, name)
			if cmd.Args == nil {
				t.Fatalf("%s has no argument validator", name)
			}
			if err := cmd.Args(cmd, []string{"unexpected"}); err == nil {
				t.Errorf("%s accepted a positional argument", name)
			}
			if err := cmd.Args(cmd, nil); err != nil {
				t.Errorf("%s rejected an empty argument list: %v", name, err)
			}
		})
	}
}

func TestRootRejectsPositionalArguments(t *testing.T) {
	// Without this, "clipack instal bat" would open the TUI instead of
	// reporting the typo.
	if rootCmd.Args == nil {
		t.Fatal("the root command has no argument validator")
	}
	if err := rootCmd.Args(rootCmd, []string{"instal"}); err == nil {
		t.Error("the root command accepted a stray argument")
	}
}

func TestUnknownCommandIsAnError(t *testing.T) {
	setupCmdTest(t)

	_, _, err := execute(t, "definitely-not-a-command")
	if err == nil {
		t.Fatal("an unknown command returned no error")
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Errorf("error = %v, want it to name the problem", err)
	}
}

func TestErrorsAreNotPrintedTwice(t *testing.T) {
	// SilenceErrors and SilenceUsage keep cobra from printing the error and the
	// whole usage block; Execute() formats it once instead.
	if !rootCmd.SilenceErrors {
		t.Error("SilenceErrors = false; errors would be printed twice")
	}
	if !rootCmd.SilenceUsage {
		t.Error("SilenceUsage = false; a runtime failure would dump the usage block")
	}
}

func TestEveryCommandHasHelpText(t *testing.T) {
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			// Cobra generates "completion" and "help" itself.
			if sub.Name() == "completion" || sub.Name() == "help" {
				continue
			}
			if sub.Short == "" {
				t.Errorf("command %q has no Short description", sub.Name())
			}
			if sub.Use == "" {
				t.Errorf("command %q has no Use string", sub.Name())
			}
			walk(sub)
		}
	}
	walk(rootCmd)
}
