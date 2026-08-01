package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/lvim-tech/clipack/cnfg"
	"github.com/spf13/cobra"
)

var themeShowColors bool

// themeCmd lists the available themes, or switches to one. Themes are either
// built in or installed as files in ~/.config/clipack/themes.
var themeCmd = &cobra.Command{
	Use:   "theme [name]",
	Short: "List the available themes, or switch to one",
	Long: `Without arguments, list every theme clipack can use and mark the active one.

A theme is either built in or a file in ~/.config/clipack/themes. Generated
themes, such as the ones lvim-colorscheme produces, are installed by copying
them into that directory:

    mkdir -p ~/.config/clipack/themes
    cp extras/clipack/*.yaml ~/.config/clipack/themes/

Given a name, the theme is written to config.yaml. Individual colours, the
border and the icon set can still be overridden there afterwards.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		config, err := loadConfig()
		if err != nil {
			return err
		}

		if len(args) == 0 {
			return listThemes(config)
		}
		return setTheme(config, args[0])
	},
}

// listThemes prints the built-in and installed themes.
func listThemes(config *cnfg.Config) error {
	custom, err := cnfg.CustomThemeNames()
	if err != nil {
		return fmt.Errorf("reading the themes directory: %w", err)
	}

	active := config.Theme.Name
	if active == "" {
		active = cnfg.DefaultThemeName
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "\tNAME\tSOURCE")
	for _, name := range cnfg.BuiltinThemeNames() {
		fmt.Fprintf(w, "%s\t%s\tbuilt-in\n", marker(name == active), name)
	}
	for _, name := range custom {
		fmt.Fprintf(w, "%s\t%s\tinstalled\n", marker(name == active), name)
	}
	if err := w.Flush(); err != nil {
		return err
	}

	dir, err := cnfg.ThemesDir()
	if err == nil {
		fmt.Printf("\nInstalled themes are read from %s\n", dir)
	}
	if len(custom) == 0 {
		fmt.Println("No themes are installed there yet.")
	}

	// An active theme that resolves to nothing is worth saying out loud: the
	// interface silently falls back to the default, which looks like the
	// setting was ignored.
	if _, err := cnfg.ResolveTheme(config.Theme); err != nil {
		fmt.Fprintf(os.Stderr, "\nwarning: %v\n", err)
	}

	if themeShowColors {
		return showPalette(config)
	}

	fmt.Printf("\nRun 'clipack theme <name>' to switch, or 'clipack theme --colors' to show the active palette.\n")
	return nil
}

// showPalette prints the resolved palette of the active theme.
func showPalette(config *cnfg.Config) error {
	theme, err := cnfg.ResolveTheme(config.Theme)
	if err != nil {
		return err
	}

	fmt.Printf("\n%s\n", theme.Name)
	fmt.Printf("  border  %s\n", theme.Border)
	fmt.Printf("  icons   %s\n", theme.Icons)
	fmt.Println()

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	for _, entry := range theme.Colors.Named() {
		value := entry.Color.String()
		if value == "" {
			value = "-"
		}
		fmt.Fprintf(w, "  %s\t%s\n", entry.Key, value)
	}
	return w.Flush()
}

// setTheme writes the chosen theme to config.yaml.
func setTheme(config *cnfg.Config, name string) error {
	// Resolve before writing, so a typo is reported instead of being saved and
	// silently falling back at the next start.
	if _, err := cnfg.ResolveTheme(cnfg.Theme{Name: name}); err != nil {
		return err
	}

	config.Theme.Name = name
	if err := config.Save(); err != nil {
		return err
	}

	path, _ := cnfg.ConfigPath()
	fmt.Printf("Theme set to %s in %s\n", name, path)

	// Overrides win over the theme, so a leftover one explains why nothing
	// appeared to change.
	var overridden []string
	if config.Theme.Border != "" {
		overridden = append(overridden, "border")
	}
	if config.Theme.Icons != "" {
		overridden = append(overridden, "icons")
	}
	for _, entry := range config.Theme.Colors.Named() {
		if !entry.Color.IsZero() {
			overridden = append(overridden, entry.Key)
		}
	}
	if len(overridden) > 0 {
		fmt.Printf("Note: config.yaml still overrides %s\n", strings.Join(overridden, ", "))
	}

	return nil
}

// marker renders the active-theme indicator.
func marker(active bool) string {
	if active {
		return "*"
	}
	return " "
}

func init() {
	themeCmd.Flags().BoolVarP(&themeShowColors, "colors", "c", false, "Show the resolved palette of the active theme")
	rootCmd.AddCommand(themeCmd)
}
