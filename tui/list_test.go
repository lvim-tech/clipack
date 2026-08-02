package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lvim-tech/clipack/cnfg"
	"github.com/lvim-tech/clipack/pkg"
)

func TestSubstringFilterOnlyReturnsRealMatches(t *testing.T) {
	// These are the filter values of real registry entries. Fuzzy matching
	// scored "duf" against "lazydocker … UI for git" (d…u…f), which made the
	// filter return almost the whole registry.
	targets := []string{
		"duf Disk Usage/Free Utility cli disk",
		"lazydocker A simple terminal UI for git commands cli git",
		"bat A cat clone with syntax highlighting cli cat",
	}

	ranks := substringFilter("duf", targets)
	if len(ranks) != 1 {
		t.Fatalf("got %d matches for %q, want only the real one: %v", len(ranks), "duf", ranks)
	}
	if ranks[0].Index != 0 {
		t.Errorf("matched index %d, want 0 (duf)", ranks[0].Index)
	}
}

func TestSubstringFilterIsCaseInsensitive(t *testing.T) {
	targets := []string{"Bat A cat clone"}

	for _, term := range []string{"bat", "BAT", "Bat", "  bat  "} {
		if ranks := substringFilter(term, targets); len(ranks) != 1 {
			t.Errorf("substringFilter(%q) returned %d matches, want 1", term, len(ranks))
		}
	}
}

func TestSubstringFilterRanksNameMatchesFirst(t *testing.T) {
	targets := []string{
		"lazygit A terminal UI for git",     // "git" appears late, in the description
		"git-delta A viewer for git output", // "git" appears at position 0
	}

	ranks := substringFilter("git", targets)
	if len(ranks) != 2 {
		t.Fatalf("got %d matches, want 2", len(ranks))
	}
	// A filter value starts with the package name, so the earliest match is
	// the most relevant.
	if ranks[0].Index != 1 {
		t.Errorf("first result is index %d, want 1 (the name match)", ranks[0].Index)
	}
}

func TestSubstringFilterEmptyTermMatchesEverything(t *testing.T) {
	targets := []string{"a", "b", "c"}

	for _, term := range []string{"", "   "} {
		ranks := substringFilter(term, targets)
		if len(ranks) != len(targets) {
			t.Errorf("substringFilter(%q) returned %d matches, want all %d", term, len(ranks), len(targets))
		}
	}
}

func TestSubstringFilterFallsBackToFuzzy(t *testing.T) {
	targets := []string{"lazygit A terminal UI for git"}

	// No substring match, so a typo still finds something rather than showing
	// an empty list.
	if ranks := substringFilter("lzygit", targets); len(ranks) == 0 {
		t.Error("substringFilter() returned nothing for a typo, want the fuzzy fallback")
	}
}

func TestSubstringFilterMatchedIndexes(t *testing.T) {
	ranks := substringFilter("cat", []string{"bat A cat clone"})
	if len(ranks) != 1 {
		t.Fatalf("got %d matches, want 1", len(ranks))
	}

	// The indexes drive the highlight in the list, so they must point at the
	// matched run.
	want := []int{6, 7, 8}
	if len(ranks[0].MatchedIndexes) != len(want) {
		t.Fatalf("MatchedIndexes = %v, want %v", ranks[0].MatchedIndexes, want)
	}
	for i, idx := range want {
		if ranks[0].MatchedIndexes[i] != idx {
			t.Errorf("MatchedIndexes = %v, want %v", ranks[0].MatchedIndexes, want)
			break
		}
	}
}

func TestPackageItemFilterValue(t *testing.T) {
	entry := packageItem{pkg: &pkg.Package{
		Name:        "bat",
		Description: "A cat clone",
		Category:    "cli",
		Tags:        []string{"syntax-highlighting", "git"},
	}}

	value := entry.FilterValue()
	// Searching by tag and category has to work, not just by name.
	for _, want := range []string{"bat", "A cat clone", "cli", "syntax-highlighting", "git"} {
		if !strings.Contains(value, want) {
			t.Errorf("FilterValue() = %q, want it to contain %q", value, want)
		}
	}
	// The name comes first so name matches rank highest.
	if !strings.HasPrefix(value, "bat") {
		t.Errorf("FilterValue() = %q, want it to start with the name", value)
	}
}

func TestPackageItemHasUpdate(t *testing.T) {
	registry := &pkg.Package{Name: "bat", Version: "v2.0.0", Commit: "new"}

	tests := []struct {
		name      string
		installed *pkg.Package
		want      bool
	}{
		{"not installed", nil, false},
		{"up to date", &pkg.Package{Version: "v2.0.0", InstallMethod: pkg.MethodVersion}, false},
		{"behind", &pkg.Package{Version: "v1.0.0", InstallMethod: pkg.MethodVersion}, true},
		{"commit method behind", &pkg.Package{Version: "v2.0.0", Commit: "old", InstallMethod: pkg.MethodCommit}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := packageItem{pkg: registry, installed: tt.installed}
			if got := entry.hasUpdate(); got != tt.want {
				t.Errorf("hasUpdate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBuildItemsPerTab(t *testing.T) {
	packages, installed := samplePackages()

	tests := []struct {
		tab   tabID
		want  []string
		label string
	}{
		{tab: tabAll, want: []string{"bat", "fzf", "yazi"}, label: "All"},
		{tab: tabInstalled, want: []string{"fzf", "yazi"}, label: "Installed"},
		{tab: tabNotInstalled, want: []string{"bat"}, label: "Not installed"},
		{tab: tabUpdates, want: []string{"yazi"}, label: "Updates"},
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			items := buildItems(packages, installed, nil, tt.tab, "")
			if len(items) != len(tt.want) {
				t.Fatalf("got %d items, want %d %v", len(items), len(tt.want), tt.want)
			}
			for i, item := range items {
				entry := item.(packageItem)
				if entry.pkg.Name != tt.want[i] {
					t.Errorf("item %d = %q, want %q", i, entry.pkg.Name, tt.want[i])
				}
			}
		})
	}
}

func TestBuildItemsAttachesInstalledRecord(t *testing.T) {
	packages, installed := samplePackages()

	items := buildItems(packages, installed, nil, tabAll, "")
	for _, item := range items {
		entry := item.(packageItem)
		want := installed[entry.pkg.Name]
		if entry.installed != want {
			t.Errorf("%s: installed record = %v, want %v", entry.pkg.Name, entry.installed, want)
		}
	}
}

func TestBuildItemsEmptyRegistry(t *testing.T) {
	if items := buildItems(nil, nil, nil, tabAll, ""); len(items) != 0 {
		t.Errorf("got %d items for an empty registry, want 0", len(items))
	}
}

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------

// listModel returns a browse model whose list holds exactly these packages.
func listModel(t *testing.T, width, height int, packages ...*pkg.Package) Model {
	t.Helper()

	m := browseModel(t)
	m = applyMsg(t, m, tea.WindowSizeMsg{Width: width, Height: height})

	items := make([]list.Item, len(packages))
	for i, p := range packages {
		items[i] = packageItem{pkg: p}
	}
	m.list.SetItems(items)
	m.list.Select(0)
	m.ensureListVisible()
	return m
}

func TestRenderEntryHeightFollowsTheDescription(t *testing.T) {
	s := DefaultStyles()

	short := packageItem{pkg: &pkg.Package{Name: "duf", Version: "v1", Description: "Disk usage"}}
	long := packageItem{pkg: &pkg.Package{
		Name:        "gowall",
		Version:     "v1",
		Description: "Gowall is a tool to convert an image ( specifically a wallpaper ) to any color-scheme you like",
	}}

	// The whole point: entries are not all the same height.
	if got := len(renderEntry(short, 44, entryState{}, s)); got != 2 {
		t.Errorf("short entry rendered %d lines, want a name and one description line", got)
	}
	if got := len(renderEntry(long, 44, entryState{}, s)); got < 3 {
		t.Errorf("long entry rendered %d lines, want the description wrapped onto several", got)
	}
	// A package with no description is just its name.
	bare := packageItem{pkg: &pkg.Package{Name: "bare", Version: "v1"}}
	if got := len(renderEntry(bare, 44, entryState{}, s)); got != 1 {
		t.Errorf("entry with no description rendered %d lines, want 1", got)
	}
}

func TestRenderEntryNeverExceedsTheWidth(t *testing.T) {
	s := DefaultStyles()
	entry := packageItem{pkg: &pkg.Package{
		Name:        "a-very-long-package-name-that-will-not-fit",
		Version:     "v1.2.3-with-a-long-suffix",
		Description: strings.Repeat("word ", 40),
	}}

	const width = 30
	for _, line := range renderEntry(entry, width, entryState{cursor: true}, s) {
		if got := len([]rune(line)); got > width {
			t.Errorf("line is %d cells wide, want at most %d: %q", got, width, line)
		}
	}
}

func TestEntryHeightCountsTheSeparator(t *testing.T) {
	s := DefaultStyles()
	entry := packageItem{pkg: &pkg.Package{Name: "duf", Version: "v1", Description: "Disk usage"}}

	if got := entryHeight(entry, 44, false, s); got != len(renderEntry(entry, 44, entryState{}, s))+1 {
		t.Errorf("entryHeight = %d, want the rendered lines plus one separator", got)
	}
}

func TestEveryEntryIsFollowedByExactlyOneBlankLine(t *testing.T) {
	short := &pkg.Package{Name: "aaa", Version: "v1", Description: "Short one"}
	long := &pkg.Package{Name: "bbb", Version: "v1", Description: strings.Repeat("long description ", 4)}
	another := &pkg.Package{Name: "ccc", Version: "v1", Description: "Short again"}

	m := listModel(t, 120, 40, short, long, another)
	lines := strings.Split(m.renderList(), "\n")

	// Whatever the description length, the gap between entries is one line —
	// that is what a fixed row height could not give.
	for _, name := range []string{"bbb", "ccc"} {
		start := -1
		for i, line := range lines {
			if strings.Contains(line, name+" ") {
				start = i
				break
			}
		}
		if start < 1 {
			t.Fatalf("entry %q not found in:\n%s", name, m.renderList())
		}
		if got := strings.TrimSpace(lines[start-1]); got != "" {
			t.Errorf("no blank line before %q, found %q", name, got)
		}
		if start >= 2 {
			if got := strings.TrimSpace(lines[start-2]); got == "" {
				t.Errorf("two blank lines before %q; the gap should be exactly one", name)
			}
		}
	}
}

func TestLongDescriptionIsShownInFull(t *testing.T) {
	long := &pkg.Package{
		Name:        "gowall",
		Version:     "v1",
		Description: "Gowall is a tool to convert an image ( specifically a wallpaper ) to any color-scheme you like!",
	}

	m := listModel(t, 120, 40, long)
	view := m.renderList()

	// Per-entry heights mean nothing has to be cut, so there is no ellipsis.
	if strings.Contains(view, "…") {
		t.Errorf("the description was truncated:\n%s", view)
	}
	// The tail of the description survives; it just lands on a later line.
	if !strings.Contains(view, "you like!") {
		t.Errorf("the end of the description is missing:\n%s", view)
	}
}

func TestRenderListStaysInsideThePane(t *testing.T) {
	packages, _ := samplePackages()
	for _, size := range []struct{ w, h int }{{120, 40}, {80, 24}, {60, 16}} {
		m := listModel(t, size.w, size.h, packages...)

		lines := strings.Split(m.renderList(), "\n")
		if len(lines) > m.listHeight {
			t.Errorf("%dx%d: list rendered %d lines into a pane of %d",
				size.w, size.h, len(lines), m.listHeight)
		}
		for _, line := range lines {
			if got := len([]rune(line)); got > m.list.Width() {
				t.Errorf("%dx%d: line is %d cells wide, want at most %d: %q",
					size.w, size.h, got, m.list.Width(), line)
			}
		}
	}
}

func TestListScrollsToKeepTheSelectionVisible(t *testing.T) {
	var packages []*pkg.Package
	for i := 0; i < 20; i++ {
		packages = append(packages, &pkg.Package{
			Name:        fmt.Sprintf("pkg%02d", i),
			Version:     "v1",
			Description: "A package description long enough to wrap onto a second line in a narrow pane",
		})
	}

	m := listModel(t, 100, 20, packages...)
	if m.listOffset != 0 {
		t.Fatalf("listOffset = %d at the top, want 0", m.listOffset)
	}

	for i := 0; i < 15; i++ {
		m = applyMsg(t, m, keyMsg("j"))
	}

	if m.listOffset == 0 {
		t.Error("the list did not scroll while the selection moved down")
	}
	// The selected entry has to be on screen, and its cursor marker with it.
	if !strings.Contains(m.renderList(), "pkg15") {
		t.Errorf("the selected entry is not visible:\n%s", m.renderList())
	}

	for i := 0; i < 15; i++ {
		m = applyMsg(t, m, keyMsg("k"))
	}
	if m.listOffset != 0 {
		t.Errorf("listOffset = %d back at the top, want 0", m.listOffset)
	}
}

func TestRenderListShowsStatusBadges(t *testing.T) {
	m := browseModel(t)
	view := m.renderList()

	if !strings.Contains(view, "installed") {
		t.Errorf("the list does not mark installed packages:\n%s", view)
	}
	if !strings.Contains(view, "update") {
		t.Errorf("the list does not mark packages with updates:\n%s", view)
	}
}

func TestRenderListShowsTheFilter(t *testing.T) {
	m := browseModel(t)
	m = applyMsg(t, m, keyMsg("/"))
	for _, r := range "yazi" {
		m = applyMsg(t, m, keyMsg(string(r)))
	}

	if !strings.Contains(m.renderList(), "yazi") {
		t.Errorf("the filter being typed is not shown:\n%s", m.renderList())
	}
}

func TestRenderListWithNoMatches(t *testing.T) {
	m := listModel(t, 100, 20)
	if !strings.Contains(m.renderList(), "No packages") {
		t.Errorf("an empty list says nothing:\n%s", m.renderList())
	}
}

func TestScrollIndicatorTracksTheCursor(t *testing.T) {
	var packages []*pkg.Package
	for i := 0; i < 30; i++ {
		packages = append(packages, &pkg.Package{
			Name: fmt.Sprintf("pkg%02d", i), Version: "v1", Description: "Description",
		})
	}

	m := listModel(t, 100, 20, packages...)
	top := m.scrollIndicator(m.list.VisibleItems(), m.list.Width())

	for i := 0; i < 20; i++ {
		m = applyMsg(t, m, keyMsg("j"))
	}
	moved := m.scrollIndicator(m.list.VisibleItems(), m.list.Width())

	if top == moved {
		t.Errorf("the indicator did not move with the cursor: %q", top)
	}
	if !strings.Contains(top, "●") || !strings.Contains(top, "○") {
		t.Errorf("indicator = %q, want a filled and a hollow dot", top)
	}
}

func TestScrollIndicatorFollowsTheIconSet(t *testing.T) {
	theme, err := cnfg.ResolveTheme(cnfg.Theme{Name: "default", Icons: cnfg.IconsASCII})
	if err != nil {
		t.Fatal(err)
	}

	m := browseModel(t)
	m.styles = NewStyles(theme)

	if got := m.scrollIndicator(m.list.VisibleItems(), m.list.Width()); strings.Contains(got, "●") {
		t.Errorf("indicator = %q, want the ascii glyphs", got)
	}
}
