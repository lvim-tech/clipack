package cnfg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// writeTheme installs a theme file into the themes directory.
func writeTheme(t *testing.T, name, contents string) {
	t.Helper()

	dir, err := ThemesDir()
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

func TestColorUnmarshalScalar(t *testing.T) {
	var set ColorSet
	if err := yaml.Unmarshal([]byte(`accent: "#b48ead"`), &set); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	// A generated theme file writes scalars: one value applies to both
	// backgrounds because the file is already a palette for one of them.
	if set.Accent.Light != "#b48ead" || set.Accent.Dark != "#b48ead" {
		t.Errorf("accent = %+v, want the scalar applied to both backgrounds", set.Accent)
	}
}

func TestColorUnmarshalMapping(t *testing.T) {
	var set ColorSet
	input := `accent: {light: "#6c3ad6", dark: "#b48ead"}`
	if err := yaml.Unmarshal([]byte(input), &set); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	if set.Accent.Light != "#6c3ad6" || set.Accent.Dark != "#b48ead" {
		t.Errorf("accent = %+v, want the two backgrounds kept apart", set.Accent)
	}
}

func TestColorUnmarshalPartialMapping(t *testing.T) {
	var set ColorSet
	if err := yaml.Unmarshal([]byte(`accent: {dark: "#b48ead"}`), &set); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	// Half a mapping still has to produce a usable colour on both backgrounds.
	if set.Accent.Light != "#b48ead" {
		t.Errorf("accent.light = %q, want it filled from dark", set.Accent.Light)
	}
}

func TestColorMarshalRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		color Color
		want  string
	}{
		{"shared value writes a scalar", Color{Light: "#aaa", Dark: "#aaa"}, "accent: '#aaa'"},
		{"split value writes a mapping", Color{Light: "#111", Dark: "#eee"}, "light:"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := yaml.Marshal(ColorSet{Accent: tt.color})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), tt.want) {
				t.Errorf("marshalled to %q, want it to contain %q", data, tt.want)
			}

			var back ColorSet
			if err := yaml.Unmarshal(data, &back); err != nil {
				t.Fatalf("re-parsing: %v", err)
			}
			if back.Accent != tt.color {
				t.Errorf("round trip = %+v, want %+v", back.Accent, tt.color)
			}
		})
	}
}

func TestColorIsZeroAndString(t *testing.T) {
	if !(Color{}).IsZero() {
		t.Error("an empty colour is not reported as zero")
	}
	if (Color{Dark: "#000"}).IsZero() {
		t.Error("a half-set colour is reported as zero")
	}

	if got := (Color{Light: "#aaa", Dark: "#aaa"}).String(); got != "#aaa" {
		t.Errorf("String() = %q, want the single value", got)
	}
	if got := (Color{Light: "#111", Dark: "#eee"}).String(); !strings.Contains(got, "light:#111") {
		t.Errorf("String() = %q, want both backgrounds named", got)
	}
}

func TestResolveThemeBuiltin(t *testing.T) {
	withHome(t)

	theme, err := ResolveTheme(Theme{Name: "mono"})
	if err != nil {
		t.Fatalf("ResolveTheme() error = %v", err)
	}
	if theme.Name != "mono" {
		t.Errorf("Name = %q, want mono", theme.Name)
	}
	if theme.Icons != IconsASCII {
		t.Errorf("Icons = %q, want the preset's ascii", theme.Icons)
	}
	if theme.Border != "normal" {
		t.Errorf("Border = %q, want the preset's normal", theme.Border)
	}
}

func TestResolveThemeEmptyNameIsDefault(t *testing.T) {
	withHome(t)

	theme, err := ResolveTheme(Theme{})
	if err != nil {
		t.Fatalf("ResolveTheme() error = %v", err)
	}
	if theme.Name != DefaultThemeName {
		t.Errorf("Name = %q, want %q", theme.Name, DefaultThemeName)
	}
	if theme.Colors.Accent.IsZero() {
		t.Error("the default theme resolved without an accent colour")
	}
}

func TestResolveThemeOverrides(t *testing.T) {
	withHome(t)

	theme, err := ResolveTheme(Theme{
		Name:   "default",
		Border: "thick",
		Colors: ColorSet{Accent: Color{Light: "#111111", Dark: "#111111"}},
	})
	if err != nil {
		t.Fatalf("ResolveTheme() error = %v", err)
	}

	if theme.Border != "thick" {
		t.Errorf("Border = %q, want the override", theme.Border)
	}
	if theme.Colors.Accent.Dark != "#111111" {
		t.Errorf("accent = %v, want the override", theme.Colors.Accent)
	}
	// Overriding one colour must not blank the rest of the palette.
	if theme.Colors.Success.IsZero() {
		t.Error("overriding accent cleared success; the base palette was not kept")
	}
	if theme.Icons != DefaultIcons {
		t.Errorf("Icons = %q, want the base value", theme.Icons)
	}
}

func TestResolveThemeFromFile(t *testing.T) {
	withHome(t)

	writeTheme(t, "LvimNord_dark.yaml", `---
name: LvimNord_dark
border: double
icons: ascii
colors:
    accent: "#a58aa0"
    text: "#b3bac6"
`)

	theme, err := ResolveTheme(Theme{Name: "LvimNord_dark"})
	if err != nil {
		t.Fatalf("ResolveTheme() error = %v", err)
	}

	if theme.Name != "LvimNord_dark" {
		t.Errorf("Name = %q", theme.Name)
	}
	if theme.Border != "double" || theme.Icons != IconsASCII {
		t.Errorf("border/icons = %q/%q, want the file's values", theme.Border, theme.Icons)
	}
	if theme.Colors.Accent.Dark != "#a58aa0" {
		t.Errorf("accent = %v, want the file's value", theme.Colors.Accent)
	}
	// A file that omits a colour still gets a usable one from the default
	// preset, so a partial theme cannot render an invisible interface.
	if theme.Colors.Success.IsZero() {
		t.Error("a colour the file omitted was not filled from the default preset")
	}
}

func TestResolveThemeFileWithYmlExtension(t *testing.T) {
	withHome(t)
	writeTheme(t, "custom.yml", "colors:\n    accent: \"#123456\"\n")

	theme, err := ResolveTheme(Theme{Name: "custom"})
	if err != nil {
		t.Fatalf("ResolveTheme() error = %v", err)
	}
	if theme.Colors.Accent.Dark != "#123456" {
		t.Errorf("accent = %v, want the .yml file to be found too", theme.Colors.Accent)
	}
}

func TestResolveThemeConfigOverridesFile(t *testing.T) {
	withHome(t)
	writeTheme(t, "custom.yaml", "border: double\ncolors:\n    accent: \"#111111\"\n")

	theme, err := ResolveTheme(Theme{
		Name:   "custom",
		Border: "thick",
		Colors: ColorSet{Accent: Color{Light: "#999999", Dark: "#999999"}},
	})
	if err != nil {
		t.Fatalf("ResolveTheme() error = %v", err)
	}

	// config.yaml is the last word: it is where a per-machine tweak lives.
	if theme.Border != "thick" {
		t.Errorf("Border = %q, want config.yaml to win over the file", theme.Border)
	}
	if theme.Colors.Accent.Dark != "#999999" {
		t.Errorf("accent = %v, want config.yaml to win over the file", theme.Colors.Accent)
	}
}

func TestResolveThemeMissing(t *testing.T) {
	withHome(t)

	theme, err := ResolveTheme(Theme{Name: "nosuchtheme"})
	if err == nil {
		t.Fatal("ResolveTheme() error = nil, want a missing theme to be reported")
	}
	if !strings.Contains(err.Error(), "nosuchtheme") {
		t.Errorf("error = %v, want it to name the theme", err)
	}
	// A usable theme is still returned so the interface can render the error.
	if theme.Colors.Accent.IsZero() {
		t.Error("no fallback theme was returned alongside the error")
	}
}

func TestResolveThemeBrokenFile(t *testing.T) {
	withHome(t)
	writeTheme(t, "broken.yaml", "colors: [unterminated")

	if _, err := ResolveTheme(Theme{Name: "broken"}); err == nil {
		t.Error("ResolveTheme() error = nil, want a parse error")
	}
}

func TestLoadThemeFileRejectsPaths(t *testing.T) {
	withHome(t)

	// A theme name comes from a config file, so it must not be usable to read
	// an arbitrary path.
	for _, name := range []string{"../config", "..", ".", "sub/theme", "/etc/passwd"} {
		if _, err := LoadThemeFile(name); err == nil {
			t.Errorf("LoadThemeFile(%q) error = nil, want it rejected", name)
		}
	}
}

func TestCustomThemeNames(t *testing.T) {
	withHome(t)

	// Nothing installed yet is a normal state, not an error.
	names, err := CustomThemeNames()
	if err != nil {
		t.Fatalf("CustomThemeNames() with no directory error = %v", err)
	}
	if len(names) != 0 {
		t.Errorf("got %v, want none", names)
	}

	writeTheme(t, "Zebra.yaml", "")
	writeTheme(t, "Alpha.yaml", "")
	writeTheme(t, "Legacy.yml", "")
	writeTheme(t, "notes.txt", "ignored")

	names, err = CustomThemeNames()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Alpha", "Legacy", "Zebra"}
	if len(names) != len(want) {
		t.Fatalf("got %v, want %v", names, want)
	}
	for i, name := range want {
		if names[i] != name {
			t.Errorf("names = %v, want %v (sorted, extension stripped, non-yaml ignored)", names, want)
		}
	}
}

func TestThemeIconSet(t *testing.T) {
	if got := (Theme{Icons: IconsASCII}).IconSet(); got.Done != "+" {
		t.Errorf("ascii Done = %q, want a plain glyph", got.Done)
	}
	if got := (Theme{Icons: IconsUnicode}).IconSet(); got.Done != "✓" {
		t.Errorf("unicode Done = %q", got.Done)
	}
	// An unset icon set falls back to unicode rather than to empty glyphs.
	if got := (Theme{}).IconSet(); got.Done == "" {
		t.Error("an unset icon set produced empty glyphs")
	}
}

func TestBuiltinThemeNames(t *testing.T) {
	names := BuiltinThemeNames()
	if len(names) < 2 {
		t.Fatalf("got %v, want at least default and mono", names)
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Errorf("names = %v, want them sorted", names)
			break
		}
	}

	if _, ok := BuiltinTheme("default"); !ok {
		t.Error("the default preset is missing")
	}
	if _, ok := BuiltinTheme("nosuchtheme"); ok {
		t.Error("BuiltinTheme returned a theme that does not exist")
	}
}

func TestMonoThemeHasNoColors(t *testing.T) {
	withHome(t)

	theme, err := ResolveTheme(Theme{Name: "mono"})
	if err != nil {
		t.Fatal(err)
	}
	// The whole point of mono is to leave the terminal's own colours alone.
	for _, entry := range theme.Colors.Named() {
		if !entry.Color.IsZero() {
			t.Errorf("mono defines %s = %v, want no colour at all", entry.Key, entry.Color)
		}
	}
}

func TestValidateTheme(t *testing.T) {
	tests := []struct {
		name    string
		theme   Theme
		wantErr string
	}{
		{name: "empty is fine", theme: Theme{}},
		{name: "known values", theme: Theme{Border: "double", Icons: IconsASCII}},
		{name: "unknown border", theme: Theme{Border: "squiggly"}, wantErr: "unknown border"},
		{name: "unknown icons", theme: Theme{Icons: "emoji"}, wantErr: "unknown icon set"},
		{
			name:    "bad colour",
			theme:   Theme{Colors: ColorSet{Accent: Color{Light: "reddish", Dark: "reddish"}}},
			wantErr: "accent",
		},
		{
			name:    "hex without a hash",
			theme:   Theme{Colors: ColorSet{Text: Color{Light: "b48ead", Dark: "b48ead"}}},
			wantErr: "text",
		},
		{
			name:  "ansi index is accepted",
			theme: Theme{Colors: ColorSet{Text: Color{Light: "5", Dark: "12"}}},
		},
		{
			name:  "short hex is accepted",
			theme: Theme{Colors: ColorSet{Text: Color{Light: "#abc", Dark: "#ABC"}}},
		},
		{
			name:    "out of range ansi index",
			theme:   Theme{Colors: ColorSet{Text: Color{Light: "300", Dark: "300"}}},
			wantErr: "text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			theme := tt.theme
			err := validateTheme(&theme)

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateTheme() error = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateTheme() error = nil, want one mentioning %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %v, want it to mention %q", err, tt.wantErr)
			}
		})
	}
}

func TestValidateThemeDoesNotCheckTheName(t *testing.T) {
	// Resolving a name needs the filesystem. LoadConfig has to keep working
	// when a theme file is temporarily missing, so the name is only checked
	// when the theme is actually resolved.
	theme := Theme{Name: "a-theme-that-does-not-exist"}
	if err := validateTheme(&theme); err != nil {
		t.Errorf("validateTheme() error = %v, want the name left alone", err)
	}
}

func TestConfigCarriesTheTheme(t *testing.T) {
	withHome(t)

	config := NewDefaultConfig(filepath.Join(t.TempDir(), "packages"))
	config.Registry.URL = "https://github.com/owner/repo.git"
	config.Theme = Theme{
		Name:   "mono",
		Border: "thick",
		Colors: ColorSet{Accent: Color{Light: "#111111", Dark: "#eeeeee"}},
	}
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if loaded.Theme.Name != "mono" || loaded.Theme.Border != "thick" {
		t.Errorf("theme = %+v, want it to round-trip", loaded.Theme)
	}
	if loaded.Theme.Colors.Accent.Light != "#111111" || loaded.Theme.Colors.Accent.Dark != "#eeeeee" {
		t.Errorf("accent = %v, want both backgrounds preserved", loaded.Theme.Colors.Accent)
	}
}

func TestNewDefaultConfigNamesTheTheme(t *testing.T) {
	// The key is written out so the knob is discoverable in a fresh config.
	if got := NewDefaultConfig("/opt/clipack").Theme.Name; got != DefaultThemeName {
		t.Errorf("Theme.Name = %q, want %q", got, DefaultThemeName)
	}
}

func TestLoadConfigRejectsABadTheme(t *testing.T) {
	withHome(t)

	config := NewDefaultConfig(filepath.Join(t.TempDir(), "packages"))
	config.Registry.URL = "https://github.com/owner/repo.git"
	config.Theme.Border = "squiggly"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("LoadConfig() error = nil, want the invalid border rejected")
	}
	if !strings.Contains(err.Error(), "theme") {
		t.Errorf("error = %v, want it to point at the theme section", err)
	}
}
