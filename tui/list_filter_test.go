package tui

import "testing"

// The filter prompt and cursor must come from the theme, not from bubbles' own
// defaults: the library applies Styles.Filter* only inside list.New(), so an
// assignment made afterwards never reaches the input that already exists.
func TestFilterInputTakesTheThemeColours(t *testing.T) {
	s := DefaultStyles()
	l := newPackageList(s)

	want := s.Prompt.GetForeground()
	if got := l.FilterInput.PromptStyle.GetForeground(); got != want {
		t.Errorf("filter prompt fg = %v, want the theme's %v", got, want)
	}
	if got := l.FilterInput.Cursor.Style.GetForeground(); got != want {
		t.Errorf("filter cursor fg = %v, want the theme's %v", got, want)
	}
}
