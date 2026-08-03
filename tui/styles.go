package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/lvim-tech/clipack/cnfg"
)

// detailKeyWidth is the column the detail pane's values start at. It has to
// clear the longest label — "Install method" is 14 characters — or the value
// runs straight into the key with no gap between them.
const detailKeyWidth = 16

// Styles is every lipgloss style the interface draws with, compiled once from a
// resolved cnfg.Theme. It is built in NewStyles and then only read, so a theme
// change means building a new Styles rather than mutating package state — which
// is also what makes the rendering testable with an arbitrary palette.
type Styles struct {
	Theme cnfg.Theme
	Icons cnfg.Icons

	App        lipgloss.Style
	Title      lipgloss.Style
	HeaderMeta lipgloss.Style

	Tab       lipgloss.Style
	TabActive lipgloss.Style

	Pane        lipgloss.Style
	PaneFocused lipgloss.Style

	DetailKey   lipgloss.Style
	DetailValue lipgloss.Style
	DetailTitle lipgloss.Style

	BadgeInstalled lipgloss.Style
	BadgeUpdate    lipgloss.Style

	Err   lipgloss.Style
	Warn  lipgloss.Style
	OK    lipgloss.Style
	Muted lipgloss.Style
	Text  lipgloss.Style
	Step  lipgloss.Style

	Cursor lipgloss.Style
	// Selection paints the visual-mode selection and the text cursor in the
	// detail pane.
	Selection lipgloss.Style

	Dialog      lipgloss.Style
	DialogTitle lipgloss.Style
	Prompt      lipgloss.Style
}

// NewStyles compiles a resolved theme into styles.
//
// An unset colour deliberately produces a style with no foreground rather than
// a black one: that is how the "mono" theme renders using the terminal's own
// colours, and how a partial custom theme degrades gracefully.
func NewStyles(theme cnfg.Theme) Styles {
	c := theme.Colors

	accent := adaptive(c.Accent)
	accentAlt := adaptive(c.AccentAlt)
	text := adaptive(c.Text)
	muted := adaptive(c.Muted)
	subtle := adaptive(c.Subtle)
	success := adaptive(c.Success)
	warning := adaptive(c.Warning)
	failure := adaptive(c.Error)

	border := borderStyle(theme.Border)

	pane := lipgloss.NewStyle().Padding(0, 1)
	if theme.Border != "none" {
		pane = pane.Border(border)
	}

	s := Styles{
		Theme: theme,
		Icons: theme.IconSet(),

		App: lipgloss.NewStyle().Padding(0, 1),

		// The title is the one place a background is painted, so it needs its
		// own foreground to stay legible on the accent colour.
		Title:      bold(fg(bg(lipgloss.NewStyle(), c.Accent), c.TitleFg)).Padding(0, 1),
		HeaderMeta: setFg(lipgloss.NewStyle(), muted),

		Tab:       setFg(lipgloss.NewStyle().Padding(0, 2), muted),
		TabActive: setFg(lipgloss.NewStyle().Padding(0, 2).Bold(true).Underline(true), accentAlt),

		Pane:        setBorderFg(pane, subtle),
		PaneFocused: setBorderFg(pane, accent),

		DetailKey:   setFg(lipgloss.NewStyle().Width(detailKeyWidth), muted),
		DetailValue: setFg(lipgloss.NewStyle(), text),
		DetailTitle: setFg(lipgloss.NewStyle().Bold(true), accentAlt),

		BadgeInstalled: setFg(lipgloss.NewStyle().Bold(true), success),
		// NOT bold, and that is what separates it from an error. The generated themes give
		// `warning` and `error` the same red, because a palette that rules out magenta and yellow
		// has four hues left for five roles — so the two are told apart by weight instead. Bold
		// here made an available update look exactly like a failure.
		BadgeUpdate: setFg(lipgloss.NewStyle(), warning),

		Err:   setFg(lipgloss.NewStyle().Bold(true), failure),
		Warn:  setFg(lipgloss.NewStyle(), warning),
		OK:    setFg(lipgloss.NewStyle(), success),
		Muted: setFg(lipgloss.NewStyle(), muted),
		Text:  setFg(lipgloss.NewStyle(), text),
		Step:  setFg(lipgloss.NewStyle().Bold(true), accentAlt),

		Cursor: setFg(lipgloss.NewStyle(), accent),

		// Reverse video is the base: it needs no colour at all, so the cursor
		// and the selection stay visible on a monochrome theme. An accent
		// colour replaces it below, where one exists.
		Selection: lipgloss.NewStyle().Reverse(true),

		Dialog:      setBorderFg(lipgloss.NewStyle().Border(border).Padding(1, 3), accent),
		DialogTitle: setFg(lipgloss.NewStyle().Bold(true), accent),
		Prompt:      setFg(lipgloss.NewStyle().Bold(true), accentAlt),
	}

	if !c.Accent.IsZero() {
		s.Selection = bg(fg(lipgloss.NewStyle(), c.TitleFg), c.Accent)
	}

	// Without colour, emphasis has to come from weight: the "mono" theme relies
	// on this so installed and outdated packages remain distinguishable.
	if c.Success.IsZero() {
		s.BadgeInstalled = s.BadgeInstalled.Faint(true)
	}
	if c.Accent.IsZero() {
		s.Title = s.Title.Reverse(true)
		// Focus is normally shown by colouring the border. With no accent
		// colour there is nothing to colour it with, and lipgloss does not
		// apply Bold to borders, so the focused pane changes border weight
		// instead. Coloured themes keep their border shape unchanged.
		if theme.Border != "none" {
			s.PaneFocused = s.PaneFocused.Border(emphasisBorder(theme.Border))
		}
	}
	if c.Muted.IsZero() {
		s.Muted = s.Muted.Faint(true)
		s.HeaderMeta = s.HeaderMeta.Faint(true)
		s.Tab = s.Tab.Faint(true)
		s.DetailKey = s.DetailKey.Faint(true)
	}

	return s
}

// DefaultStyles is the fallback used before a configuration is available.
func DefaultStyles() Styles {
	return NewStyles(cnfg.DefaultTheme())
}

// adaptive turns a theme colour into a lipgloss colour, or nil when unset.
// lipgloss has no "no colour" value, so an unset entry is represented by a nil
// TerminalColor and simply not applied.
func adaptive(c cnfg.Color) lipgloss.TerminalColor {
	if c.IsZero() {
		return nil
	}
	return lipgloss.AdaptiveColor{Light: c.Light, Dark: c.Dark}
}

// setFg applies a foreground only when the colour is set.
func setFg(s lipgloss.Style, color lipgloss.TerminalColor) lipgloss.Style {
	if color == nil {
		return s
	}
	return s.Foreground(color)
}

// setBorderFg applies a border colour only when it is set.
func setBorderFg(s lipgloss.Style, color lipgloss.TerminalColor) lipgloss.Style {
	if color == nil {
		return s
	}
	return s.BorderForeground(color)
}

func fg(s lipgloss.Style, c cnfg.Color) lipgloss.Style {
	return setFg(s, adaptive(c))
}

func bg(s lipgloss.Style, c cnfg.Color) lipgloss.Style {
	if c.IsZero() {
		return s
	}
	return s.Background(lipgloss.AdaptiveColor{Light: c.Light, Dark: c.Dark})
}

func bold(s lipgloss.Style) lipgloss.Style {
	return s.Bold(true)
}

// emphasisBorder returns a visibly heavier counterpart of a border, used to
// mark the focused pane when there is no accent colour to do it with.
func emphasisBorder(name string) lipgloss.Border {
	switch name {
	case "thick", "double":
		return lipgloss.DoubleBorder()
	case "hidden", "none":
		return lipgloss.NormalBorder()
	default:
		// rounded and normal are both light box-drawing, so thick reads as a
		// clear step up from either.
		return lipgloss.ThickBorder()
	}
}

// borderStyle maps a theme border name to a lipgloss border. "none" still needs
// a value here because the caller decides whether to apply it at all.
func borderStyle(name string) lipgloss.Border {
	switch name {
	case "normal":
		return lipgloss.NormalBorder()
	case "thick":
		return lipgloss.ThickBorder()
	case "double":
		return lipgloss.DoubleBorder()
	case "hidden":
		return lipgloss.HiddenBorder()
	case "none":
		return lipgloss.HiddenBorder()
	default:
		return lipgloss.RoundedBorder()
	}
}
