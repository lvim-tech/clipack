package tui

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lvim-tech/clipack/pkg"
)

// packageItem adapts a registry package to the bubbles list, carrying the
// installed counterpart so the delegate can render status without a lookup.
type packageItem struct {
	pkg       *pkg.Package
	installed *pkg.Package
	// broken means the manifest says installed but the binaries or resource
	// trees it names are not there.
	broken bool
}

// hasUpdate reports whether an installed package is behind the registry.
func (i packageItem) hasUpdate() bool {
	return pkg.HasUpdate(i.pkg, i.installed)
}

// requirements is what this package needs for the method it will be built with:
// the one it is installed under when it is on disk, so an update shows what its
// own rebuild needs rather than what the other ref would.
func (i packageItem) requirements(method string) pkg.MethodRequirements {
	if i.installed != nil && i.installed.InstallMethod != "" {
		method = i.installed.InstallMethod
	}
	return i.pkg.Requirements.For(method)
}

// FilterValue implements list.Item. Tags and category are included so "/" can
// find a package by what it does, not only by its name.
func (i packageItem) FilterValue() string {
	return strings.Join([]string{
		i.pkg.Name,
		i.pkg.Description,
		i.pkg.Category,
		strings.Join(i.pkg.Tags, " "),
	}, " ")
}

// Checkbox glyphs. Deliberately plain ASCII rather than themed: they carry
// state the user is about to act on, so they have to be unmistakable on any
// terminal and in any icon set.
const (
	checkOn  = "[x] "
	checkOff = "[ ] "
)

// packageDelegate satisfies list.ItemDelegate so a bubbles list can be built.
//
// clipack does not use it to draw: bubbles asks a delegate for ONE height for
// the whole list, which forces every entry to be as tall as the longest
// description. Rendering is done by Model.renderList instead, where each entry
// takes exactly the lines it needs. The list is kept for what it is good at —
// holding the items, running the filter and tracking the cursor.
type packageDelegate struct{}

// Height implements list.ItemDelegate. One line, so bubbles' own pagination
// puts every item on a single page and Index() moves linearly.
func (d packageDelegate) Height() int { return 1 }

// Spacing implements list.ItemDelegate.
func (d packageDelegate) Spacing() int { return 0 }

// Update implements list.ItemDelegate. The delegate is stateless.
func (d packageDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }

// Render implements list.ItemDelegate. Unused; see the type comment.
func (d packageDelegate) Render(w io.Writer, m list.Model, index int, item list.Item) {
	if entry, ok := item.(packageItem); ok {
		fmt.Fprint(w, entry.pkg.Name)
	}
}

// wrapDescription splits a description into the lines it needs at width.
func wrapDescription(text string, width int) []string {
	if width < 8 {
		width = 8
	}
	if strings.TrimSpace(text) == "" {
		return nil
	}
	return strings.Split(lipgloss.NewStyle().Width(width).Render(text), "\n")
}

// entryState is how one entry should be drawn.
type entryState struct {
	// cursor marks the entry the keyboard is on.
	cursor bool
	// checked marks it as picked for a batch operation.
	checked bool
	// showCheck draws the checkbox at all. It is off in the All tab, which is
	// for browsing: a checkbox there would have no single action behind it,
	// because the tab mixes installed and uninstalled packages.
	showCheck bool
}

// renderEntry draws one package: the name line plus however many lines its
// description wraps onto. No padding — the caller adds the single blank line
// that separates entries, so the gap is the same whatever the description
// length.
//
// The description is indented to line up under the name, so it follows the
// width of the cursor and the checkbox rather than a fixed constant.
func renderEntry(entry packageItem, width int, st entryState, s Styles) []string {
	if width < 12 {
		width = 12
	}

	marker := strings.Repeat(" ", len([]rune(s.Icons.Cursor)))
	nameStyle := s.Text
	if st.cursor {
		marker = s.Cursor.Render(s.Icons.Cursor)
		nameStyle = s.Cursor.Bold(true)
	}

	box := ""
	if st.showCheck {
		box = s.Muted.Render(checkOff)
		if st.checked {
			box = s.BadgeInstalled.Render(checkOn)
		}
	}

	badge := ""
	switch {
	// Broken outranks the update badge: offering an update for something that
	// is not on disk is the misleading half of the same fact.
	case entry.broken:
		badge = " " + s.Err.Render(s.Icons.Error+" broken")
	case entry.hasUpdate():
		badge = " " + s.BadgeUpdate.Render(s.Icons.Update)
	case entry.installed != nil:
		badge = " " + s.BadgeInstalled.Render(s.Icons.Installed)
	}

	title := marker + box + nameStyle.Render(entry.pkg.Name) +
		s.Muted.Render(" "+entry.pkg.Version) + badge

	// MaxWidth clips without wrapping and is ANSI-aware, so a long name or an
	// unusually wide badge cannot silently add a line the caller did not count.
	clip := lipgloss.NewStyle().MaxWidth(width)
	out := []string{clip.Render(title)}

	indent := entryIndent(st.showCheck, s)
	for _, line := range wrapDescription(entry.pkg.Description, width-indent) {
		out = append(out, clip.Render(strings.Repeat(" ", indent)+
			s.Muted.Render(strings.TrimRight(line, " "))))
	}

	return out
}

// entryIndent is the column the description starts at.
func entryIndent(showCheck bool, s Styles) int {
	indent := len([]rune(s.Icons.Cursor))
	if showCheck {
		indent += len([]rune(checkOff))
	}
	return indent
}

// entryHeight is how many screen rows an entry occupies, separator included.
// The checkbox shifts the description right, which can change how it wraps, so
// it has to be accounted for here too.
func entryHeight(item list.Item, width int, showCheck bool, s Styles) int {
	entry, ok := item.(packageItem)
	if !ok {
		return 1
	}
	return len(renderEntry(entry, width, entryState{showCheck: showCheck}, s)) + 1
}

// listFilterHeight is how many rows the filter line takes when it is shown:
// the prompt plus a blank line under it.
const listFilterHeight = 2

// renderList draws the package list.
//
// Entries have individual heights — one line for the name plus however many the
// description wraps onto — and each is followed by exactly one blank line. That
// is why this exists instead of bubbles' own View: a delegate declares a single
// height for every row, so a long description either truncates or makes every
// other entry equally tall.
func (m Model) renderList() string {
	s := m.styles
	width := m.list.Width()

	var out []string
	switch {
	case m.list.SettingFilter():
		out = append(out, m.list.FilterInput.View(), "")
	case m.list.FilterState() == list.FilterApplied:
		out = append(out, s.Prompt.Render("filter: ")+m.list.FilterValue(), "")
	}

	items := m.list.VisibleItems()
	if len(items) == 0 {
		return strings.Join(append(out, s.Muted.Render("  No packages.")), "\n")
	}

	height := m.listContentHeight()
	cursor := m.list.Index()
	showCheck := m.showChecks()

	used := 0
	for i := m.listOffset; i < len(items); i++ {
		entry, ok := items[i].(packageItem)
		if !ok {
			continue
		}

		lines := renderEntry(entry, width, entryState{
			cursor:    i == cursor,
			checked:   m.checked[entry.pkg.Name],
			showCheck: showCheck,
		}, s)
		if used+len(lines)+1 > height && used > 0 {
			break
		}

		out = append(out, lines...)
		out = append(out, "")
		used += len(lines) + 1
	}

	// Pad to the full height so the scroll indicator stays pinned to the
	// bottom of the pane rather than floating under the last entry.
	for used < height {
		out = append(out, "")
		used++
	}

	return strings.Join(append(out, m.scrollIndicator(items, width)), "\n")
}

// listContentHeight is the room entries have, once the filter line and the
// scroll indicator have taken theirs.
func (m Model) listContentHeight() int {
	height := m.listHeight - 1 // the scroll indicator
	if m.list.SettingFilter() || m.list.FilterState() == list.FilterApplied {
		height -= listFilterHeight
	}
	if height < 1 {
		return 1
	}
	return height
}

// scrollIndicator draws one dot per screenful, with the current one filled.
func (m Model) scrollIndicator(items []list.Item, width int) string {
	s := m.styles
	height := m.listContentHeight()

	// Position is measured from the CURSOR, not the scroll offset: the dots
	// answer "where am I in the list", and the offset lags a screen behind the
	// selection whenever the visible window has room to spare.
	total, before := 0, 0
	cursor := m.list.Index()
	for i, item := range items {
		h := entryHeight(item, width, m.showChecks(), s)
		if i < cursor {
			before += h
		}
		total += h
	}

	screens := (total + height - 1) / height
	if screens < 1 {
		screens = 1
	}
	current := before / height
	if current >= screens {
		current = screens - 1
	}

	var b strings.Builder
	b.WriteString("  ")
	for i := 0; i < screens; i++ {
		if i == current {
			b.WriteString(s.Cursor.Render(s.Icons.PageActive))
		} else {
			b.WriteString(s.Muted.Render(s.Icons.PageInactive))
		}
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(b.String())
}

// substringFilter ranks items by a plain case-insensitive substring match.
//
// The default fuzzy filter scores characters scattered anywhere in the target.
// Because a package's filter value contains its description and tags, typing
// "duf" fuzzy-matched almost every entry ("lazydocker ... UI for git"), which
// made the filter useless. Substring matching keeps searching by description
// and tag working while only returning entries that genuinely contain the term.
// Fuzzy matching is kept as a fallback so a typo still finds something.
func substringFilter(term string, targets []string) []list.Rank {
	term = strings.ToLower(strings.TrimSpace(term))
	if term == "" {
		ranks := make([]list.Rank, len(targets))
		for i := range targets {
			ranks[i] = list.Rank{Index: i}
		}
		return ranks
	}

	var ranks []list.Rank
	for i, target := range targets {
		idx := strings.Index(strings.ToLower(target), term)
		if idx < 0 {
			continue
		}

		matched := make([]int, len(term))
		for j := range matched {
			matched[j] = idx + j
		}
		ranks = append(ranks, list.Rank{Index: i, MatchedIndexes: matched})
	}

	if len(ranks) == 0 {
		return list.DefaultFilter(term, targets)
	}

	// A filter value starts with the package name, so the earlier the match
	// begins the more relevant it is: name first, then description, then tags.
	sort.SliceStable(ranks, func(a, b int) bool {
		return ranks[a].MatchedIndexes[0] < ranks[b].MatchedIndexes[0]
	})

	return ranks
}

// newPackageList builds the list model with clipack's chrome disabled; the
// root model draws its own header, help and status line.
func newPackageList(s Styles) list.Model {
	l := list.New(nil, packageDelegate{}, 0, 0)
	l.SetShowTitle(false)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(true)
	l.InfiniteScrolling = false
	l.Filter = substringFilter
	l.Styles.FilterPrompt = s.Prompt
	l.Styles.FilterCursor = s.Prompt
	l.Styles.NoItems = s.Muted.Padding(1, 2)

	// bubbles binds h/l and ←/→ to paging. clipack needs those for moving
	// between the list and the detail pane, and the list already pages itself
	// as the cursor moves, so paging keeps only the explicit page keys.
	l.KeyMap.PrevPage = key.NewBinding(
		key.WithKeys("pgup"),
		key.WithHelp("pgup", "prev page"),
	)
	l.KeyMap.NextPage = key.NewBinding(
		key.WithKeys("pgdown"),
		key.WithHelp("pgdn", "next page"),
	)

	// bubbles defaults both pagination dots to a bullet in a subdued grey,
	// which is almost invisible under the list. Filled and hollow circles in
	// the accent colour make the current page readable at a glance.
	l.Paginator.ActiveDot = s.Cursor.Render(s.Icons.PageActive)
	l.Paginator.InactiveDot = s.Muted.Render(s.Icons.PageInactive)

	return l
}

// buildItems turns the registry and installed sets into list items for a tab.
func buildItems(packages []*pkg.Package, installed map[string]*pkg.Package, broken map[string]bool, tab tabID) []list.Item {
	items := make([]list.Item, 0, len(packages))
	for _, p := range packages {
		entry := packageItem{pkg: p, installed: installed[p.Name], broken: broken[p.Name]}
		switch tab {
		case tabInstalled:
			if entry.installed == nil {
				continue
			}
		case tabNotInstalled:
			if entry.installed != nil {
				continue
			}
		case tabUpdates:
			if !entry.hasUpdate() {
				continue
			}
		}
		items = append(items, entry)
	}
	return items
}
