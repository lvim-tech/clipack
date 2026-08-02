package tui

import (
	"strings"
	"testing"
)

// The sample registry has two categories: cli (bat, fzf) and file_managers
// (yazi), so one full cycle is all -> cli -> file_managers -> all.

func visibleNames(m Model) []string {
	var names []string
	for _, item := range m.list.Items() {
		names = append(names, item.(packageItem).pkg.Name)
	}
	return names
}

func TestCategoryCyclesForwardAndFiltersTheList(t *testing.T) {
	m := browseModel(t)

	m = applyMsg(t, m, keyMsg("c"))
	if m.category != "cli" {
		t.Fatalf("category = %q after c, want cli", m.category)
	}
	if got := visibleNames(m); len(got) != 2 || got[0] != "bat" || got[1] != "fzf" {
		t.Errorf("visible = %v, want only the cli packages", got)
	}

	m = applyMsg(t, m, keyMsg("c"))
	if m.category != "file_managers" {
		t.Fatalf("category = %q, want file_managers", m.category)
	}
	if got := visibleNames(m); len(got) != 1 || got[0] != "yazi" {
		t.Errorf("visible = %v, want only yazi", got)
	}

	// The cycle wraps back to everything rather than sticking at the end.
	m = applyMsg(t, m, keyMsg("c"))
	if m.category != "" {
		t.Fatalf("category = %q after the full cycle, want all", m.category)
	}
	if got := visibleNames(m); len(got) != 3 {
		t.Errorf("visible = %v, want the whole registry again", got)
	}
}

func TestCategoryCyclesBackward(t *testing.T) {
	m := browseModel(t)

	// One step back from "all" is the LAST category, not the first.
	m = applyMsg(t, m, keyMsg("C"))
	if m.category != "file_managers" {
		t.Errorf("category = %q after C from all, want file_managers", m.category)
	}
}

// TestCategoryChangeClearsTheMarks is the same rule the tabs follow: a mark on
// a package that just left the screen is a mark the user has forgotten about,
// and the next batch action would act on it invisibly.
func TestCategoryChangeClearsTheMarks(t *testing.T) {
	m := browseModel(t)
	m = applyMsg(t, m, keyMsg("tab")) // Installed — има отметки
	m = applyMsg(t, m, keyMsg(" "))
	if m.checkedCount() != 1 {
		t.Fatalf("checkedCount = %d, want 1 before the category change", m.checkedCount())
	}

	m = applyMsg(t, m, keyMsg("c"))
	if m.checkedCount() != 0 {
		t.Errorf("checkedCount = %d after a category change, want the marks cleared", m.checkedCount())
	}
}

func TestHeaderFollowsTheCategory(t *testing.T) {
	m := browseModel(t)
	m = applyMsg(t, m, keyMsg("c")) // cli: bat + fzf, единият инсталиран

	header := m.header()
	if !strings.Contains(header, "category:") || !strings.Contains(header, "cli") {
		t.Errorf("header = %q, want the active category shown", header)
	}
	// Counts describe what the list shows: 2 cli packages, 1 of them installed.
	if !strings.Contains(header, "2 packages") || !strings.Contains(header, "1 installed") {
		t.Errorf("header = %q, want the counts scoped to the category", header)
	}

	// The collapsed help doubles as the cycle indicator.
	if !strings.Contains(helpFor(m), "category: cli") {
		t.Errorf("help = %q, want it to name the active category", helpFor(m))
	}
}
