package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// The help line lists only the keys that apply right now, so it is longer on
// the tabs that allow marking — long enough to wrap. When it wraps, the body
// has to give up a line; otherwise the view is taller than the terminal and the
// terminal scrolls, taking the title bar off the top.
func TestViewNeverOutgrowsTheTerminal(t *testing.T) {
	for _, size := range []struct{ w, h int }{
		{120, 40}, {100, 30}, {80, 24}, {160, 50},
	} {
		var model tea.Model = browseModel(t)
		model, _ = model.Update(tea.WindowSizeMsg{Width: size.w, Height: size.h})

		// Walk every tab the way a user does, with the key.
		for i := 0; i <= len(tabTitles); i++ {
			m := model.(Model)
			out := m.styles.App.Render(m.viewBrowse())
			if h := lipgloss.Height(out); h > size.h {
				t.Errorf("%dx%d, tab %s: rendered %d lines into %d",
					size.w, size.h, tabTitles[m.tab], h, size.h)
			}
			model, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab})
		}
	}
}

// Marking entries adds keys to the help too, by the same route.
func TestViewFitsAfterMarking(t *testing.T) {
	var model tea.Model = browseModel(t)
	model, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyTab}) // leave All, where marks are off

	for _, key := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{' '}},
		{Type: tea.KeyRunes, Runes: []rune{'a'}},
	} {
		model, _ = model.Update(key)
		m := model.(Model)
		if h := lipgloss.Height(m.styles.App.Render(m.viewBrowse())); h > 30 {
			t.Errorf("after %v: rendered %d lines into 30", key, h)
		}
	}
}
