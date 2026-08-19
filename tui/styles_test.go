package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/lvim-tech/clipack/cnfg"
	"github.com/lvim-tech/clipack/pkg"
)

func TestNewStylesCarriesTheThemeAndIcons(t *testing.T) {
	theme, err := cnfg.ResolveTheme(cnfg.Theme{Name: "mono"})
	if err != nil {
		t.Fatal(err)
	}

	s := NewStyles(theme)
	if s.Theme.Name != "mono" {
		t.Errorf("Theme.Name = %q, want mono", s.Theme.Name)
	}
	// The glyph set travels with the styles so every renderer uses the same one.
	if s.Icons.Done != "+" {
		t.Errorf("Icons.Done = %q, want the ascii glyph the mono preset asks for", s.Icons.Done)
	}
}

func TestNewStylesAppliesColors(t *testing.T) {
	theme := cnfg.Theme{
		Border: "rounded",
		Icons:  cnfg.IconsUnicode,
		Colors: cnfg.ColorSet{
			Accent:  cnfg.Color{Light: "#111111", Dark: "#111111"},
			Success: cnfg.Color{Light: "#00ff00", Dark: "#00ff00"},
		},
	}

	s := NewStyles(theme)

	if got := s.Cursor.GetForeground(); got == nil {
		t.Error("the accent colour was not applied to the cursor style")
	}
	if got := s.OK.GetForeground(); got == nil {
		t.Error("the success colour was not applied")
	}
}

func TestNewStylesLeavesUnsetColorsAlone(t *testing.T) {
	// lipgloss has no "no colour" value, so an unset entry must simply not be
	// applied — otherwise "mono" would paint everything black.
	s := NewStyles(cnfg.Theme{Border: "normal"})

	if fg := s.Text.GetForeground(); fg != lipgloss.NoColor(struct{}{}) && fg != nil {
		// lipgloss returns NoColor for an unset foreground.
		if _, isNoColor := fg.(lipgloss.NoColor); !isNoColor {
			t.Errorf("Text foreground = %#v, want it left unset", fg)
		}
	}
}

func TestNewStylesFallsBackToWeightWithoutColor(t *testing.T) {
	// The mono preset has no colours at all, so emphasis has to come from
	// weight and dimming or installed and outdated packages become identical.
	s := NewStyles(cnfg.Theme{Icons: cnfg.IconsASCII})

	if !s.BadgeInstalled.GetBold() {
		t.Error("BadgeInstalled is not bold; nothing distinguishes it without colour")
	}
	if !s.Muted.GetFaint() {
		t.Error("Muted is not faint; secondary text would read as body text")
	}
	if !s.Title.GetReverse() {
		t.Error("Title is not reversed; the badge would be invisible without a background")
	}
}

func TestBorderStyle(t *testing.T) {
	tests := map[string]lipgloss.Border{
		// Right angles only: "rounded" stays accepted for old configs, but it
		// draws square, and so does anything unrecognised.
		"rounded":  lipgloss.NormalBorder(),
		"normal":   lipgloss.NormalBorder(),
		"thick":    lipgloss.ThickBorder(),
		"double":   lipgloss.DoubleBorder(),
		"hidden":   lipgloss.HiddenBorder(),
		"":         lipgloss.NormalBorder(),
		"nonsense": lipgloss.NormalBorder(),
	}

	for name, want := range tests {
		if got := borderStyle(name); got.TopLeft != want.TopLeft {
			t.Errorf("borderStyle(%q).TopLeft = %q, want %q", name, got.TopLeft, want.TopLeft)
		}
	}
}

func TestBorderNoneDrawsNoBorder(t *testing.T) {
	withBorder := NewStyles(cnfg.Theme{Border: "normal"})
	without := NewStyles(cnfg.Theme{Border: "none"})

	if !withBorder.Pane.GetBorderTop() {
		t.Fatal("the normal border was not applied")
	}
	if without.Pane.GetBorderTop() {
		t.Error(`border: none still drew a border`)
	}
}

func TestThemeChangesTheRenderedGlyphs(t *testing.T) {
	unicode, err := cnfg.ResolveTheme(cnfg.Theme{Name: "default"})
	if err != nil {
		t.Fatal(err)
	}
	ascii, err := cnfg.ResolveTheme(cnfg.Theme{Name: "default", Icons: cnfg.IconsASCII})
	if err != nil {
		t.Fatal(err)
	}

	entry := packageItem{pkg: samplePackagesFirst(), installed: samplePackagesFirst()}

	unicodeOut := renderStatus(entry, NewStyles(unicode))
	asciiOut := renderStatus(entry, NewStyles(ascii))

	// renderStatus itself has no glyphs, but the detail pane does; check the
	// bullet used for lists instead.
	unicodeDetail := renderDetail(entry, pkg.MethodVersion, 60, NewStyles(unicode))
	asciiDetail := renderDetail(entry, pkg.MethodVersion, 60, NewStyles(ascii))

	if !strings.Contains(unicodeDetail, "•") {
		t.Errorf("the unicode theme did not use its bullet:\n%s", unicodeDetail)
	}
	if strings.Contains(asciiDetail, "•") {
		t.Errorf("the ascii theme still used a unicode bullet:\n%s", asciiDetail)
	}
	if unicodeOut == "" || asciiOut == "" {
		t.Error("renderStatus produced nothing")
	}
}

func TestFormatEventUsesTheThemeGlyphs(t *testing.T) {
	ascii, err := cnfg.ResolveTheme(cnfg.Theme{Name: "default", Icons: cnfg.IconsASCII})
	if err != nil {
		t.Fatal(err)
	}

	line := formatEvent(errorEvent(), NewStyles(ascii))
	if strings.Contains(line, "✗") {
		t.Errorf("formatEvent = %q, want the ascii marker", line)
	}
	if !strings.Contains(line, "x ") {
		t.Errorf("formatEvent = %q, want it to start with the ascii error marker", line)
	}
}

func TestModelUsesTheConfiguredTheme(t *testing.T) {
	config := testConfig(t)
	config.Theme = cnfg.Theme{Name: "mono"}

	m := New(config)
	if m.styles.Theme.Name != "mono" {
		t.Errorf("model theme = %q, want mono", m.styles.Theme.Name)
	}
	if m.status != "" {
		t.Errorf("status = %q, want no theme warning for a valid theme", m.status)
	}
}

func TestModelReportsAnUnresolvableTheme(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	config := testConfig(t)
	config.Theme = cnfg.Theme{Name: "nosuchtheme"}

	m := New(config)

	// A broken theme must not stop clipack; the reason is surfaced instead.
	if m.styles.Theme.Name != cnfg.DefaultThemeName {
		t.Errorf("theme = %q, want the default fallback", m.styles.Theme.Name)
	}
	if !strings.Contains(m.status, "nosuchtheme") {
		t.Errorf("status = %q, want it to explain which theme failed", m.status)
	}
}

func TestDefaultStyles(t *testing.T) {
	s := DefaultStyles()
	if s.Theme.Name != cnfg.DefaultThemeName {
		t.Errorf("DefaultStyles() theme = %q, want %q", s.Theme.Name, cnfg.DefaultThemeName)
	}
	if s.Icons.Done == "" {
		t.Error("DefaultStyles() has no glyphs")
	}
}
