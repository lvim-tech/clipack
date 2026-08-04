package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lvim-tech/clipack/cnfg"
	"github.com/lvim-tech/clipack/pkg"
)

func TestNewWithoutConfigStartsSetup(t *testing.T) {
	m := New(nil)

	if m.screen != screenSetup {
		t.Errorf("screen = %v, want screenSetup when no config exists", m.screen)
	}
	if !m.input.Focused() {
		t.Error("the install-directory input is not focused on the setup screen")
	}
	if m.input.Value() == "" {
		t.Error("the input is not prefilled with a default install directory")
	}
}

func TestNewWithConfigStartsLoading(t *testing.T) {
	config := testConfig(t)
	config.Options.InstallMethod = pkg.MethodCommit

	m := New(config)

	if m.screen != screenLoading {
		t.Errorf("screen = %v, want screenLoading", m.screen)
	}
	if m.method != pkg.MethodCommit {
		t.Errorf("method = %q, want the configured commit", m.method)
	}
}

func TestNewDefaultsMethodWhenUnset(t *testing.T) {
	config := testConfig(t)
	config.Options.InstallMethod = ""

	if m := New(config); m.method != pkg.MethodVersion {
		t.Errorf("method = %q, want version", m.method)
	}
}

func TestActionLabel(t *testing.T) {
	tests := map[action]string{
		actionInstall: "Install",
		actionUpdate:  "Update",
		actionRemove:  "Remove",
		actionNone:    "",
	}
	for a, want := range tests {
		if got := a.label(); got != want {
			t.Errorf("action(%d).label() = %q, want %q", a, got, want)
		}
	}
}

func TestInstalledMethod(t *testing.T) {
	entry := packageItem{
		pkg:       &pkg.Package{Name: "bat"},
		installed: &pkg.Package{InstallMethod: pkg.MethodCommit},
	}
	// An update must follow how the package was installed, not the currently
	// selected method.
	if got := installedMethod(entry, pkg.MethodVersion); got != pkg.MethodCommit {
		t.Errorf("installedMethod() = %q, want the recorded commit method", got)
	}

	noMethod := packageItem{pkg: &pkg.Package{}, installed: &pkg.Package{}}
	if got := installedMethod(noMethod, pkg.MethodVersion); got != pkg.MethodVersion {
		t.Errorf("installedMethod() = %q, want the fallback", got)
	}

	notInstalled := packageItem{pkg: &pkg.Package{}}
	if got := installedMethod(notInstalled, pkg.MethodCommit); got != pkg.MethodCommit {
		t.Errorf("installedMethod() = %q, want the fallback", got)
	}
}

func TestClonePackageDoesNotMutateTheOriginal(t *testing.T) {
	original := &pkg.Package{Name: "bat", InstallMethod: ""}

	copied := clonePackage(original)
	copied.InstallMethod = pkg.MethodCommit
	copied.Name = "changed"

	// The installer writes InstallMethod onto the package it is given; without
	// the copy that would edit the cached registry entry in place.
	if original.InstallMethod != "" {
		t.Errorf("original.InstallMethod = %q, want it untouched", original.InstallMethod)
	}
	if original.Name != "bat" {
		t.Errorf("original.Name = %q, want it untouched", original.Name)
	}
}

func TestFormatEvent(t *testing.T) {
	tests := []struct {
		name  string
		event pkg.Event
		want  []string
	}{
		{
			name:  "step shows a counter",
			event: pkg.Event{Kind: pkg.EventStep, Step: 2, Total: 4, Text: "cargo build"},
			want:  []string{"2/4", "cargo build"},
		},
		{
			name:  "output is indented",
			event: pkg.Event{Kind: pkg.EventOutput, Text: "Compiling bat"},
			want:  []string{"│", "Compiling bat"},
		},
		{
			name:  "warning is marked",
			event: pkg.Event{Kind: pkg.EventWarn, Text: "man page missing"},
			want:  []string{"!", "man page missing"},
		},
		{
			name:  "error is marked",
			event: pkg.Event{Kind: pkg.EventError, Text: "build failed"},
			want:  []string{"✗", "build failed"},
		},
		{
			name:  "done is marked",
			event: pkg.Event{Kind: pkg.EventDone, Text: "installed bat"},
			want:  []string{"✓", "installed bat"},
		},
		{
			name:  "info falls through",
			event: pkg.Event{Kind: pkg.EventInfo, Text: "Removing bat"},
			want:  []string{"Removing bat"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatEvent(tt.event, DefaultStyles())
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Errorf("formatEvent() = %q, want it to contain %q", got, want)
				}
			}
		})
	}
}

func TestLayoutKeepsPanesInsideTheWindow(t *testing.T) {
	// The header used to be pushed off-screen because the chrome height was a
	// hardcoded constant that ignored the pane borders.
	for _, size := range []tea.WindowSizeMsg{
		{Width: 80, Height: 24},
		{Width: 120, Height: 40},
		{Width: 200, Height: 60},
		{Width: 60, Height: 20},
	} {
		m := browseModel(t)
		m = applyMsg(t, m, size)

		view := m.View()
		lines := strings.Split(view, "\n")
		if len(lines) > size.Height {
			t.Errorf("%dx%d: view is %d lines, want at most %d",
				size.Width, size.Height, len(lines), size.Height)
		}
		for _, line := range lines {
			if width := len([]rune(line)); width > size.Width {
				t.Errorf("%dx%d: a line is %d cells wide, want at most %d",
					size.Width, size.Height, width, size.Width)
			}
		}
	}
}

func TestViewDoesNotPanicAtExtremeSizes(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{
		{Width: 1, Height: 1},
		{Width: 10, Height: 3},
		{Width: 20, Height: 5},
		{Width: 300, Height: 100},
	} {
		m := browseModel(t)
		m = applyMsg(t, m, size)
		_ = m.View() // must not panic
	}
}

func TestViewBeforeFirstWindowSize(t *testing.T) {
	// Bubble Tea renders once before the first WindowSizeMsg arrives.
	if out := New(nil).View(); out == "" {
		t.Error("View() before a size is known returned nothing")
	}
}

func TestQuittingRendersNothing(t *testing.T) {
	m := browseModel(t)
	m = applyMsg(t, m, keyMsg("q"))

	if !m.quitting {
		t.Fatal("q did not set quitting")
	}
	if out := m.View(); out != "" {
		t.Errorf("View() while quitting = %q, want an empty string so the terminal is left clean", out)
	}
}

func TestCtrlCQuitsFromEveryScreen(t *testing.T) {
	screens := map[string]Model{
		"browse":  browseModel(t),
		"setup":   New(nil),
		"confirm": func() Model { m := browseModel(t); m.screen = screenConfirm; return m }(),
		"run":     func() Model { m := browseModel(t); m.screen = screenRun; return m }(),
	}

	for name, m := range screens {
		t.Run(name, func(t *testing.T) {
			got := applyMsg(t, m, keyMsg("ctrl+c"))
			if !got.quitting {
				t.Error("ctrl+c did not quit")
			}
		})
	}
}

func TestTabCyclesThroughViews(t *testing.T) {
	m := browseModel(t)

	if m.tab != tabAll {
		t.Fatalf("initial tab = %v, want tabAll", m.tab)
	}

	m = applyMsg(t, m, keyMsg("tab"))
	if m.tab != tabInstalled {
		t.Errorf("after tab: %v, want tabInstalled", m.tab)
	}
	if len(m.list.Items()) != 2 {
		t.Errorf("Installed tab shows %d packages, want 2", len(m.list.Items()))
	}

	m = applyMsg(t, m, keyMsg("tab"))
	if m.tab != tabNotInstalled {
		t.Errorf("after tab: %v, want tabNotInstalled", m.tab)
	}
	if len(m.list.Items()) != 1 {
		t.Errorf("Not installed tab shows %d packages, want 1", len(m.list.Items()))
	}

	m = applyMsg(t, m, keyMsg("tab"))
	if m.tab != tabUpdates {
		t.Errorf("after tab: %v, want tabUpdates", m.tab)
	}
	if len(m.list.Items()) != 1 {
		t.Errorf("Updates tab shows %d packages, want 1", len(m.list.Items()))
	}

	// Wraps back round.
	m = applyMsg(t, m, keyMsg("tab"))
	if m.tab != tabAll {
		t.Errorf("after tab: %v, want it to wrap to tabAll", m.tab)
	}

	m = applyMsg(t, m, keyMsg("shift+tab"))
	if m.tab != tabUpdates {
		t.Errorf("after shift+tab: %v, want it to wrap back to tabUpdates", m.tab)
	}
}

func TestGlobalMethodToggle(t *testing.T) {
	m := browseModel(t)

	m = applyMsg(t, m, keyMsg("M"))
	if m.method != pkg.MethodCommit {
		t.Errorf("method = %q after M, want commit", m.method)
	}
	if !strings.Contains(m.status, pkg.MethodCommit) {
		t.Errorf("status = %q, want it to report the new method", m.status)
	}

	m = applyMsg(t, m, keyMsg("M"))
	if m.method != pkg.MethodVersion {
		t.Errorf("method = %q after a second M, want version", m.method)
	}
}

// TestMethodChoiceIsPerPackage is the point of splitting the key in two: before
// it, choosing commit for one package that was not installed yet meant choosing
// it for every package, because the only method there was the global one.
func TestMethodChoiceIsPerPackage(t *testing.T) {
	m := browseModel(t)
	m = selectPackage(t, m, "bat")

	m = applyMsg(t, m, keyMsg("m"))
	if got := m.methodOf("bat"); got != pkg.MethodCommit {
		t.Errorf("methodOf(bat) = %q after m, want commit", got)
	}
	if m.method != pkg.MethodVersion {
		t.Errorf("global method = %q, want it untouched by m", m.method)
	}
	if got := m.methodOf("yazi"); got != pkg.MethodVersion {
		t.Errorf("methodOf(yazi) = %q, want the other packages left on the default", got)
	}
	if !strings.Contains(m.status, "bat") || !strings.Contains(m.status, pkg.MethodCommit) {
		t.Errorf("status = %q, want it to name the package and the method", m.status)
	}

	// Pressing it again returns the package to the default rather than storing a
	// second, equal value — which is what lets M move it again.
	m = applyMsg(t, m, keyMsg("m"))
	if got := m.methodOf("bat"); got != pkg.MethodVersion {
		t.Errorf("methodOf(bat) = %q after a second m, want version", got)
	}
	if _, pinned := m.methodFor["bat"]; pinned {
		t.Error("the package is still pinned after returning to the default")
	}
}

// TestPerPackageMethodSurvivesTheGlobalToggle keeps the two keys apart: a choice
// made deliberately for one package must not follow the default around.
func TestPerPackageMethodSurvivesTheGlobalToggle(t *testing.T) {
	m := browseModel(t)
	m = selectPackage(t, m, "bat")
	m = applyMsg(t, m, keyMsg("m"))

	m = applyMsg(t, m, keyMsg("M"))
	if m.method != pkg.MethodCommit {
		t.Fatalf("global method = %q after M, want commit", m.method)
	}
	if got := m.methodOf("bat"); got != pkg.MethodCommit {
		t.Errorf("methodOf(bat) = %q, want its own choice kept", got)
	}

	m = applyMsg(t, m, keyMsg("M"))
	if got := m.methodOf("bat"); got != pkg.MethodCommit {
		t.Errorf("methodOf(bat) = %q after the default moved back, want the choice kept", got)
	}
}

// TestMethodOnAnInstalledPackageStillRepins covers the other half of the key: an
// installed package has a manifest, so m means a rebuild onto the other ref and
// asks before doing it.
func TestMethodOnAnInstalledPackageStillRepins(t *testing.T) {
	m := browseModel(t)
	m = selectPackage(t, m, "fzf")

	m = applyMsg(t, m, keyMsg("m"))
	if m.screen != screenConfirm {
		t.Fatalf("screen = %v, want the switch confirmed first (status: %q)", m.screen, m.status)
	}
	if m.pending != actionSwitchMethod {
		t.Errorf("pending = %v, want actionSwitchMethod", m.pending)
	}
	if len(m.methodFor) != 0 {
		t.Errorf("methodFor = %v, want an installed package to be repinned rather than marked", m.methodFor)
	}
}

func TestHelpToggle(t *testing.T) {
	m := browseModel(t)

	m = applyMsg(t, m, keyMsg("?"))
	if !m.showFullHelp || !m.help.ShowAll {
		t.Error("? did not expand the help")
	}
	// The footer grows, so the body must shrink to keep the view on screen.
	expanded := strings.Count(m.View(), "\n")

	m = applyMsg(t, m, keyMsg("?"))
	if m.showFullHelp || m.help.ShowAll {
		t.Error("? did not collapse the help again")
	}

	if collapsed := strings.Count(m.View(), "\n"); collapsed > expanded {
		t.Errorf("collapsed view is %d lines and expanded is %d; the layout did not adapt", collapsed, expanded)
	}
}

func TestSelectionUpdatesTheDetailPane(t *testing.T) {
	m := browseModel(t)

	if !strings.Contains(m.detail.View(), "bat") {
		t.Fatalf("detail pane does not show the first package: %q", m.detail.View())
	}

	m = applyMsg(t, m, keyMsg("j"))
	if m.detailFor != "fzf" {
		t.Errorf("detailFor = %q after moving down, want fzf", m.detailFor)
	}
	if !strings.Contains(m.detail.View(), "fuzzy finder") {
		t.Errorf("detail pane did not follow the selection: %q", m.detail.View())
	}
}

func TestDetailRefreshIsIdempotent(t *testing.T) {
	m := browseModel(t)
	m = selectPackage(t, m, "bat")

	// A short pane, so the content is actually scrollable: viewport clamps the
	// offset to zero when everything already fits.
	m.detail.Height = 3

	// Scroll, then re-render for the same package: the position must survive,
	// which is why refreshDetail is keyed on the package name.
	m.detail.SetYOffset(3)
	if m.detail.YOffset != 3 {
		t.Fatalf("YOffset = %d, want the pane scrolled before the check", m.detail.YOffset)
	}
	m.refreshDetail()

	if m.detail.YOffset != 3 {
		t.Errorf("YOffset = %d after a redundant refresh, want 3", m.detail.YOffset)
	}

	m.invalidateDetail()
	m.refreshDetail()
	if m.detail.YOffset != 0 {
		t.Errorf("YOffset = %d after invalidation, want the pane reset to the top", m.detail.YOffset)
	}
}

func TestFilteringRoutesKeysToTheList(t *testing.T) {
	m := browseModel(t)

	m = applyMsg(t, m, keyMsg("/"))
	for _, r := range "yazi" {
		m = applyMsg(t, m, keyMsg(string(r)))
	}

	// "i" is the install binding, but while the filter is focused it has to be
	// typed into the filter instead of starting an install.
	if m.screen != screenBrowse {
		t.Errorf("screen = %v while filtering, want it to stay on browse", m.screen)
	}
	if m.list.FilterValue() != "yazi" {
		t.Errorf("filter = %q, want yazi", m.list.FilterValue())
	}
}

func TestPaneFocus(t *testing.T) {
	m := browseModel(t)

	if m.focus != focusList {
		t.Fatalf("focus = %v, want the list focused at startup", m.focus)
	}

	m = applyMsg(t, m, keyMsg("right"))
	if m.focus != focusDetail {
		t.Errorf("focus = %v after →, want the detail pane", m.focus)
	}

	m = applyMsg(t, m, keyMsg("left"))
	if m.focus != focusList {
		t.Errorf("focus = %v after ←, want the list", m.focus)
	}

	// h/l are the vim equivalents.
	m = applyMsg(t, m, keyMsg("l"))
	if m.focus != focusDetail {
		t.Errorf("focus = %v after l, want the detail pane", m.focus)
	}
	m = applyMsg(t, m, keyMsg("h"))
	if m.focus != focusList {
		t.Errorf("focus = %v after h, want the list", m.focus)
	}
}

func TestEscapeLeavesTheDetailPane(t *testing.T) {
	m := browseModel(t)
	m = applyMsg(t, m, keyMsg("right"))

	m = applyMsg(t, m, keyMsg("esc"))
	if m.focus != focusList {
		t.Errorf("focus = %v after esc, want the list", m.focus)
	}
}

func TestMovementFollowsTheFocus(t *testing.T) {
	m := browseModel(t)
	// A short pane, so the cursor runs past the bottom quickly. Done through the
	// window size rather than by assigning detail.Height: that field belongs to
	// layout(), which reruns whenever the help line changes height — and it does,
	// as soon as the selection moves onto an installed package and gains a key.
	m = applyMsg(t, m, tea.WindowSizeMsg{Width: 120, Height: 12})

	// With the list focused, j moves the selection and leaves the detail at
	// the top.
	m = applyMsg(t, m, keyMsg("j"))
	if m.list.Index() != 1 {
		t.Errorf("list index = %d after j, want the selection moved", m.list.Index())
	}
	if m.detail.YOffset != 0 {
		t.Errorf("detail scrolled to %d while the list was focused", m.detail.YOffset)
	}

	// With the detail focused, the same key drives the text cursor and leaves
	// the package selection alone.
	m = applyMsg(t, m, keyMsg("right"))
	index := m.list.Index()

	for i := 0; i < 6; i++ {
		m = applyMsg(t, m, keyMsg("j"))
	}

	if m.cursor.line != 6 {
		t.Errorf("cursor.line = %d, want the text cursor to have moved", m.cursor.line)
	}
	// The pane scrolls only once the cursor leaves the visible window.
	if m.detail.YOffset == 0 {
		t.Error("the detail pane did not scroll to follow the cursor")
	}
	if m.list.Index() != index {
		t.Errorf("list index = %d, want the selection untouched while the detail is focused", m.list.Index())
	}
}

func TestActionsStillWorkFromTheDetailPane(t *testing.T) {
	m := browseModel(t)
	m = selectPackage(t, m, "bat")
	m = applyMsg(t, m, keyMsg("right"))

	// The selection is still what the actions apply to, so they must not
	// require moving the focus back first.
	m = applyMsg(t, m, keyMsg("i"))
	if m.screen != screenConfirm {
		t.Errorf("screen = %v, want install to work from the detail pane", m.screen)
	}
	if m.pendingItem.pkg.Name != "bat" {
		t.Errorf("pending package = %q, want bat", m.pendingItem.pkg.Name)
	}
}

func TestFilterReturnsFocusToTheList(t *testing.T) {
	m := browseModel(t)
	m = applyMsg(t, m, keyMsg("right"))

	m = applyMsg(t, m, keyMsg("/"))
	if m.focus != focusList {
		t.Errorf("focus = %v, want filtering to take the focus back to the list", m.focus)
	}
}

func TestFocusIsVisibleWithColor(t *testing.T) {
	m := browseModel(t)

	// The panes swap border colours. The rendered strings cannot be compared
	// here — a test binary has no TTY, so lipgloss strips every colour — so the
	// styles themselves are what get checked.
	focused := m.styles.PaneFocused.GetBorderTopForeground()
	unfocused := m.styles.Pane.GetBorderTopForeground()
	if focused == unfocused {
		t.Error("the focused and unfocused panes use the same border colour")
	}
}

func TestFocusIsVisibleWithoutColor(t *testing.T) {
	config := testConfig(t)
	config.Theme = cnfg.Theme{Name: "mono"}

	m := New(config)
	m = applyMsg(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	packages, installed := samplePackages()
	m = applyMsg(t, m, registryLoadedMsg{packages: packages, installed: installed})

	// mono has no accent colour, so the border cannot signal focus by colour.
	// It changes weight instead, which survives a monochrome terminal.
	listFocused := m.View()
	detailFocused := applyMsg(t, m, keyMsg("right")).View()

	if listFocused == detailFocused {
		t.Error("without colour the view is identical whichever pane is focused")
	}
	if !strings.Contains(listFocused, "┏") && !strings.Contains(listFocused, "┃") {
		t.Errorf("the focused pane does not use a heavier border:\n%s", listFocused)
	}
}

func TestRequestActionGuards(t *testing.T) {
	tests := []struct {
		name        string
		packageName string
		key         string
		wantScreen  screen
		wantStatus  string
	}{
		{
			name:        "update a package that is not installed",
			packageName: "bat",
			key:         "u",
			wantScreen:  screenBrowse,
			wantStatus:  "not installed",
		},
		{
			name:        "update a package that is current",
			packageName: "fzf",
			key:         "u",
			wantScreen:  screenBrowse,
			wantStatus:  "up to date",
		},
		{
			name:        "remove a package that is not installed",
			packageName: "bat",
			key:         "x",
			wantScreen:  screenBrowse,
			wantStatus:  "not installed",
		},
		{
			name:        "update a package that is behind",
			packageName: "yazi",
			key:         "u",
			wantScreen:  screenConfirm,
		},
		{
			name:        "install something that is not installed",
			packageName: "bat",
			key:         "i",
			wantScreen:  screenConfirm,
		},
		{
			// Reinstalling over an existing install would strand the previous
			// version's binaries: only update reads the old manifest and knows
			// what to clean up.
			name:        "install a package that is already installed",
			packageName: "fzf",
			key:         "i",
			wantScreen:  screenBrowse,
			wantStatus:  "is installed",
		},
		{
			name:        "install a package that has an update",
			packageName: "yazi",
			key:         "i",
			wantScreen:  screenBrowse,
			wantStatus:  "u to update",
		},
		{
			name:        "remove an installed package",
			packageName: "fzf",
			key:         "x",
			wantScreen:  screenConfirm,
		},
		{
			// The case no other key covers: the version has not moved, but the
			// registry entry may have.
			name:        "rebuild a package that is current",
			packageName: "fzf",
			key:         "R",
			wantScreen:  screenConfirm,
		},
		{
			name:        "rebuild a package that is behind",
			packageName: "yazi",
			key:         "R",
			wantScreen:  screenConfirm,
		},
		{
			name:        "rebuild a package that is not installed",
			packageName: "bat",
			key:         "R",
			wantScreen:  screenBrowse,
			wantStatus:  "not installed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := browseModel(t)
			m = selectPackage(t, m, tt.packageName)
			m = applyMsg(t, m, keyMsg(tt.key))

			if m.screen != tt.wantScreen {
				t.Errorf("screen = %v, want %v (status: %q)", m.screen, tt.wantScreen, m.status)
			}
			if tt.wantStatus != "" && !strings.Contains(m.status, tt.wantStatus) {
				t.Errorf("status = %q, want it to mention %q", m.status, tt.wantStatus)
			}
		})
	}
}

func TestConfirmCancel(t *testing.T) {
	m := browseModel(t)
	m = selectPackage(t, m, "bat")
	m = applyMsg(t, m, keyMsg("i"))

	if m.screen != screenConfirm {
		t.Fatalf("screen = %v, want screenConfirm", m.screen)
	}

	m = applyMsg(t, m, keyMsg("esc"))
	if m.screen != screenBrowse {
		t.Errorf("screen = %v after esc, want screenBrowse", m.screen)
	}
	if m.pending != actionNone {
		t.Errorf("pending = %v after cancelling, want actionNone", m.pending)
	}
}

func TestConfirmViewDescribesTheAction(t *testing.T) {
	tests := []struct {
		packageName string
		key         string
		want        []string
	}{
		{"bat", "i", []string{"Install bat?", "method", "v0.25.0"}},
		{"yazi", "u", []string{"Update yazi?", "installed", "available"}},
		{"fzf", "x", []string{"Remove fzf?", "binaries"}},
		// A rebuild has to say the two things that distinguish it from an
		// update: nothing about the version changes, and the configuration
		// directory does not survive.
		{"fzf", "R", []string{"Reinstall fzf?", "Nothing about the version changes", "configuration directory is deleted"}},
	}

	for _, tt := range tests {
		t.Run(tt.packageName+"/"+tt.key, func(t *testing.T) {
			m := browseModel(t)
			m = selectPackage(t, m, tt.packageName)
			m = applyMsg(t, m, keyMsg(tt.key))

			view := m.View()
			for _, want := range tt.want {
				if !strings.Contains(view, want) {
					t.Errorf("confirm view is missing %q:\n%s", want, view)
				}
			}
		})
	}
}

func TestRegistryLoadedFailure(t *testing.T) {
	m := New(testConfig(t))
	m = applyMsg(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	m = applyMsg(t, m, registryLoadedMsg{err: errors.New("network unreachable")})

	if m.screen != screenBrowse {
		t.Errorf("screen = %v, want screenBrowse so the error is shown", m.screen)
	}
	view := m.View()
	if !strings.Contains(view, "network unreachable") {
		t.Errorf("view does not show the error:\n%s", view)
	}
	if !strings.Contains(view, "press r to retry") {
		t.Errorf("view does not offer a retry:\n%s", view)
	}
}

func TestRegistryLoadedWithWarning(t *testing.T) {
	m := New(testConfig(t))
	m = applyMsg(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	packages, installed := samplePackages()
	// A partial fetch: the packages are usable but the problem is surfaced.
	m = applyMsg(t, m, registryLoadedMsg{
		packages: packages, installed: installed,
		warn: errors.New("skipped 2 unreadable packages"),
	})

	if m.screen != screenBrowse {
		t.Fatalf("screen = %v, want screenBrowse", m.screen)
	}
	if len(m.list.Items()) != 3 {
		t.Errorf("got %d items, want the packages that did load", len(m.list.Items()))
	}
	if !strings.Contains(m.status, "skipped 2") {
		t.Errorf("status = %q, want the warning surfaced", m.status)
	}
}

func TestRefreshShowsLoadingScreen(t *testing.T) {
	m := browseModel(t)

	m, cmd := applyMsgCmd(t, m, keyMsg("r"))
	if m.screen != screenLoading {
		t.Errorf("screen = %v after r, want screenLoading", m.screen)
	}
	if cmd == nil {
		t.Error("r did not schedule a refresh command")
	}
	if !strings.Contains(m.status, "Refresh") {
		t.Errorf("status = %q, want it to say what is happening", m.status)
	}
}

func TestSetupFlow(t *testing.T) {
	m := New(nil)
	m = applyMsg(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	if !strings.Contains(m.View(), "first run") {
		t.Errorf("setup view does not identify itself:\n%s", m.View())
	}

	// The shell-integration checkbox toggles with space or tab.
	if !m.setupAddShell {
		t.Fatal("shell integration should default to on")
	}
	m = applyMsg(t, m, keyMsg(" "))
	if m.setupAddShell {
		t.Error("space did not toggle the shell-integration checkbox")
	}
	m = applyMsg(t, m, keyMsg("tab"))
	if !m.setupAddShell {
		t.Error("tab did not toggle the checkbox back")
	}

	// First enter answers the installation directory and moves on to the
	// registry — clipack has no default for it, so the wizard has to ask.
	m = applyMsg(t, m, keyMsg("enter"))
	if m.screen != screenSetup {
		t.Fatalf("screen = %v, want the wizard still open for the registry", m.screen)
	}
	if !strings.Contains(m.View(), "Registry URL") {
		t.Errorf("second step does not ask for the registry:\n%s", m.View())
	}

	// An empty answer is refused rather than guessed at.
	m = applyMsg(t, m, keyMsg("enter"))
	if m.screen != screenSetup || m.err == nil {
		t.Errorf("an empty registry was accepted: screen = %v, err = %v", m.screen, m.err)
	}

	m.input.SetValue("https://github.com/owner/repo.git")
	m, cmd := applyMsgCmd(t, m, keyMsg("enter"))
	if m.screen != screenLoading {
		t.Errorf("screen = %v after the registry was given, want screenLoading", m.screen)
	}
	if cmd == nil {
		t.Error("enter did not schedule the config save")
	}
}

func TestSetupDoneError(t *testing.T) {
	m := New(nil)
	m = applyMsg(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = applyMsg(t, m, setupDoneMsg{err: errors.New("permission denied")})

	if m.screen != screenSetup {
		t.Errorf("screen = %v, want the wizard to stay open after a failure", m.screen)
	}
	if !strings.Contains(m.View(), "permission denied") {
		t.Errorf("the error is not shown:\n%s", m.View())
	}
}

func TestSetupDoneSuccess(t *testing.T) {
	config := testConfig(t)

	m := New(nil)
	m = applyMsg(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m, cmd := applyMsgCmd(t, m, setupDoneMsg{config: config})

	if m.config != config {
		t.Error("the saved config was not adopted")
	}
	if m.screen != screenLoading {
		t.Errorf("screen = %v, want it to move on to loading the registry", m.screen)
	}
	if cmd == nil {
		t.Error("no registry load was scheduled after setup")
	}
}

func TestRunScreenAccumulatesLog(t *testing.T) {
	m := browseModel(t)
	m.screen = screenRun
	m.runAction = actionInstall
	m.runTarget = "bat"

	m = applyMsg(t, m, opEventsMsg{events: []pkg.Event{
		{Kind: pkg.EventStep, Step: 1, Total: 2, Text: "git clone"},
		{Kind: pkg.EventOutput, Text: "Cloning into '.'"},
		{Kind: pkg.EventStep, Step: 2, Total: 2, Text: "cargo build"},
	}})

	if len(m.logLines) != 3 {
		t.Fatalf("got %d log lines, want 3", len(m.logLines))
	}
	view := m.logView.View()
	if !strings.Contains(view, "cargo build") {
		t.Errorf("the log view does not show the latest output:\n%s", view)
	}
	if m.runDone {
		t.Error("runDone = true while events are still arriving")
	}
}

func TestRunScreenMarksFailureFromEvents(t *testing.T) {
	m := browseModel(t)
	m.screen = screenRun
	m.runAction = actionInstall
	m.runTarget = "bat"

	m = applyMsg(t, m, opEventsMsg{events: []pkg.Event{
		{Kind: pkg.EventError, Text: "compiler not found"},
	}})

	if !m.runFailed {
		t.Error("an error event did not mark the run as failed")
	}
}

func TestRunScreenFinishesWithTheBatch(t *testing.T) {
	m := browseModel(t)
	m.screen = screenRun
	m.runAction = actionInstall
	m.runTarget = "bat"

	// The tail of the output and the completion arrive together, so nothing is
	// lost when the operation ends mid-batch.
	m = applyMsg(t, m, opEventsMsg{
		events:   []pkg.Event{{Kind: pkg.EventDone, Text: "Successfully installed bat"}},
		finished: true,
	})

	if !m.runDone {
		t.Error("runDone = false after a finished batch")
	}
	if m.runFailed {
		t.Error("runFailed = true for a successful run")
	}
	if !strings.Contains(strings.Join(m.logLines, "\n"), "Successfully installed bat") {
		t.Error("the final event was dropped")
	}
}

func TestRunScreenReportsAFailedOperation(t *testing.T) {
	m := browseModel(t)
	m.screen = screenRun
	m.runAction = actionInstall
	m.runTarget = "bat"

	m = applyMsg(t, m, opFinishedMsg{err: errors.New("step 2/4 failed")})

	if !m.runFailed {
		t.Error("runFailed = false after an error")
	}
	if !strings.Contains(strings.Join(m.logLines, "\n"), "step 2/4 failed") {
		t.Error("the error was not appended to the log")
	}
	if !strings.Contains(m.View(), "failed") {
		t.Errorf("the run view does not report the failure:\n%s", m.View())
	}
}

func TestRunScreenCapsTheLog(t *testing.T) {
	m := browseModel(t)
	m.screen = screenRun

	// A long compile must not grow the retained log without bound.
	events := make([]pkg.Event, maxLogLines+500)
	for i := range events {
		events[i] = pkg.Event{Kind: pkg.EventOutput, Text: "line"}
	}
	m = applyMsg(t, m, opEventsMsg{events: events})

	if len(m.logLines) != maxLogLines {
		t.Errorf("retained %d log lines, want the cap of %d", len(m.logLines), maxLogLines)
	}
}

func TestRunScreenRefusesToExitWhileRunning(t *testing.T) {
	m := browseModel(t)
	m.screen = screenRun
	m.runDone = false

	m = applyMsg(t, m, keyMsg("esc"))
	if m.screen != screenRun {
		t.Error("esc left the run screen while the operation was still going")
	}
	if !strings.Contains(m.status, "still running") {
		t.Errorf("status = %q, want it to explain why esc did nothing", m.status)
	}
}

func TestRunScreenReturnsToBrowseWhenDone(t *testing.T) {
	m := browseModel(t)
	m.screen = screenRun
	m.runDone = true
	m.runAction = actionInstall
	m.runTarget = "bat"

	m = applyMsg(t, m, keyMsg("esc"))
	if m.screen != screenBrowse {
		t.Errorf("screen = %v after esc, want screenBrowse", m.screen)
	}
	if !strings.Contains(m.status, "bat") {
		t.Errorf("status = %q, want a summary of what happened", m.status)
	}
}

func TestRunSummary(t *testing.T) {
	m := browseModel(t)
	m.runTarget = "bat"

	m.runAction = actionInstall
	m.runFailed = false
	if got := m.runSummary(); !strings.Contains(got, "bat") || !strings.Contains(got, "install") {
		t.Errorf("runSummary() = %q, want it to name the package and the action", got)
	}

	m.runFailed = true
	if got := m.runSummary(); !strings.Contains(got, "failed") {
		t.Errorf("runSummary() = %q, want it to report the failure", got)
	}
}

func TestInitReturnsACommand(t *testing.T) {
	if cmd := New(nil).Init(); cmd == nil {
		t.Error("Init() on the setup screen returned no command")
	}
	if cmd := New(testConfig(t)).Init(); cmd == nil {
		t.Error("Init() with a config returned no command; the registry would never load")
	}
}

// helpFor renders the collapsed help line for the current state.
func helpFor(m Model) string {
	return m.help.View(m.contextualKeys())
}

func TestHelpFollowsTheSelection(t *testing.T) {
	tests := []struct {
		name        string
		packageName string
		want        []string
		absent      []string
	}{
		{
			name:        "not installed offers only install",
			packageName: "bat",
			want:        []string{"install"},
			absent:      []string{"update", "remove"},
		},
		{
			name:        "installed and current offers only remove",
			packageName: "fzf",
			want:        []string{"remove"},
			absent:      []string{"install", "update"},
		},
		{
			name:        "installed and behind offers update and remove",
			packageName: "yazi",
			want:        []string{"update", "remove"},
			absent:      []string{"install"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := browseModel(t)
			m = selectPackage(t, m, tt.packageName)

			help := helpFor(m)
			for _, want := range tt.want {
				if !strings.Contains(help, want) {
					t.Errorf("help = %q, want it to offer %q", help, want)
				}
			}
			for _, absent := range tt.absent {
				if strings.Contains(help, absent) {
					t.Errorf("help = %q, want %q left out — it does not apply here", help, absent)
				}
			}
		})
	}
}

func TestHelpFollowsTheFocus(t *testing.T) {
	m := browseModel(t)

	// Selecting and copying belong to the detail pane, filtering to the list.
	list := helpFor(m)
	if strings.Contains(list, "select") || strings.Contains(list, "copy") {
		t.Errorf("help on the list = %q, want the detail-pane keys left out", list)
	}
	if !strings.Contains(list, "filter") {
		t.Errorf("help on the list = %q, want filter offered", list)
	}

	detail := helpFor(applyMsg(t, m, keyMsg("right")))
	if !strings.Contains(detail, "select") || !strings.Contains(detail, "copy") {
		t.Errorf("help on the details = %q, want the selection keys offered", detail)
	}
	if strings.Contains(detail, "filter") {
		t.Errorf("help on the details = %q, want filter left out", detail)
	}
}

func TestHelpWithNothingSelected(t *testing.T) {
	m := browseModel(t)
	m.list.SetItems(nil)

	// An empty list offers no package action at all.
	help := helpFor(m)
	for _, absent := range []string{"install", "update", "remove"} {
		if strings.Contains(help, absent) {
			t.Errorf("help = %q, want %q left out with nothing selected", help, absent)
		}
	}
	if !strings.Contains(help, "quit") {
		t.Errorf("help = %q, want the global keys still offered", help)
	}
}

func TestRealBindingsStayEnabled(t *testing.T) {
	m := browseModel(t)
	m = selectPackage(t, m, "fzf")

	// Only the help view is filtered. Pressing an action that does not apply
	// has to explain itself rather than do nothing.
	if !m.keys.Install.Enabled() {
		t.Error("the install binding was disabled on the model, not just in the help")
	}
	m = applyMsg(t, m, keyMsg("i"))
	if m.status == "" {
		t.Error("pressing i on an installed package said nothing")
	}
}

func TestHelpOffersTheRegistryRefresh(t *testing.T) {
	m := browseModel(t)

	// Refreshing the registry used to exist only in the expanded help, so it
	// was invisible unless you already knew to press ?.
	if !strings.Contains(helpFor(m), "refresh") {
		t.Errorf("collapsed help = %q, want it to offer the registry refresh", helpFor(m))
	}
}

func TestHelpIsNeverCutOff(t *testing.T) {
	// Truncating the help hides whatever is at the end of the line — which is
	// exactly how the refresh key went missing. It wraps instead.
	for _, size := range []tea.WindowSizeMsg{{Width: 60, Height: 24}, {Width: 80, Height: 24}, {Width: 200, Height: 40}} {
		m := browseModel(t)
		m = applyMsg(t, m, size)

		if strings.Contains(m.footer(), "…") {
			t.Errorf("%d columns: the help was truncated:\n%s", size.Width, m.footer())
		}
		for _, line := range strings.Split(m.footer(), "\n") {
			if got := len([]rune(line)); got > m.contentWidth() {
				t.Errorf("%d columns: footer line is %d cells wide, want at most %d: %q",
					size.Width, got, m.contentWidth(), line)
			}
		}
	}
}

func TestNarrowHelpWrapsAndShrinksTheBody(t *testing.T) {
	wide := browseModel(t)
	wide = applyMsg(t, wide, tea.WindowSizeMsg{Width: 200, Height: 30})

	narrow := browseModel(t)
	narrow = applyMsg(t, narrow, tea.WindowSizeMsg{Width: 60, Height: 30})

	// A wrapped help takes an extra row, and layout measures the footer, so the
	// panes give that row up rather than the view overflowing.
	if narrow.listHeight >= wide.listHeight {
		t.Errorf("list height %d at 60 columns vs %d at 200; the wrapped help should shorten the body",
			narrow.listHeight, wide.listHeight)
	}
	if lines := strings.Split(narrow.View(), "\n"); len(lines) > 30 {
		t.Errorf("view is %d lines, want at most 30", len(lines))
	}
}

func TestHelpOffersTheInstallMethodToggle(t *testing.T) {
	m := browseModel(t)

	// The header shows the method but said nothing about how to change it, and
	// the key was only in the expanded help. Both keys are listed, and each says
	// what it will do rather than only that it exists.
	help := helpFor(m)
	for _, want := range []string{"m install as commit", "M global: commit"} {
		if !strings.Contains(help, want) {
			t.Errorf("collapsed help = %q, want it to contain %q", help, want)
		}
	}
}

// TestStartupFrameNeverExceedsTheTerminal is the "title scrolled off on first
// open" regression. Loading the registry grows the help line — a selected
// package brings its action keys — and only the key handlers used to pass
// through relayoutIfNeeded, so the first frame after startup was taller than
// the terminal until the first keystroke fixed it.
func TestStartupFrameNeverExceedsTheTerminal(t *testing.T) {
	packages, installed := samplePackages()

	for _, width := range []int{60, 80, 100, 120} {
		m := New(testConfig(t))
		m = applyMsg(t, m, tea.WindowSizeMsg{Width: width, Height: 24})
		m = applyMsg(t, m, registryLoadedMsg{packages: packages, installed: installed})

		// The budget has to match reality on the very first frame after the
		// load, not after the next keypress.
		if got := lipgloss.Height(m.footer()); got != m.footerHeight {
			t.Errorf("width %d: footer is %d lines but %d were budgeted", width, got, m.footerHeight)
		}
		if got := lipgloss.Height(m.View()); got > 24 {
			t.Errorf("width %d: the frame is %d lines on a 24-line terminal", width, got)
		}
	}
}
