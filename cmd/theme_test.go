package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lvim-tech/clipack/cnfg"
)

// installTheme writes a theme file into the themes directory.
func installTheme(t *testing.T, name, contents string) {
	t.Helper()

	dir, err := cnfg.ThemesDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

// generatedTheme mirrors what lvim-colorscheme writes for clipack.
const generatedTheme = `---
name: LvimNord_dark
border: normal
icons: unicode
colors:
    accent: "#a58aa0"
    accent_alt: "#8097af"
    text: "#b3bac6"
    muted: "#677185"
    subtle: "#3c475a"
    success: "#97ab86"
    warning: "#cbae72"
    error: "#af7177"
    title_fg: "#232831"
`

func TestThemeListsBuiltinsAndInstalled(t *testing.T) {
	setupCmdTest(t)
	installTheme(t, "LvimNord_dark.yaml", generatedTheme)

	stdout, _, err := execute(t, "theme")
	if err != nil {
		t.Fatalf("theme error = %v", err)
	}

	for _, want := range []string{"default", "mono", "built-in", "LvimNord_dark", "installed"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("theme output is missing %q:\n%s", want, stdout)
		}
	}
	// The active theme is marked so the listing answers "which one am I on".
	if !strings.Contains(stdout, "*  default") {
		t.Errorf("the active theme is not marked:\n%s", stdout)
	}
	if !strings.Contains(stdout, "themes") {
		t.Errorf("the output does not say where themes are read from:\n%s", stdout)
	}
}

func TestThemeSaysWhenNoneAreInstalled(t *testing.T) {
	setupCmdTest(t)

	stdout, _, err := execute(t, "theme")
	if err != nil {
		t.Fatalf("theme error = %v", err)
	}
	if !strings.Contains(stdout, "No themes are installed") {
		t.Errorf("output does not mention the empty themes directory:\n%s", stdout)
	}
}

func TestThemeSetsTheTheme(t *testing.T) {
	setupCmdTest(t)
	installTheme(t, "LvimNord_dark.yaml", generatedTheme)

	stdout, _, err := execute(t, "theme", "LvimNord_dark")
	if err != nil {
		t.Fatalf("theme error = %v", err)
	}
	if !strings.Contains(stdout, "LvimNord_dark") {
		t.Errorf("output does not confirm the change:\n%s", stdout)
	}

	config, err := cnfg.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Theme.Name != "LvimNord_dark" {
		t.Errorf("Theme.Name = %q, want it written to config.yaml", config.Theme.Name)
	}
}

func TestThemeSetPreservesTheRestOfTheConfig(t *testing.T) {
	config := setupCmdTest(t)
	config.Registry.Token = "secret-token"
	config.Options.InstallMethod = "commit"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}

	if _, _, err := execute(t, "theme", "mono"); err != nil {
		t.Fatalf("theme error = %v", err)
	}

	loaded, err := cnfg.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Registry.Token != "secret-token" {
		t.Errorf("Token = %q, want it preserved by a theme change", loaded.Registry.Token)
	}
	if loaded.Options.InstallMethod != "commit" {
		t.Errorf("InstallMethod = %q, want it preserved", loaded.Options.InstallMethod)
	}
}

func TestThemeRejectsAnUnknownName(t *testing.T) {
	config := setupCmdTest(t)

	_, _, err := execute(t, "theme", "nosuchtheme")
	if err == nil {
		t.Fatal("theme error = nil, want an unknown theme rejected")
	}
	if !strings.Contains(err.Error(), "nosuchtheme") {
		t.Errorf("error = %v, want it to name the theme", err)
	}

	// A typo must not be written to disk, or the next start would silently
	// fall back to the default.
	loaded, err := cnfg.LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Theme.Name != config.Theme.Name {
		t.Errorf("Theme.Name = %q, want the config left untouched", loaded.Theme.Name)
	}
}

func TestThemeWarnsAboutRemainingOverrides(t *testing.T) {
	config := setupCmdTest(t)
	config.Theme.Border = "thick"
	config.Theme.Colors.Accent = cnfg.Color{Light: "#111111", Dark: "#111111"}
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := execute(t, "theme", "mono")
	if err != nil {
		t.Fatalf("theme error = %v", err)
	}

	// Overrides win over the theme, so a leftover one explains why switching
	// appeared to do nothing.
	if !strings.Contains(stdout, "still overrides") {
		t.Errorf("output does not warn about the overrides:\n%s", stdout)
	}
	for _, want := range []string{"border", "accent"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("the warning does not name %q:\n%s", want, stdout)
		}
	}
}

func TestThemeColorsShowsTheResolvedPalette(t *testing.T) {
	setupCmdTest(t)
	installTheme(t, "LvimNord_dark.yaml", generatedTheme)

	if _, _, err := execute(t, "theme", "LvimNord_dark"); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := execute(t, "theme", "--colors")
	if err != nil {
		t.Fatalf("theme --colors error = %v", err)
	}

	for _, want := range []string{"LvimNord_dark", "border", "normal", "icons", "unicode", "accent", "#a58aa0", "title_fg"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("palette output is missing %q:\n%s", want, stdout)
		}
	}
}

func TestThemeWarnsWhenTheActiveThemeIsMissing(t *testing.T) {
	config := setupCmdTest(t)
	// Written directly, bypassing the command's own validation, as if the file
	// had been deleted after being selected.
	config.Theme.Name = "deleted-theme"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}

	_, stderr, err := execute(t, "theme")
	if err != nil {
		t.Fatalf("theme error = %v, want the listing to still work", err)
	}
	if !strings.Contains(stderr, "deleted-theme") {
		t.Errorf("stderr = %q, want a warning that the active theme cannot be resolved", stderr)
	}
}

func TestThemeAcceptsAtMostOneArgument(t *testing.T) {
	cmd := findCommand(t, "theme")
	if err := cmd.Args(cmd, []string{"a", "b"}); err == nil {
		t.Error("theme accepted two names")
	}
	if err := cmd.Args(cmd, []string{"a"}); err != nil {
		t.Errorf("theme rejected a single name: %v", err)
	}
	if err := cmd.Args(cmd, nil); err != nil {
		t.Errorf("theme rejected an empty argument list: %v", err)
	}
}

func TestGeneratedThemeFileIsUsable(t *testing.T) {
	setupCmdTest(t)
	installTheme(t, "LvimNord_dark.yaml", generatedTheme)

	// The exact shape lvim-colorscheme emits has to resolve without any
	// further editing: this is the contract between the two repositories.
	theme, err := cnfg.ResolveTheme(cnfg.Theme{Name: "LvimNord_dark"})
	if err != nil {
		t.Fatalf("ResolveTheme() error = %v", err)
	}

	if theme.Border != "normal" || theme.Icons != cnfg.IconsUnicode {
		t.Errorf("border/icons = %q/%q, want the generated values", theme.Border, theme.Icons)
	}
	for _, entry := range theme.Colors.Named() {
		if entry.Color.IsZero() {
			t.Errorf("colour %q is unset; the generated file should be complete", entry.Key)
		}
	}
	if theme.Colors.Accent.Dark != "#a58aa0" {
		t.Errorf("accent = %v, want the generated value", theme.Colors.Accent)
	}
}
