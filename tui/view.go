package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/lvim-tech/clipack/pkg"
)

// View implements tea.Model and renders whichever screen is active.
func (m Model) View() string {
	if m.quitting {
		return ""
	}
	if m.width == 0 {
		return "\n  Starting clipack…\n"
	}

	switch m.screen {
	case screenSetup:
		return m.styles.App.Render(m.viewSetup())
	case screenLoading:
		return m.styles.App.Render(m.viewLoading())
	case screenRun:
		return m.styles.App.Render(m.viewRun())
	case screenConfirm:
		return m.viewConfirm()
	default:
		return m.styles.App.Render(m.viewBrowse())
	}
}

// ---------------------------------------------------------------------------
// Setup
// ---------------------------------------------------------------------------

// viewSetup renders the first-run wizard.
func (m Model) viewSetup() string {
	s := m.styles

	checkbox := "[ ]"
	if m.setupAddShell {
		checkbox = s.OK.Render("[x]")
	}

	body := []string{
		s.Title.Render(" clipack ") + "  " + s.HeaderMeta.Render("first run"),
		"",
		"No configuration was found. Choose where clipack should keep",
		s.Muted.Render("binaries, configs, build trees, man pages and the registry cache."),
		"",
		s.DetailTitle.Render("Installation directory"),
		m.input.View(),
		"",
		fmt.Sprintf("%s  add bin/ and man/ to your shell rc file", checkbox),
		"",
		s.Muted.Render(m.hint("enter confirm", "tab toggle", "esc quit")),
	}

	if m.err != nil {
		body = append(body, "", s.Err.Render("Error: "+m.err.Error()))
	}

	// The wizard runs before any layout has been computed, so every line is
	// wrapped to the terminal width here.
	wrap := lipgloss.NewStyle().Width(m.contentWidth())
	for i, line := range body {
		body[i] = wrap.Render(line)
	}

	return lipgloss.JoinVertical(lipgloss.Left, body...)
}

// ---------------------------------------------------------------------------
// Loading
// ---------------------------------------------------------------------------

// viewLoading renders the spinner shown while the registry is fetched.
func (m Model) viewLoading() string {
	status := m.status
	if status == "" {
		status = "Loading registry…"
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		m.header(),
		"",
		lipgloss.NewStyle().Width(m.contentWidth()).Render("  "+m.spinner.View()+" "+status),
	)
}

// ---------------------------------------------------------------------------
// Browse
// ---------------------------------------------------------------------------

// viewBrowse renders the header, the split list/detail body and the footer.
func (m Model) viewBrowse() string {
	s := m.styles

	if m.err != nil && len(m.packages) == 0 {
		wrap := lipgloss.NewStyle().Width(m.contentWidth())
		return lipgloss.JoinVertical(lipgloss.Left,
			m.header(),
			"",
			s.Err.Render("  Could not load the registry"),
			wrap.Render(s.Muted.Render("  "+m.err.Error())),
			"",
			s.Muted.Render("  press r to retry, q to quit"),
		)
	}

	// The focused pane is the one the movement keys drive, so it carries the
	// accent border; without that the focus is invisible.
	leftStyle, rightStyle := s.Pane, s.PaneFocused
	if m.focus == focusList {
		leftStyle, rightStyle = s.PaneFocused, s.Pane
	}

	// A bordered pane's Width covers content plus horizontal padding, so the
	// sub-model width is padded back out here to keep the borders aligned.
	// The list is given a huge height so bubbles keeps one page; the pane's real
	// height is tracked separately.
	left := leftStyle.
		Width(m.list.Width() + 2).
		Height(m.listHeight).
		Render(m.renderList())

	right := rightStyle.
		Width(m.detail.Width + 2).
		Height(m.detail.Height).
		Render(m.detail.View())

	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	return lipgloss.JoinVertical(lipgloss.Left,
		m.header(),
		body,
		m.footer(),
	)
}

// header renders the title bar, counters and tab strip.
func (m Model) header() string {
	s := m.styles
	sep := s.Icons.Separator

	installedCount := 0
	updateCount := 0
	for _, p := range m.packages {
		if inst, ok := m.installed[p.Name]; ok {
			installedCount++
			if pkg.HasUpdate(p, inst) {
				updateCount++
			}
		}
	}

	// The counters are abbreviated on a narrow terminal rather than truncated
	// mid-word.
	var meta string
	if m.contentWidth() < 64 {
		meta = fmt.Sprintf("%d pkg %s %d inst", len(m.packages), sep, installedCount)
		if updateCount > 0 {
			meta += fmt.Sprintf(" %s %s", sep, s.BadgeUpdate.Render(fmt.Sprintf("%d upd", updateCount)))
		}
	} else {
		meta = fmt.Sprintf("%d packages %s %d installed", len(m.packages), sep, installedCount)
		if updateCount > 0 {
			meta += fmt.Sprintf(" %s %s", sep, s.BadgeUpdate.Render(fmt.Sprintf("%d updates", updateCount)))
		}
		// "global" because m repins a single package in the Installed tab; this
		// value is the default a fresh install uses.
		meta += s.Muted.Render(fmt.Sprintf("  %s  global install method: %s", sep, m.method))
	}

	// Marks are the one piece of state an action reads that is not the row
	// under the cursor, so the count is always on screen.
	if n := m.checkedCount(); n > 0 {
		meta += fmt.Sprintf("  %s %s", sep, s.BadgeInstalled.Render(fmt.Sprintf("%d marked", n)))
	}

	title := lipgloss.JoinHorizontal(lipgloss.Center,
		s.Title.Render(" clipack "),
		"  ",
		s.HeaderMeta.Render(meta),
	)

	// Every tab carries its count, so how much is behind each one is visible
	// without switching to it. They are dropped on a narrow terminal, where the
	// strip would otherwise be clipped and a tab would disappear entirely.
	counts := map[tabID]int{
		tabAll:          len(m.packages),
		tabInstalled:    installedCount,
		tabNotInstalled: len(m.packages) - installedCount,
		tabUpdates:      updateCount,
	}
	showCounts := m.contentWidth() >= 64

	tabs := make([]string, 0, len(tabTitles))
	for i, name := range tabTitles {
		label := name
		if showCounts {
			label = fmt.Sprintf("%s (%d)", name, counts[tabID(i)])
		}
		if tabID(i) == m.tab {
			tabs = append(tabs, s.TabActive.Render(label))
		} else {
			tabs = append(tabs, s.Tab.Render(label))
		}
	}

	// Clipping is the backstop: whatever the counters and tab labels add up to,
	// the header can never be wider than the terminal.
	clip := m.clipStyle()

	lines := []string{
		clip.Render(title),
		clip.Render(lipgloss.JoinHorizontal(lipgloss.Bottom, tabs...)),
	}
	if notice, ok := m.shellNotice(); ok {
		lines = append(lines, clip.Render(notice))
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// shellNotice renders the line that says the shell clipack is running under
// cannot find the packages it installs.
//
// It is deliberately about one shell — the current one. A user who runs clipack
// from zsh and later from fish has two startup files to extend, and each shell
// is told about its own, from inside it, rather than clipack guessing at files
// for shells the user may not even have.
func (m Model) shellNotice() (string, bool) {
	if !m.shellKnown || m.config == nil {
		return "", false
	}

	s := m.styles
	name := m.shellStatus.Shell.Name
	narrow := m.contentWidth() < 72

	switch {
	case m.shellStatus.NeedsPaths():
		if narrow {
			return s.Err.Render(fmt.Sprintf("%s %s: press p to add bin to PATH", s.Icons.Warn, name)), true
		}
		return s.Err.Render(fmt.Sprintf("%s %s cannot find %s — press p to add it to %s",
			s.Icons.Warn, name, m.config.Paths.Bin, m.shellStatus.RCFile)), true

	case m.shellStatus.NeedsRestart():
		// The file is already right and only this session is behind. Saying so
		// in muted text rather than red is the difference between "you have
		// something to do" and "you did it, restart the shell" — without it the
		// warning looks like the key did nothing.
		if narrow {
			return s.Muted.Render(fmt.Sprintf("%s restart %s to pick up bin", s.Icons.Bullet, name)), true
		}
		return s.Muted.Render(fmt.Sprintf("%s %s references %s — restart %s to pick it up",
			s.Icons.Bullet, m.shellStatus.RCFile, m.config.Paths.Bin, name)), true
	}

	return "", false
}

// footer renders the status line and the help line.
func (m Model) footer() string {
	status := m.styles.Muted.Render(m.status)
	if m.err != nil {
		status = m.styles.Err.Render(m.err.Error())
	}

	// The help wraps rather than being cut at the right edge: a key the user
	// cannot see is a key they do not have, and truncation hid the tail of the
	// line — which is how the registry refresh went missing. layout measures
	// this footer, so a second line simply shortens the body.
	return lipgloss.JoinVertical(lipgloss.Left,
		m.clipStyle().Render(status),
		lipgloss.NewStyle().Width(m.contentWidth()).Render(m.help.View(m.contextualKeys())),
	)
}

// hint joins key hints with the theme's separator glyph.
func (m Model) hint(parts ...string) string {
	return strings.Join(parts, "  "+m.styles.Icons.Bullet+"  ")
}

// contentWidth is the width available inside the app's own padding.
func (m Model) contentWidth() int {
	width := m.width - appPadding
	if width < 1 {
		width = 1
	}
	return width
}

// clipStyle truncates a rendered line to the usable width. It is ANSI-aware, so
// styled text is cut at the right visual column.
func (m Model) clipStyle() lipgloss.Style {
	return lipgloss.NewStyle().MaxWidth(m.contentWidth())
}

// ---------------------------------------------------------------------------
// Confirm
// ---------------------------------------------------------------------------

// viewConfirm draws the confirmation modal centred over the browse screen.
func (m Model) viewConfirm() string {
	s := m.styles
	entry := m.pendingItem
	verb := m.pending.label()

	// Keep the dialog inside the terminal: a long build path used to push the
	// box past the right edge and break the border.
	maxWidth := m.width - 12
	if maxWidth < 30 {
		maxWidth = 30
	}
	wrap := lipgloss.NewStyle().Width(maxWidth)

	if len(m.pendingBatch) > 0 {
		return m.viewConfirmBatch(wrap)
	}

	title := verb + " " + entry.pkg.Name + "?"
	if m.pending == actionSwitchMethod {
		title = fmt.Sprintf("Switch %s to %s?", entry.pkg.Name, m.pendingMethod)
	}

	lines := []string{
		s.DialogTitle.Render(title),
		"",
	}

	switch m.pending {
	case actionInstall:
		lines = append(lines,
			s.Muted.Render("method  ")+m.method,
			s.Muted.Render("ref     ")+entry.pkg.Ref(m.method),
			"",
			s.Muted.Render("The package is built from source in"),
			wrap.Render(s.Muted.Render(m.config.Paths.Build)),
		)
	case actionUpdate:
		method := installedMethod(entry, m.method)
		lines = append(lines,
			s.Muted.Render("installed  ")+entry.installed.Ref(method),
			s.Muted.Render("available  ")+entry.pkg.Ref(method),
			"",
			wrap.Render(s.Muted.Render("Binaries, configs, man pages"+
				desktopClause(entry.pkg, entry.installed)+
				resourceClause(entry.pkg, entry.installed)+" are replaced.")),
		)
	case actionRemove:
		lines = append(lines,
			wrap.Render(s.Muted.Render("Removes binaries, man pages"+
				desktopClause(entry.installed)+resourceClause(entry.installed)+
				" and the config directory for this package.")),
		)
	case actionSwitchMethod:
		from := installedMethod(entry, m.method)
		lines = append(lines,
			s.Muted.Render("now     ")+fmt.Sprintf("%-8s %s", from, entry.installed.Ref(from)),
			s.Muted.Render("after   ")+fmt.Sprintf("%-8s %s", m.pendingMethod, entry.pkg.Ref(m.pendingMethod)),
			"",
			wrap.Render(s.Muted.Render(fmt.Sprintf(
				"Switching pins rebuilds %s from source and replaces its binaries and man pages.",
				entry.pkg.Name))),
			"",
			// The configuration is the only part of this that cannot be
			// rebuilt, so it is spelled out rather than left to "reinstall".
			wrap.Render(s.Muted.Render(
				"Its configuration directory is deleted and recreated — any changes you made there are lost:")),
			wrap.Render(s.Muted.Render(filepath.Join(m.config.Paths.Configs, entry.pkg.Name))),
		)
	}

	lines = append(lines, "", s.Muted.Render(m.hint("y confirm", "esc cancel")))

	dialog := s.Dialog.MaxWidth(m.width - 2).
		Render(lipgloss.JoinVertical(lipgloss.Left, lines...))

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, dialog)
}

// maxBatchListed is how many packages the confirmation spells out before it
// falls back to a count. A dialog taller than the terminal helps nobody.
const maxBatchListed = 12

// viewConfirmBatch draws the confirmation for an operation on marked packages.
// It names them rather than only counting: a batch remove is the most
// destructive thing clipack does, and "Remove 7 packages?" is not enough to
// decide on.
func (m Model) viewConfirmBatch(wrap lipgloss.Style) string {
	s := m.styles
	verb := m.pending.label()

	lines := []string{
		s.DialogTitle.Render(fmt.Sprintf("%s %d package(s)?", verb, len(m.pendingBatch))),
		"",
	}

	shown := m.pendingBatch
	if len(shown) > maxBatchListed {
		shown = shown[:maxBatchListed]
	}
	for _, entry := range shown {
		lines = append(lines, "  "+m.batchLine(entry))
	}
	if rest := len(m.pendingBatch) - len(shown); rest > 0 {
		lines = append(lines, s.Muted.Render(fmt.Sprintf("  … and %d more", rest)))
	}

	if len(m.pendingSkipped) > 0 {
		lines = append(lines,
			"",
			wrap.Render(s.Muted.Render(fmt.Sprintf("%d skipped: %s",
				len(m.pendingSkipped), strings.Join(m.pendingSkipped, ", ")))),
		)
	}

	if m.pending == actionInstall {
		lines = append(lines, "", s.Muted.Render("Built from source, one after another."))
	}

	lines = append(lines, "", s.Muted.Render(m.hint("y confirm", "esc cancel")))

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
		s.Dialog.MaxWidth(m.width-2).Render(lipgloss.JoinVertical(lipgloss.Left, lines...)))
}

// batchLine describes one package in the confirmation, showing the change the
// action will make where there is one.
func (m Model) batchLine(entry packageItem) string {
	s := m.styles
	name := lipgloss.NewStyle().Width(18).Render(entry.pkg.Name)

	switch m.pending {
	case actionUpdate:
		method := installedMethod(entry, m.method)
		return name + s.Muted.Render(entry.installed.Ref(method)+" → ") + entry.pkg.Ref(method)
	case actionInstall:
		return name + s.Muted.Render(entry.pkg.Ref(m.method))
	default:
		return name
	}
}

// ---------------------------------------------------------------------------
// Run
// ---------------------------------------------------------------------------

// viewRun renders the streaming operation log.
func (m Model) viewRun() string {
	s := m.styles
	verb := m.runAction.label()

	var indicator string
	switch {
	case !m.runDone:
		indicator = m.spinner.View() + " " + fmt.Sprintf("%sing %s…", strings.TrimSuffix(verb, "e"), m.runTarget)
	case m.runFailed:
		indicator = s.Err.Render(fmt.Sprintf("%s %s of %s failed", s.Icons.Error, verb, m.runTarget))
	default:
		indicator = s.OK.Render(fmt.Sprintf("%s %s of %s finished", s.Icons.Done, verb, m.runTarget))
	}

	// The keys are worth spelling out here: a failed build is exactly when
	// someone wants the text, and a copy key nobody knows about is no key.
	parts := []string{"↑/↓ scroll", "y copy all", "v/V select"}
	if m.visual != visualNone {
		parts = []string{"hjkl move", "y copy selection", "esc cancel"}
	} else if m.runDone {
		parts = append(parts, "esc back to list")
	}
	hint := s.Muted.Render(m.hint(parts...))

	clip := m.clipStyle()

	return lipgloss.JoinVertical(lipgloss.Left,
		clip.Render(lipgloss.JoinHorizontal(lipgloss.Center, s.Title.Render(" clipack "), "  ", indicator)),
		"",
		s.Pane.Width(m.logView.Width+2).Height(m.logView.Height).Render(m.logView.View()),
		clip.Render(hint),
	)
}

// desktopClause mentions the menu entry when the package has one, so a remove
// says it will take the program out of the application menu instead of leaving
// that to be discovered afterwards.
//
// It takes several packages for the same reason resourceClause does: an update
// reads one manifest and writes another.
func desktopClause(packages ...*pkg.Package) string {
	for _, p := range packages {
		if p != nil && len(p.Install.Desktop) > 0 {
			return ", the menu entry"
		}
	}
	return ""
}

// resourceClause names the directories an install put outside bin/, configs/
// and man/, so the confirmation says what it will actually touch instead of
// listing three categories and quietly meaning four.
//
// It takes several packages because update reads one manifest and writes
// another: a tree the installed version owns still gets removed even when the
// incoming version has dropped it.
func resourceClause(packages ...*pkg.Package) string {
	var targets []string
	seen := map[string]bool{}
	for _, p := range packages {
		if p == nil {
			continue
		}
		for _, res := range p.Install.Resources {
			if seen[res.Target] {
				continue
			}
			seen[res.Target] = true
			targets = append(targets, res.Target)
		}
	}
	if len(targets) == 0 {
		return ""
	}
	sort.Strings(targets)
	return " and " + strings.Join(targets, ", ")
}
