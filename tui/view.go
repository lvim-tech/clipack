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

	var frame string
	switch m.screen {
	case screenSetup:
		frame = m.styles.App.Render(m.viewSetup())
	case screenLoading:
		frame = m.styles.App.Render(m.viewLoading())
	case screenRun:
		frame = m.styles.App.Render(m.viewRun())
	case screenConfirm:
		frame = m.viewConfirm()
	default:
		frame = m.styles.App.Render(m.viewBrowse())
	}

	// The backstop: a frame taller than the terminal scrolls the top line off,
	// and the title never comes back until something re-lays-out. Sizing is
	// layout()'s job and relayoutIfNeeded keeps it honest per message — but any
	// transient state between the two must degrade to a clipped bottom line,
	// never to a lost title.
	return clampHeight(frame, m.height)
}

// clampHeight cuts a rendered frame to at most max lines, keeping the top.
func clampHeight(frame string, max int) string {
	if max <= 0 {
		return frame
	}
	lines := strings.Split(frame, "\n")
	if len(lines) <= max {
		return frame
	}
	return strings.Join(lines[:max], "\n")
}

// stickyFrame composes a full-height frame: the header at the top, the footer
// glued to the bottom edge of the terminal, and the body in the space between.
// The body is clipped and padded to exactly that space, so only it ever
// scrolls — the footer can neither be pushed off screen by a tall body nor
// float up under a short one.
func (m Model) stickyFrame(header, body, footer string) string {
	bodyHeight := m.height - lipgloss.Height(header) - lipgloss.Height(footer)
	if bodyHeight < 1 {
		// A terminal shorter than the chrome itself: degrade to a top-anchored
		// stack and let View's clamp cut the bottom line.
		return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
	}

	lines := strings.Split(body, "\n")
	if len(lines) > bodyHeight {
		lines = lines[:bodyHeight]
	}
	for len(lines) < bodyHeight {
		lines = append(lines, "")
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, strings.Join(lines, "\n"), footer)
}

// ---------------------------------------------------------------------------
// Setup
// ---------------------------------------------------------------------------

// viewSetup renders the first-run wizard. The hint line is a footer: it stays
// on the bottom edge of the terminal, like everywhere else in the interface.
func (m Model) viewSetup() string {
	s := m.styles

	checkbox := "[ ]"
	if m.setupAddShell {
		checkbox = s.OK.Render("[x]")
	}

	header := m.clipStyle().Render(
		s.Title.Render(" clipack ") + "  " + s.HeaderMeta.Render("first run"))

	body := []string{
		"",
	}
	if m.setupStep == 0 {
		body = append(body,
			"No configuration was found. Choose where clipack should keep",
			s.Muted.Render("binaries, configs, build trees, man pages and the registry cache."),
			"",
			s.DetailTitle.Render("Installation directory"),
			m.input.View(),
		)
	} else {
		body = append(body,
			"clipack ships no registry — it is the tool, not the content.",
			s.Muted.Render("A registry is a git repository with an index.yaml listing packages."),
			s.Muted.Render("For a private one, add a token to the config file afterwards."),
			"",
			s.DetailTitle.Render("Registry URL"),
			m.input.View(),
		)
	}
	body = append(body,
		"",
		fmt.Sprintf("%s  add bin/ and man/ to your shell rc file", checkbox),
	)

	if m.err != nil {
		body = append(body, "", s.Err.Render("Error: "+m.err.Error()))
	}

	// The wizard runs before any layout has been computed, so every line is
	// wrapped to the terminal width here.
	wrap := lipgloss.NewStyle().Width(m.contentWidth())
	for i, line := range body {
		body[i] = wrap.Render(line)
	}

	footer := m.clipStyle().Render(m.hint("enter confirm", "tab toggle", "esc quit"))

	return m.stickyFrame(header, lipgloss.JoinVertical(lipgloss.Left, body...), footer)
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
		body := lipgloss.JoinVertical(lipgloss.Left,
			"",
			s.Err.Render("  Could not load the registry"),
			wrap.Render(s.Muted.Render("  "+m.err.Error())),
		)
		return m.stickyFrame(m.header(), body,
			m.clipStyle().Render(m.hint("r retry", "q quit")))
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

	// The footer is pinned to the bottom edge; only the panes between the
	// header and it ever scroll.
	return m.stickyFrame(m.header(), body, m.footer())
}

// header renders the title bar, counters and tab strip.
func (m Model) header() string {
	s := m.styles
	sep := s.Icons.Separator

	// Counts follow the active category, so the tab labels agree with what
	// the list actually shows: "Installed (21)" over a list of 3 terminals
	// would be a header describing a different screen.
	total := 0
	installedCount := 0
	updateCount := 0
	for _, p := range m.packages {
		if m.category != "" && p.Category != m.category {
			continue
		}
		total++
		if inst, ok := m.installed[p.Name]; ok {
			installedCount++
			if pkg.HasUpdate(p, inst) {
				updateCount++
			}
		}
	}

	// The counters are abbreviated on a narrow terminal rather than truncated
	// mid-word.
	shown := "all"
	if m.category != "" {
		shown = m.category
	}

	var meta string
	if m.contentWidth() < 64 {
		meta = fmt.Sprintf("%d pkg %s %d inst", total, sep, installedCount)
		if updateCount > 0 {
			meta += fmt.Sprintf(" %s %s", sep, s.BadgeUpdate.Render(fmt.Sprintf("%d upd", updateCount)))
		}
	} else {
		meta = fmt.Sprintf("%d packages %s %d installed", total, sep, installedCount)
		if updateCount > 0 {
			meta += fmt.Sprintf(" %s %s", sep, s.BadgeUpdate.Render(fmt.Sprintf("%d updates", updateCount)))
		}
		// "global" because m repins a single package in the Installed tab; this
		// value is the default a fresh install uses.
		meta += s.Muted.Render(fmt.Sprintf("  %s  global install method: %s", sep, m.method))
		meta += s.Muted.Render(fmt.Sprintf("  %s  category: ", sep)) + s.BadgeInstalled.Render(shown)
	}

	// Marks are the one piece of state an action reads that is not the row
	// under the cursor, so the count is always on screen.
	if n := m.checkedCount(); n > 0 {
		meta += fmt.Sprintf("  %s %s", sep, s.BadgeInstalled.Render(fmt.Sprintf("%d marked", n)))
	}

	// The top bar: the title chip on the left, the tab buttons right beside it.
	counts := map[tabID]int{
		tabAll:          total,
		tabInstalled:    installedCount,
		tabNotInstalled: total - installedCount,
		tabUpdates:      updateCount,
	}
	chip := s.Title.Render(" clipack ")
	topBar := chip + " " + m.tabStrip(m.contentWidth()-lipgloss.Width(chip)-1, counts)

	// Clipping is the backstop: whatever the counters and tab labels add up to,
	// the header can never be wider than the terminal.
	clip := m.clipStyle()

	lines := []string{
		clip.Render(topBar),
		clip.Render(s.HeaderMeta.Render(meta)),
	}
	if notice, ok := m.shellNotice(); ok {
		lines = append(lines, clip.Render(notice))
	}

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// tabStrip draws the tab buttons, narrowing them until they fit rather than
// letting the strip run off the right edge — a tab that is clipped away is a
// tab the user does not know exists.
//
// Three widths, tried in order: full buttons, each carrying its count; the
// bare titles with the padding taken away; and finally only the tab you are
// on, with its position in the strip and the key that moves it.
func (m Model) tabStrip(avail int, counts map[tabID]int) string {
	s := m.styles

	full := make([]string, 0, len(tabTitles))
	tight := make([]string, 0, len(tabTitles))
	for i, name := range tabTitles {
		label := fmt.Sprintf("%s (%d)", name, counts[tabID(i)])
		if tabID(i) == m.tab {
			full = append(full, s.TabActive.Render(label))
			// The active tab keeps its button shape in the tight tier: the
			// background is what marks where you are.
			tight = append(tight, s.TabActive.Render(name))
		} else {
			full = append(full, s.Tab.Render(label))
			tight = append(tight, s.TabTight.Render(name))
		}
	}

	if strip := strings.Join(full, ""); lipgloss.Width(strip) <= avail {
		return strip
	}
	if strip := strings.Join(tight, " "); lipgloss.Width(strip) <= avail {
		return strip
	}

	// Nothing else fits. Naming where you are beats a strip cut off at an
	// arbitrary point, which reads as though the missing tabs do not exist.
	return s.TabActive.Render(tabTitles[m.tab]) +
		s.Muted.Render(fmt.Sprintf("  %d/%d  tab moves", int(m.tab)+1, len(tabTitles)))
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
		m.helpView(),
	)
}

// helpView renders the contextual key hints as "[key] description" buttons,
// wrapped on pair boundaries so no key is ever cut off at the right edge.
func (m Model) helpView() string {
	keys := m.contextualKeys()
	bindings := keys.ShortHelp()
	if m.showFullHelp {
		bindings = bindings[:0:0]
		for _, group := range keys.FullHelp() {
			bindings = append(bindings, group...)
		}
	}

	pairs := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		if !binding.Enabled() {
			continue
		}
		h := binding.Help()
		pairs = append(pairs, m.hintPair(h.Key, h.Desc))
	}
	return wrapHints(pairs, m.contentWidth())
}

// hintPair renders one "[key] label" button: the key bracketed in the accent
// colour, the description as plain muted text beside it.
func (m Model) hintPair(key, label string) string {
	return m.styles.KeyHint.Render("["+key+"]") + " " + m.styles.Muted.Render(label)
}

// wrapHints lays hint pairs into lines of at most width columns, breaking only
// between pairs: a hint split mid-pair separates a key from what it does.
func wrapHints(pairs []string, width int) string {
	var lines []string
	current, currentWidth := "", 0
	for _, pair := range pairs {
		w := lipgloss.Width(pair)
		switch {
		case current == "":
			current, currentWidth = pair, w
		case currentWidth+2+w <= width:
			current += "  " + pair
			currentWidth += 2 + w
		default:
			lines = append(lines, current)
			current, currentWidth = pair, w
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return strings.Join(lines, "\n")
}

// hint renders inline key hints — "esc cancel" becomes "[esc] cancel" — in the
// same form the footer uses, so every hint in the interface reads the same.
func (m Model) hint(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		key, label, found := strings.Cut(part, " ")
		if !found {
			out = append(out, m.styles.Muted.Render(part))
			continue
		}
		out = append(out, m.hintPair(key, label))
	}
	return strings.Join(out, "  ")
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
		method := m.methodOf(entry.pkg.Name)
		lines = append(lines,
			s.Muted.Render("method  ")+method,
			s.Muted.Render("ref     ")+entry.pkg.Ref(method),
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
	case actionReinstall:
		method := installedMethod(entry, m.method)
		lines = append(lines,
			s.Muted.Render("method  ")+method,
			s.Muted.Render("ref     ")+entry.installed.Ref(method),
			"",
			wrap.Render(s.Muted.Render(fmt.Sprintf(
				"Rebuilds %s from source at the ref it is already on. Nothing about the version changes — "+
					"this picks up a registry entry that changed without it.", entry.pkg.Name))),
			"",
			wrap.Render(s.Muted.Render("Binaries, man pages"+
				desktopClause(entry.pkg, entry.installed)+
				resourceClause(entry.pkg, entry.installed)+" are replaced.")),
			"",
			// Same warning the switch carries, for the same reason: this is the
			// one part of the operation that cannot be rebuilt.
			wrap.Render(s.Muted.Render(
				"Its configuration directory is deleted and recreated — any changes you made there are lost:")),
			wrap.Render(s.Muted.Render(filepath.Join(m.config.Paths.Configs, entry.pkg.Name))),
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

	lines = append(lines, "", m.hint("y confirm", "esc cancel"))

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
	if m.pending == actionReinstall {
		lines = append(lines, "",
			wrap.Render(s.Muted.Render("Each is rebuilt from source at the ref it is already on. "+
				"Configuration directories are deleted and recreated — changes made there are lost.")))
	}

	lines = append(lines, "", m.hint("y confirm", "esc cancel"))

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
		// Each package's own method, because a batch can mix them: the ref shown
		// has to be the one that package will actually be built from.
		method := m.methodOf(entry.pkg.Name)
		return name + s.Muted.Render(method+" "+entry.pkg.Ref(method))
	default:
		return name
	}
}

// ---------------------------------------------------------------------------
// Run
// ---------------------------------------------------------------------------

// viewRun renders the streaming operation log between the run header and the
// hint line pinned to the bottom edge; only the log pane scrolls.
func (m Model) viewRun() string {
	body := m.styles.Pane.
		Width(m.logView.Width + 2).
		Height(m.logView.Height).
		Render(m.logView.View())

	return m.stickyFrame(m.runHeader(), body, m.runFooter())
}

// runHeader renders the title chip and the live indicator of the running
// operation, plus the blank line that separates them from the log.
func (m Model) runHeader() string {
	s := m.styles
	verb := m.runAction.label()

	var indicator string
	switch {
	case !m.runDone:
		// The live line: which package, where in the batch, which step, what
		// command. All of it comes from events the log already receives — this
		// is the same information, put where the eye rests instead of where
		// the log happens to have scrolled.
		subject := m.runTarget
		if m.runCurrent != "" {
			subject = m.runCurrent
			if m.runTotal > 1 {
				subject += fmt.Sprintf(" (%d/%d)", m.runIndex, m.runTotal)
			}
		}
		progress := ""
		if m.runStepOf > 0 {
			progress = fmt.Sprintf(" · step %d/%d", m.runStep, m.runStepOf)
			if m.runStepText != "" {
				// One line only: the first line of a multi-line step is the
				// command, the rest is the comment block above it.
				text := m.runStepText
				if i := strings.IndexByte(text, '\n'); i >= 0 {
					text = text[:i]
				}
				progress += ": " + text
			}
		}
		indicator = m.spinner.View() + " " + fmt.Sprintf("%sing %s…%s", strings.TrimSuffix(verb, "e"), subject, progress)
	case m.runFailed:
		indicator = s.Err.Render(fmt.Sprintf("%s %s of %s failed", s.Icons.Error, verb, m.runTarget))
	default:
		indicator = s.OK.Render(fmt.Sprintf("%s %s of %s finished", s.Icons.Done, verb, m.runTarget))
	}

	return lipgloss.JoinVertical(lipgloss.Left,
		m.clipStyle().Render(lipgloss.JoinHorizontal(lipgloss.Center, s.Title.Render(" clipack "), "  ", indicator)),
		"",
	)
}

// runFooter renders the run screen's hint line. The keys are worth spelling
// out here: a failed build is exactly when someone wants the text, and a copy
// key nobody knows about is no key.
func (m Model) runFooter() string {
	parts := []string{"↑/↓ scroll", "y copy all", "v/V select"}
	if m.visual != visualNone {
		parts = []string{"hjkl move", "y copy selection", "esc cancel"}
	} else if m.runDone {
		parts = append(parts, "esc back to list")
	}
	return m.clipStyle().Render(m.hint(parts...))
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
