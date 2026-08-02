package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lvim-tech/clipack/pkg"
)

// atTab returns a browse model sitting on the given tab.
func atTab(t *testing.T, tab tabID) Model {
	t.Helper()

	m := browseModel(t)
	m.tab = tab
	m.applyTab()
	return m
}

func TestCheckboxesOnlyInTheActionTabs(t *testing.T) {
	// The three filtered tabs each stand for one action, so a mark there is
	// unambiguous. All mixes installed and uninstalled packages, which leaves a
	// mark with nothing to mean.
	tests := map[tabID]bool{
		tabAll:          false,
		tabInstalled:    true,
		tabNotInstalled: true,
		tabUpdates:      true,
	}

	for tab, want := range tests {
		m := atTab(t, tab)
		if got := m.showChecks(); got != want {
			t.Errorf("tab %v: showChecks() = %v, want %v", tab, got, want)
		}

		view := m.renderList()
		if hasBox := strings.Contains(view, checkOff) || strings.Contains(view, checkOn); hasBox != want {
			t.Errorf("tab %v: checkbox drawn = %v, want %v:\n%s", tab, hasBox, want, view)
		}
	}
}

func TestTabAction(t *testing.T) {
	tests := map[tabID]action{
		tabNotInstalled: actionInstall,
		tabUpdates:      actionUpdate,
		tabInstalled:    actionRemove,
		tabAll:          actionNone,
	}
	for tab, want := range tests {
		if got := tabAction(tab); got != want {
			t.Errorf("tabAction(%v) = %v, want %v", tab, got, want)
		}
	}
}

func TestMarkingWithSpaceAndAll(t *testing.T) {
	m := atTab(t, tabInstalled)
	if m.checkedCount() != 0 {
		t.Fatalf("started with %d marks", m.checkedCount())
	}

	m = applyMsg(t, m, keyMsg(" "))
	if m.checkedCount() != 1 {
		t.Errorf("space marked %d, want 1", m.checkedCount())
	}

	// The same key unmarks.
	m = applyMsg(t, m, keyMsg(" "))
	if m.checkedCount() != 0 {
		t.Errorf("a second space left %d marks, want 0", m.checkedCount())
	}

	m = applyMsg(t, m, keyMsg("a"))
	if got, want := m.checkedCount(), len(m.list.VisibleItems()); got != want {
		t.Errorf("a marked %d, want all %d", got, want)
	}

	// a again clears, so it is a toggle rather than a one-way door.
	m = applyMsg(t, m, keyMsg("a"))
	if m.checkedCount() != 0 {
		t.Errorf("a second a left %d marks, want 0", m.checkedCount())
	}
}

func TestMarkingIsIgnoredInTheAllTab(t *testing.T) {
	m := atTab(t, tabAll)

	m = applyMsg(t, m, keyMsg(" "))
	m = applyMsg(t, m, keyMsg("a"))

	if m.checkedCount() != 0 {
		t.Errorf("the All tab accepted %d marks, want none", m.checkedCount())
	}
}

func TestMarksClearOnTabChange(t *testing.T) {
	m := atTab(t, tabInstalled)
	m = applyMsg(t, m, keyMsg("a"))
	if m.checkedCount() == 0 {
		t.Fatal("nothing marked to begin with")
	}

	// A mark you can no longer see is one you have forgotten about.
	m = applyMsg(t, m, keyMsg("tab"))
	if m.checkedCount() != 0 {
		t.Errorf("%d marks survived a tab change, want none", m.checkedCount())
	}
}

func TestMarksClearWhenTheFilterChanges(t *testing.T) {
	m := atTab(t, tabInstalled)
	m = applyMsg(t, m, keyMsg("a"))
	if m.checkedCount() == 0 {
		t.Fatal("nothing marked to begin with")
	}

	m = applyMsg(t, m, keyMsg("/"))
	for _, r := range "yaz" {
		m = applyMsg(t, m, keyMsg(string(r)))
	}

	if m.checkedCount() != 0 {
		t.Errorf("%d marks survived a filter change, want none", m.checkedCount())
	}
}

func TestBatchRunsOnlyForTheTabsOwnAction(t *testing.T) {
	m := atTab(t, tabInstalled)
	m = applyMsg(t, m, keyMsg("a"))
	marked := m.checkedCount()
	if marked < 2 {
		t.Fatalf("need at least two marked packages, got %d", marked)
	}

	// x is what the Installed tab stands for, so it takes the marks.
	batch := applyMsg(t, m, keyMsg("x"))
	if batch.screen != screenConfirm {
		t.Fatalf("screen = %v after x, want the confirmation", batch.screen)
	}
	if len(batch.pendingBatch) != marked {
		t.Errorf("batch holds %d packages, want the %d marked", len(batch.pendingBatch), marked)
	}

	// u is not, so it keeps acting on the row under the cursor.
	single := selectPackage(t, m, "yazi")
	single = applyMsg(t, single, keyMsg("u"))
	if single.screen != screenConfirm {
		t.Fatalf("screen = %v after u, want the confirmation", single.screen)
	}
	if len(single.pendingBatch) != 0 {
		t.Errorf("u ran on %d marked packages, want it to act on the cursor row", len(single.pendingBatch))
	}
	if single.pendingItem.pkg.Name != "yazi" {
		t.Errorf("pending package = %q, want the one under the cursor", single.pendingItem.pkg.Name)
	}
}

func TestBatchSkipsWhatTheActionCannotTouch(t *testing.T) {
	m := atTab(t, tabUpdates)
	m = applyMsg(t, m, keyMsg("a"))

	// Marks are made per tab, so everything marked is eligible by
	// construction. The skipped list still exists for the case where the
	// registry reloads between the mark and the confirmation.
	m = applyMsg(t, m, keyMsg("u"))
	if len(m.pendingSkipped) != 0 {
		t.Errorf("skipped = %v, want none within a tab", m.pendingSkipped)
	}
	if len(m.pendingBatch) == 0 {
		t.Error("nothing queued for the batch")
	}
}

func TestEligible(t *testing.T) {
	packages, installed := samplePackages()
	notInstalled := packageItem{pkg: packages[0]}
	current := packageItem{pkg: packages[1], installed: installed["fzf"]}
	behind := packageItem{pkg: packages[2], installed: installed["yazi"]}

	tests := []struct {
		name  string
		entry packageItem
		act   action
		want  bool
	}{
		{"install what is not installed", notInstalled, actionInstall, true},
		{"install what is installed", current, actionInstall, false},
		{"update what is behind", behind, actionUpdate, true},
		{"update what is current", current, actionUpdate, false},
		{"update what is not installed", notInstalled, actionUpdate, false},
		{"remove what is installed", current, actionRemove, true},
		{"remove what is not installed", notInstalled, actionRemove, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := eligible(tt.entry, tt.act); got != tt.want {
				t.Errorf("eligible() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBatchConfirmationNamesThePackages(t *testing.T) {
	m := atTab(t, tabInstalled)
	m = applyMsg(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = applyMsg(t, m, keyMsg("a"))
	m = applyMsg(t, m, keyMsg("x"))

	view := m.View()
	if !strings.Contains(view, "Remove 2 package(s)?") {
		t.Errorf("the dialog does not say what will happen:\n%s", view)
	}
	// A batch remove is the most destructive thing clipack does, so the
	// packages are named rather than only counted.
	for _, name := range []string{"fzf", "yazi"} {
		if !strings.Contains(view, name) {
			t.Errorf("the dialog does not name %q:\n%s", name, view)
		}
	}
}

func TestBatchConfirmationShowsTheVersionChange(t *testing.T) {
	m := atTab(t, tabUpdates)
	m = applyMsg(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = applyMsg(t, m, keyMsg("a"))
	m = applyMsg(t, m, keyMsg("u"))

	view := m.View()
	for _, want := range []string{"v25.0.0", "v25.4.8", "→"} {
		if !strings.Contains(view, want) {
			t.Errorf("the dialog does not show the change (%q missing):\n%s", want, view)
		}
	}
}

func TestBatchRunClearsTheMarks(t *testing.T) {
	m := atTab(t, tabInstalled)
	m = applyMsg(t, m, keyMsg("a"))
	m = applyMsg(t, m, keyMsg("x"))

	m = applyMsg(t, m, keyMsg("y"))

	if m.screen != screenRun {
		t.Fatalf("screen = %v after confirming, want the run screen", m.screen)
	}
	// The marks named an operation that is now under way; leaving them set
	// would arm the next keypress with a stale batch.
	if m.checkedCount() != 0 {
		t.Errorf("%d marks survived the run, want none", m.checkedCount())
	}
	if !strings.Contains(m.runTarget, "packages") {
		t.Errorf("runTarget = %q, want it to describe the batch", m.runTarget)
	}
}

func TestHeaderShowsTheMarkCount(t *testing.T) {
	m := atTab(t, tabInstalled)
	m = applyMsg(t, m, tea.WindowSizeMsg{Width: 140, Height: 40})

	if strings.Contains(m.header(), "marked") {
		t.Error("the header mentions marks before anything is marked")
	}

	m = applyMsg(t, m, keyMsg("a"))
	if !strings.Contains(m.header(), "2 marked") {
		t.Errorf("the header does not show the count:\n%s", m.header())
	}
}

func TestHelpShowsTheBatchCount(t *testing.T) {
	m := atTab(t, tabNotInstalled)
	m = applyMsg(t, m, keyMsg("a"))

	// The label is what tells you the key stopped meaning "the row under the
	// cursor".
	if help := helpFor(m); !strings.Contains(help, "install 1") {
		t.Errorf("help = %q, want the batch count on the action key", help)
	}
}

func TestMarkKeysAreOfferedOnlyWhereTheyWork(t *testing.T) {
	if help := helpFor(atTab(t, tabAll)); strings.Contains(help, "mark") {
		t.Errorf("help in the All tab = %q, want the mark keys left out", help)
	}
	if help := helpFor(atTab(t, tabUpdates)); !strings.Contains(help, "mark") {
		t.Errorf("help in the Updates tab = %q, want the mark keys offered", help)
	}
}

func TestSingleFailureIsReportedOnce(t *testing.T) {
	m := browseModel(t)
	m.screen = screenRun
	m.runAction = actionInstall
	m.runTarget = "yazi"

	// A one-package run gets its error from the joined result only. Reporting
	// it inline as well printed the same line twice.
	m = applyMsg(t, m, opFinishedMsg{err: errors.New(`yazi: step 2/2 "cargo build" failed`)})

	if got := strings.Count(strings.Join(m.logLines, "\n"), "cargo build"); got != 1 {
		t.Errorf("the failure appears %d times in the log, want once:\n%s",
			got, strings.Join(m.logLines, "\n"))
	}
}

// TestMethodKeysMeanTheSameInEveryTab is the rule the split was made for. The
// keys used to swap roles with the tab, so what m did depended on where you
// were standing rather than on what you were pointing at.
func TestMethodKeysMeanTheSameInEveryTab(t *testing.T) {
	for _, tab := range []tabID{tabAll, tabInstalled, tabNotInstalled, tabUpdates} {
		m := atTab(t, tab)
		before := m.method

		m = applyMsg(t, m, keyMsg("M"))

		if m.method == before {
			t.Errorf("tab %v: M did not change the global method", tab)
		}
		if m.screen != screenBrowse {
			t.Errorf("tab %v: M opened %v, want it to stay on the list", tab, m.screen)
		}
		if !strings.Contains(m.status, "Global") {
			t.Errorf("tab %v: status = %q, want it to name the global setting", tab, m.status)
		}
	}
}

// TestMethodKeyChoosesPerPackageInEveryTab covers m in the tabs where it used to
// move the global default: pressing it must now leave that alone.
func TestMethodKeyChoosesPerPackageInEveryTab(t *testing.T) {
	for _, tab := range []tabID{tabAll, tabNotInstalled} {
		m := atTab(t, tab)
		m = selectPackage(t, m, "bat")
		before := m.method

		m = applyMsg(t, m, keyMsg("m"))

		if m.method != before {
			t.Errorf("tab %v: m moved the global method to %q", tab, m.method)
		}
		if got := m.methodOf("bat"); got == before {
			t.Errorf("tab %v: methodOf(bat) = %q, want the package's own choice", tab, got)
		}
	}
}

func TestMethodKeyRepinsInTheInstalledTab(t *testing.T) {
	m := atTab(t, tabInstalled)
	m = selectPackage(t, m, "fzf")
	before := m.method

	m = applyMsg(t, m, keyMsg("m"))

	if m.screen != screenConfirm {
		t.Fatalf("screen = %v, want the switch confirmation", m.screen)
	}
	if m.pending != actionSwitchMethod {
		t.Errorf("pending = %v, want actionSwitchMethod", m.pending)
	}
	// fzf is installed by version, so the offer is the other pin.
	if m.pendingMethod != pkg.MethodCommit {
		t.Errorf("pendingMethod = %q, want commit", m.pendingMethod)
	}
	// The global default is a separate setting and must not move with it.
	if m.method != before {
		t.Errorf("global method changed to %q, want it left at %q", m.method, before)
	}
}

func TestSwitchConfirmationSpellsOutWhatIsLost(t *testing.T) {
	m := atTab(t, tabInstalled)
	m = applyMsg(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = selectPackage(t, m, "fzf")
	m = applyMsg(t, m, keyMsg("m"))

	view := m.View()
	for _, want := range []string{
		"Switch fzf to commit?",
		"version",       // what it is pinned to now
		"rebuilds",      // that this is not a metadata change
		"configuration", // the part that cannot be rebuilt
		"lost",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("the dialog does not mention %q:\n%s", want, view)
		}
	}
}

func TestSwitchRefusesWithoutARefToSwitchTo(t *testing.T) {
	m := atTab(t, tabInstalled)
	m = selectPackage(t, m, "fzf")

	// A registry entry with no commit cannot be pinned to one.
	for i, item := range m.list.Items() {
		if entry, ok := item.(packageItem); ok && entry.pkg.Name == "fzf" {
			entry.pkg.Commit = ""
			m.list.SetItem(i, entry)
		}
	}
	m = selectPackage(t, m, "fzf")

	m = applyMsg(t, m, keyMsg("m"))
	if m.screen != screenBrowse {
		t.Errorf("screen = %v, want the switch refused", m.screen)
	}
	if !strings.Contains(m.status, "no commit") {
		t.Errorf("status = %q, want it to say why", m.status)
	}
}

func TestSwitchRunsAsAnUpdateWithTheNewMethod(t *testing.T) {
	m := atTab(t, tabInstalled)
	m = selectPackage(t, m, "fzf")
	m = applyMsg(t, m, keyMsg("m"))

	m = applyMsg(t, m, keyMsg("y"))

	if m.screen != screenRun {
		t.Fatalf("screen = %v, want the run screen", m.screen)
	}
	if m.runAction != actionSwitchMethod {
		t.Errorf("runAction = %v, want actionSwitchMethod", m.runAction)
	}
	if m.runTarget != "fzf" {
		t.Errorf("runTarget = %q, want fzf", m.runTarget)
	}
}

func TestActionPastTense(t *testing.T) {
	// "install" + "d" gave "installd" in the status line for every install.
	tests := map[action]string{
		actionInstall:      "installed",
		actionUpdate:       "updated",
		actionRemove:       "removed",
		actionSwitchMethod: "switched",
	}
	for a, want := range tests {
		if got := a.past(); got != want {
			t.Errorf("action(%d).past() = %q, want %q", a, got, want)
		}
	}
}

func TestDetailShowsTheInstallMethod(t *testing.T) {
	m := browseModel(t)

	// Installed: the pin is a field of its own, because the header only shows
	// the global default.
	m = selectPackage(t, m, "fzf")
	if detail := m.detail.View(); !strings.Contains(detail, "Install method") {
		t.Errorf("the detail pane does not show the install method:\n%s", detail)
	}

	// Not installed: there is no pin to show.
	m = selectPackage(t, m, "bat")
	if detail := m.detail.View(); strings.Contains(detail, "Install method") {
		t.Errorf("the detail pane shows an install method for a package that is not installed:\n%s", detail)
	}
}

func TestHeaderCallsTheMethodGlobal(t *testing.T) {
	m := browseModel(t)
	m = applyMsg(t, m, tea.WindowSizeMsg{Width: 140, Height: 40})

	if !strings.Contains(m.header(), "global install method") {
		t.Errorf("header = %q, want the method labelled as the global default", m.header())
	}
}

func TestEveryTabShowsItsCount(t *testing.T) {
	m := browseModel(t)
	m = applyMsg(t, m, tea.WindowSizeMsg{Width: 140, Height: 40})

	// The sample registry: 3 packages, 2 installed, 1 of those behind.
	header := m.header()
	for _, want := range []string{"All (3)", "Installed (2)", "Not installed (1)", "Updates (1)"} {
		if !strings.Contains(header, want) {
			t.Errorf("header does not show %q:\n%s", want, header)
		}
	}
}

func TestTabCountsAreDroppedOnANarrowTerminal(t *testing.T) {
	m := browseModel(t)
	m = applyMsg(t, m, tea.WindowSizeMsg{Width: 58, Height: 24})

	header := m.header()
	if strings.Contains(header, "(") {
		t.Errorf("counts kept on a narrow terminal:\n%s", header)
	}
	// Dropping them is what keeps the last tab from being clipped away.
	for _, want := range []string{"All", "Installed", "Not installed", "Updates"} {
		if !strings.Contains(header, want) {
			t.Errorf("tab %q was clipped off:\n%s", want, header)
		}
	}
}

func TestDetailKeysAndValuesAreSeparated(t *testing.T) {
	m := browseModel(t)
	m = selectPackage(t, m, "fzf")

	// "Install method" is the longest label; at a 14-column key it ran straight
	// into its value ("Install methodversion").
	detail := m.detail.View()
	if strings.Contains(detail, "Install methodversion") {
		t.Errorf("the key and value have no gap:\n%s", detail)
	}
	if !strings.Contains(detail, "Install method  version") {
		t.Errorf("the install method row is not aligned:\n%s", detail)
	}
}
