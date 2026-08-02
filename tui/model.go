package tui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lvim-tech/clipack/cnfg"
	"github.com/lvim-tech/clipack/pkg"
)

// screen identifies which view the root model is currently rendering.
type screen int

const (
	screenSetup screen = iota
	screenLoading
	screenBrowse
	screenConfirm
	screenRun
)

// focusArea identifies which pane of the browse screen has the keyboard.
type focusArea int

const (
	focusList focusArea = iota
	focusDetail
)

// tabID identifies the active filter tab in the browse screen.
type tabID int

const (
	tabAll tabID = iota
	tabInstalled
	tabNotInstalled
	tabUpdates
)

// tabTitles labels the tabs in display order.
var tabTitles = []string{"All", "Installed", "Not installed", "Updates"}

// action is the kind of operation awaiting confirmation.
type action int

const (
	actionNone action = iota
	actionInstall
	actionUpdate
	actionRemove
	// actionSwitchMethod repins an installed package to the other ref. It runs
	// the same rebuild an update does, just with a method the manifest does not
	// already record.
	actionSwitchMethod
)

// label renders the action as a verb for the confirmation dialog.
func (a action) label() string {
	switch a {
	case actionInstall:
		return "Install"
	case actionUpdate:
		return "Update"
	case actionRemove:
		return "Remove"
	case actionSwitchMethod:
		return "Switch"
	default:
		return ""
	}
}

// past renders the action for a sentence about something already done.
// Spelled out rather than derived: "install" + "d" gives "installd".
func (a action) past() string {
	switch a {
	case actionInstall:
		return "installed"
	case actionUpdate:
		return "updated"
	case actionRemove:
		return "removed"
	case actionSwitchMethod:
		return "switched"
	default:
		return ""
	}
}

// otherMethod is the install method a package is not currently pinned to.
func otherMethod(method string) string {
	if method == pkg.MethodCommit {
		return pkg.MethodVersion
	}
	return pkg.MethodCommit
}

// Model is the root Bubble Tea model for clipack.
type Model struct {
	config *cnfg.Config
	keys   keyMap
	help   help.Model

	// styles is compiled once from the configured theme. Everything the model
	// renders goes through it, so the whole interface follows config.yaml.
	styles Styles

	// Sub-models.
	list    list.Model
	detail  viewport.Model
	logView viewport.Model
	spinner spinner.Model
	input   textinput.Model

	// Screen state.
	screen screen
	tab    tabID
	focus  focusArea

	// Data.
	packages  []*pkg.Package
	installed map[string]*pkg.Package
	// broken names the installed packages whose manifest describes artifacts
	// that are no longer on disk. Computed when `installed` is loaded rather
	// than while drawing: the list re-renders on every keystroke, and this
	// costs a stat per binary.
	broken map[string]bool
	method string

	// checked holds the packages picked for a batch operation, keyed by name so
	// the marks survive scrolling and re-sorting. lastFilter is what the filter
	// said when the marks were last valid: changing the tab or the filter
	// changes what is on screen, and marks you can no longer see are marks you
	// have forgotten about, so both clear the selection.
	checked    map[string]bool
	lastFilter string

	// Pending confirmation. pendingBatch is what the action will actually run
	// on; pendingSkipped names the marked packages it does not apply to.
	pending        action
	pendingItem    packageItem
	pendingBatch   []packageItem
	pendingSkipped []string
	// pendingMethod is the method a switch will repin to. Empty for every other
	// action, which use the global toggle or the manifest.
	pendingMethod string

	// Running operation.
	stream    *opStream
	logLines  []string
	runAction action
	runTarget string
	runFailed bool
	runDone   bool

	// Setup wizard.
	setupAddShell bool

	// shellStatus is where the shell clipack was started from stands with
	// respect to the managed bin directory, and shellKnown whether that could be
	// determined at all — a shell clipack cannot write to gets no warning,
	// because there would be no key to fix it with. Filled in by a command after
	// startup, so a model built in a test says nothing about the machine it runs
	// on until it is told.
	shellStatus cnfg.ShellStatus
	shellKnown  bool

	// listOffset is the index of the first entry drawn in the list pane, and
	// listHeight the pane's inner height. Entries have individual heights, so
	// the scroll position is tracked here rather than by bubbles' paginator.
	listOffset int
	listHeight int

	// detailFor is the package currently rendered in the detail pane.
	detailFor string

	// footerHeight is the height layout() last budgeted for the footer. The
	// help line lists only the keys that apply right now, so it grows and
	// shrinks as the tab, the focus and the marks change — and when it grows
	// past what the body was sized for, the whole view is one line too tall and
	// the terminal scrolls the title off the top. Update compares against this
	// after every message rather than relying on each of those call sites to
	// remember to re-lay-out, which is exactly what went wrong before.
	footerHeight int

	// Detail pane text cursor and vi-style selection.
	detailBuf detailBuffer
	// logBuf is the build log, addressable line by line so the same selection
	// code that serves the detail pane can serve the run screen.
	logBuf detailBuffer
	cursor pos
	anchor pos
	visual visualMode

	// Chrome.
	width, height int
	status        string
	err           error
	showFullHelp  bool
	quitting      bool
}

// New builds the root model. A nil config means no configuration exists yet and
// the first-run wizard is shown instead of the package browser.
func New(config *cnfg.Config) Model {
	styles, err := themeFor(config)
	return NewWithStyles(config, styles, err)
}

// themeFor compiles the styles a configuration asks for, falling back to the
// default theme when the configured one cannot be resolved. A broken theme must
// not stop clipack from running; the reason is surfaced in the status line.
func themeFor(config *cnfg.Config) (Styles, error) {
	if config == nil {
		return DefaultStyles(), nil
	}

	theme, err := cnfg.ResolveTheme(config.Theme)
	if err != nil {
		return DefaultStyles(), err
	}
	return NewStyles(theme), nil
}

// NewWithStyles builds the root model with an explicit style set. It is the
// seam the tests use to render an arbitrary theme.
func NewWithStyles(config *cnfg.Config, styles Styles, themeErr error) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = styles.Cursor

	in := textinput.New()
	in.Placeholder = cnfg.DefaultInstallDir()
	in.SetValue(cnfg.DefaultInstallDir())
	in.Prompt = "› "
	in.PromptStyle = styles.Prompt
	in.CharLimit = 512

	m := Model{
		config:        config,
		keys:          defaultKeys(),
		help:          help.New(),
		styles:        styles,
		list:          newPackageList(styles),
		detail:        viewport.New(0, 0),
		logView:       viewport.New(0, 0),
		spinner:       sp,
		input:         in,
		method:        pkg.MethodVersion,
		setupAddShell: true,
		checked:       map[string]bool{},
	}

	if config == nil {
		m.screen = screenSetup
		m.input.Focus()
	} else {
		m.screen = screenLoading
		m.method = config.Options.InstallMethod
		if m.method == "" {
			m.method = pkg.MethodVersion
		}
	}

	// A theme that could not be resolved is reported rather than rendered as a
	// silent fallback, so a typo in config.yaml is visible.
	if themeErr != nil {
		m.status = "theme: " + themeErr.Error()
	}

	return m
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	if m.screen == screenSetup {
		return tea.Batch(textinput.Blink, m.spinner.Tick)
	}
	return tea.Batch(m.spinner.Tick, loadRegistryCmd(m.config, false), checkShellPathCmd(m.config))
}

// Update implements tea.Model. It dispatches to a per-screen handler after
// processing messages that apply everywhere.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return m, nil

	case spinner.TickMsg:
		// Only keep the spinner animating while something is in flight.
		if m.screen == screenLoading || (m.screen == screenRun && !m.runDone) {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case setupDoneMsg:
		return m.handleSetupDone(msg)

	case shellPathMsg:
		return m.handleShellPath(msg)

	case registryLoadedMsg:
		return m.handleRegistryLoaded(msg)

	case opEventsMsg:
		return m.handleOpEvents(msg)

	case opFinishedMsg:
		return m.handleOpFinished(msg)

	case tea.KeyMsg:
		// Ctrl+C always quits, whatever the screen or focus.
		if msg.String() == "ctrl+c" {
			m.quitting = true
			return m, tea.Quit
		}
	}

	switch m.screen {
	case screenSetup:
		return m.updateSetup(msg)
	case screenBrowse:
		return relayoutIfNeeded(m.updateBrowse(msg))
	case screenConfirm:
		return relayoutIfNeeded(m.updateConfirm(msg))
	case screenRun:
		return relayoutIfNeeded(m.updateRun(msg))
	}

	return m, nil
}

// relayoutIfNeeded re-runs the layout when the footer no longer occupies the
// height it was budgeted. It wraps the per-screen handlers so that any state
// change reaching the help line is accounted for, whether or not whoever wrote
// that state change thought about the layout.
func relayoutIfNeeded(model tea.Model, cmd tea.Cmd) (tea.Model, tea.Cmd) {
	m, ok := model.(Model)
	if !ok || m.width == 0 || m.height == 0 {
		return model, cmd
	}
	if lipgloss.Height(m.footer()) != m.footerHeight {
		m.layout()
	}
	return m, cmd
}

// ---------------------------------------------------------------------------
// Layout
// ---------------------------------------------------------------------------

// paneFrame is the number of columns and rows a bordered pane adds around its
// content: one border on each side plus one column of horizontal padding.
const (
	paneFrameWidth  = 4 // left border + left padding + right padding + right border
	paneFrameHeight = 2 // top and bottom border
	appPadding      = 2 // styleApp adds one column on each side

	// listSinglePageHeight is handed to the bubbles list so its paginator puts
	// every item on one page. clipack scrolls the list itself, and a single page
	// is what makes Index() count linearly instead of resetting per page.
	listSinglePageHeight = 1 << 16
)

// layout recomputes sub-model dimensions after a resize.
//
// Header and footer are measured rather than assumed: the footer grows when the
// expanded help is toggled, and a hardcoded guess pushed the header off-screen.
func (m *Model) layout() {
	if m.width == 0 || m.height == 0 {
		return
	}

	available := m.width - appPadding
	if available < 20 {
		available = 20
	}
	// Zero means "do not truncate": footer() wraps the help itself, so nothing
	// falls off the right edge.
	m.help.Width = 0

	m.footerHeight = lipgloss.Height(m.footer())

	bodyHeight := m.height -
		lipgloss.Height(m.header()) -
		m.footerHeight -
		paneFrameHeight
	if bodyHeight < 4 {
		bodyHeight = 4
	}

	// Split the row into two bordered panes that together fill the width.
	leftTotal := available * 42 / 100
	if leftTotal < 32 {
		leftTotal = 32
	}
	if leftTotal > available-32 {
		leftTotal = available - 32
	}
	if leftTotal < paneFrameWidth+8 {
		leftTotal = paneFrameWidth + 8
	}

	listWidth := leftTotal - paneFrameWidth
	detailWidth := available - leftTotal - paneFrameWidth
	if detailWidth < 12 {
		detailWidth = 12
	}

	// The pane height is tracked separately: the list is given a large height
	// so bubbles keeps every item on one page, which is what makes Index() move
	// linearly. Scrolling and the visible window are handled by renderList.
	m.listHeight = bodyHeight
	m.list.SetSize(listWidth, listSinglePageHeight)
	m.ensureListVisible()

	m.detail.Width = detailWidth
	m.detail.Height = bodyHeight
	// The content is wrapped to the pane width, so a resize must re-render it.
	m.invalidateDetail()
	m.refreshDetail()

	m.logView.Width = available - paneFrameWidth
	m.logView.Height = bodyHeight
}

// ---------------------------------------------------------------------------
// Setup screen
// ---------------------------------------------------------------------------

// updateSetup handles the first-run wizard.
func (m Model) updateSetup(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "esc":
			m.quitting = true
			return m, tea.Quit
		case "tab", " ":
			m.setupAddShell = !m.setupAddShell
			return m, nil
		case "enter":
			dir := strings.TrimSpace(m.input.Value())
			if dir == "" {
				dir = cnfg.DefaultInstallDir()
			}
			m.screen = screenLoading
			m.status = "Creating configuration…"
			return m, tea.Batch(m.spinner.Tick, saveConfigCmd(dir, m.setupAddShell))
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// handleSetupDone moves on to loading the registry once the config is written.
func (m Model) handleSetupDone(msg setupDoneMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.err = msg.err
		m.screen = screenSetup
		m.input.Focus()
		return m, nil
	}

	m.config = msg.config
	m.method = msg.config.Options.InstallMethod
	m.screen = screenLoading
	m.status = "Loading registry…"
	return m, tea.Batch(m.spinner.Tick, loadRegistryCmd(m.config, false), checkShellPathCmd(m.config))
}

// handleShellPath records the state of the current shell's PATH, whether that
// came from the check at startup or from having just written the rc file.
//
// The layout is recomputed because the warning is a header line: without this
// the body stays sized for the taller header and the view is one line short —
// the same failure the help line had when it changed height.
func (m Model) handleShellPath(msg shellPathMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		// An unknown shell is not reported. There is nothing the user could
		// press, and clipack has no business editing a file whose syntax it
		// cannot write.
		m.shellKnown = false
		m.layout()
		return m, nil
	}

	added := m.shellStatus.NeedsPaths() && !msg.status.NeedsPaths()

	m.shellStatus = msg.status
	m.shellKnown = true
	if added {
		m.status = fmt.Sprintf("%s added to %s — restart %s to pick it up",
			m.config.Paths.Bin, msg.status.RCFile, msg.status.Shell.Name)
	}
	m.layout()
	return m, nil
}

// ---------------------------------------------------------------------------
// Registry loading
// ---------------------------------------------------------------------------

// handleRegistryLoaded installs a fetched package set into the browse screen.
func (m Model) handleRegistryLoaded(msg registryLoadedMsg) (tea.Model, tea.Cmd) {
	if len(msg.packages) == 0 {
		m.err = msg.err
		m.screen = screenBrowse
		return m, nil
	}

	m.packages = msg.packages
	m.installed = msg.installed
	m.refreshBroken()
	m.err = nil

	switch {
	case msg.warn != nil:
		m.status = "Loaded with warnings: " + msg.warn.Error()
	case msg.refreshed:
		m.status = fmt.Sprintf("Registry refreshed — %d packages", len(msg.packages))
	default:
		m.status = fmt.Sprintf("%d packages", len(msg.packages))
	}

	m.screen = screenBrowse
	m.applyTab()
	return m, nil
}

// applyTab repopulates the list for the active tab, keeping the selection in
// range, and refreshes the detail pane.
// refreshBroken re-checks every installed package against what is on disk.
func (m *Model) refreshBroken() {
	m.broken = make(map[string]bool, len(m.installed))
	if m.config == nil {
		return
	}
	for name, p := range m.installed {
		if !p.IsIntact(m.config) {
			m.broken[name] = true
		}
	}
}

func (m *Model) applyTab() {
	items := buildItems(m.packages, m.installed, m.broken, m.tab)
	m.list.SetItems(items)
	if m.list.Index() >= len(items) {
		m.list.Select(0)
	}
	m.ensureListVisible()
	// The installed state shown in the detail pane may have just changed.
	m.invalidateDetail()
	m.refreshDetail()
}

// ensureListVisible scrolls the list just enough to show the selected entry in
// full. Entries have individual heights, so this walks them rather than doing
// arithmetic on a fixed row size.
func (m *Model) ensureListVisible() {
	items := m.list.VisibleItems()
	index := m.list.Index()

	if len(items) == 0 || index < 0 {
		m.listOffset = 0
		return
	}
	if m.listOffset > index || m.listOffset >= len(items) {
		m.listOffset = index
	}
	if m.listOffset < 0 {
		m.listOffset = 0
	}

	height := m.listContentHeight()
	for m.listOffset < index {
		used := 0
		for i := m.listOffset; i <= index; i++ {
			used += entryHeight(items[i], m.list.Width(), m.showChecks(), m.styles)
		}
		if used <= height {
			break
		}
		m.listOffset++
	}
}

// refreshDetail re-renders the right-hand pane for the current selection.
// It is a no-op when the same package is already rendered, so scroll position
// survives unrelated updates.
func (m *Model) refreshDetail() {
	entry, ok := m.selected()
	if !ok {
		if m.detailFor != "" {
			m.detail.SetContent(m.styles.Muted.Render("No package selected."))
			m.detailFor = ""
		}
		return
	}
	if entry.pkg.Name == m.detailFor {
		return
	}

	m.detailBuf = newDetailBuffer(renderDetail(entry, m.detail.Width, m.styles))
	// A new package means a new buffer, so any selection into the old one is
	// meaningless.
	m.cursor, m.anchor, m.visual = pos{}, pos{}, visualNone
	m.syncDetail()
	m.detail.GotoTop()
	m.detailFor = entry.pkg.Name
}

// syncDetail redraws the detail viewport from the buffer, cursor and selection.
func (m *Model) syncDetail() {
	m.detail.SetContent(m.renderDetailBuffer())
}

// activeBuf is the buffer the selection applies to: the build log while an
// operation's output is on screen, the detail pane otherwise.
func (m Model) activeBuf() detailBuffer {
	if m.screen == screenRun {
		return m.logBuf
	}
	return m.detailBuf
}

// syncActive redraws whichever pane the selection is currently in.
func (m *Model) syncActive() {
	if m.screen == screenRun {
		m.logView.SetContent(m.renderDetailBuffer())
		return
	}
	m.syncDetail()
}

// moveCursor moves the text cursor, keeps it inside the buffer and scrolls the
// pane to follow it.
func (m *Model) moveCursor(p pos) {
	m.cursor = m.activeBuf().clamp(p)
	m.ensureCursorVisible()
	m.syncActive()
}

// ensureCursorVisible scrolls the viewport just enough to show the cursor line.
func (m *Model) ensureCursorVisible() {
	view := &m.detail
	if m.screen == screenRun {
		view = &m.logView
	}
	if view.Height <= 0 {
		return
	}
	switch {
	case m.cursor.line < view.YOffset:
		view.SetYOffset(m.cursor.line)
	case m.cursor.line >= view.YOffset+view.Height:
		view.SetYOffset(m.cursor.line - view.Height + 1)
	}
}

// invalidateDetail forces the next refreshDetail to re-render, which is needed
// after a resize or when the installed state of the shown package changed.
func (m *Model) invalidateDetail() {
	m.detailFor = ""
}

// showChecks reports whether the current tab supports marking packages.
//
// The three filtered tabs each stand for exactly one action, so a checkbox
// there is unambiguous. The All tab mixes installed and uninstalled packages,
// which leaves nothing for a mark to mean.
func (m Model) showChecks() bool { return m.tab != tabAll }

// tabAction is the operation a tab's checkboxes stand for.
func tabAction(tab tabID) action {
	switch tab {
	case tabNotInstalled:
		return actionInstall
	case tabUpdates:
		return actionUpdate
	case tabInstalled:
		return actionRemove
	default:
		return actionNone
	}
}

// checkedCount is how many packages are marked.
func (m Model) checkedCount() int {
	n := 0
	for _, on := range m.checked {
		if on {
			n++
		}
	}
	return n
}

// checkedItems returns the marked packages, in the order they appear on screen.
func (m Model) checkedItems() []packageItem {
	var out []packageItem
	for _, item := range m.list.VisibleItems() {
		entry, ok := item.(packageItem)
		if ok && m.checked[entry.pkg.Name] {
			out = append(out, entry)
		}
	}
	return out
}

// clearChecks drops the marks. Called whenever what is on screen changes, so a
// mark can never survive out of sight of the tab it was made in.
func (m *Model) clearChecks() {
	if len(m.checked) > 0 {
		m.checked = map[string]bool{}
	}
}

// toggleChecked marks or unmarks the package under the cursor.
func (m *Model) toggleChecked() {
	entry, ok := m.selected()
	if !ok {
		return
	}
	if m.checked[entry.pkg.Name] {
		delete(m.checked, entry.pkg.Name)
	} else {
		m.checked[entry.pkg.Name] = true
	}
}

// toggleCheckedAll marks every visible package, or clears them when they are
// already all marked.
func (m *Model) toggleCheckedAll() {
	items := m.list.VisibleItems()
	if len(items) == 0 {
		return
	}

	allMarked := true
	for _, item := range items {
		if entry, ok := item.(packageItem); ok && !m.checked[entry.pkg.Name] {
			allMarked = false
			break
		}
	}

	if allMarked {
		m.clearChecks()
		return
	}
	for _, item := range items {
		if entry, ok := item.(packageItem); ok {
			m.checked[entry.pkg.Name] = true
		}
	}
}

// contextualKeys returns the bindings that apply to what is selected and to the
// pane that has the focus, with the rest disabled — bubbles' help skips those.
//
// Only the help view uses this. The real bindings stay enabled, so pressing an
// action that does not apply still explains why instead of doing nothing.
func (m Model) contextualKeys() keyMap {
	keys := m.keys
	entry, ok := m.selected()

	installed := ok && entry.installed != nil

	// A package offers install, or update and remove — never both sets.
	keys.Install.SetEnabled(ok && !installed)
	keys.Update.SetEnabled(installed && entry.hasUpdate())
	keys.Remove.SetEnabled(installed)

	// Marking is per tab, and the tab's own action is the only one that runs on
	// a batch. Its label carries the count, so "u update" becoming "u update 3"
	// is what tells you the key stopped meaning "the row under the cursor".
	inDetail := m.focus == focusDetail
	marks := m.showChecks() && !inDetail
	keys.Check.SetEnabled(marks)
	keys.CheckAll.SetEnabled(marks)

	if n := m.checkedCount(); marks && n > 0 {
		switch tabAction(m.tab) {
		case actionInstall:
			keys.Install.SetEnabled(true)
			keys.Install.SetHelp("i", fmt.Sprintf("install %d", n))
		case actionUpdate:
			keys.Update.SetEnabled(true)
			keys.Update.SetHelp("u", fmt.Sprintf("update %d", n))
		case actionRemove:
			keys.Remove.SetEnabled(true)
			keys.Remove.SetHelp("x", fmt.Sprintf("remove %d", n))
		}
	}

	// m repins one package in the Installed tab and changes the global default
	// everywhere else, so the label has to say which.
	if m.tab == tabInstalled {
		keys.Method.SetEnabled(installed)
		if installed {
			keys.Method.SetHelp("m", "switch to "+otherMethod(installedMethod(entry, m.method)))
		}
	} else {
		keys.Method.SetHelp("m", "global method")
	}

	// Selecting and copying belong to the detail pane; filtering to the list.
	keys.Visual.SetEnabled(inDetail)
	keys.Yank.SetEnabled(inDetail)
	keys.Filter.SetEnabled(!inDetail)

	// Offered only while there is something to fix, and named after the shell
	// it will write to: "p add to zsh" says which of your shells this is about,
	// which matters precisely because the answer differs per shell.
	keys.Path.SetEnabled(m.needsShellPath())
	if m.needsShellPath() {
		keys.Path.SetHelp("p", "add to "+m.shellStatus.Shell.Name)
	}

	return keys
}

// needsShellPath reports whether the warning and its key belong on screen: the
// shell is one clipack can write to, and neither its PATH nor its startup file
// mentions the managed bin directory.
func (m Model) needsShellPath() bool {
	return m.shellKnown && m.shellStatus.NeedsPaths()
}

// selected returns the highlighted package, if any.
func (m Model) selected() (packageItem, bool) {
	item, ok := m.list.SelectedItem().(packageItem)
	return item, ok
}

// ---------------------------------------------------------------------------
// Browse screen
// ---------------------------------------------------------------------------

// updateBrowse handles the package list and detail pane.
func (m Model) updateBrowse(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, isKey := msg.(tea.KeyMsg)

	// While the filter input is focused every key belongs to the list.
	if isKey && m.list.FilterState() == list.Filtering {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		m.syncFilterChecks()
		m.ensureListVisible()
		m.refreshDetail()
		return m, cmd
	}

	// bubbles/list applies a filter asynchronously, so the selection can change
	// on a non-key message. Refreshing on every message (guarded by the package
	// name) keeps the detail pane in step with the highlighted row.
	if !isKey {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		m.syncFilterChecks()
		m.ensureListVisible()
		m.refreshDetail()
		return m, cmd
	}

	// The detail pane owns text navigation and selection, so it gets first
	// refusal on a key. It deliberately does not take the action keys, which
	// stay bound to the selected package from either pane.
	if isKey && m.focus == focusDetail {
		if next, cmd, handled := m.updateDetailPane(keyMsg); handled {
			return next, cmd
		}
	}

	if isKey {
		switch {
		case key.Matches(keyMsg, m.keys.Quit):
			m.quitting = true
			return m, tea.Quit

		case key.Matches(keyMsg, m.keys.FocusRight):
			m.focus = focusDetail
			m.syncDetail()
			return m, nil

		case key.Matches(keyMsg, m.keys.FocusLeft):
			m.focus = focusList
			return m, nil

		case key.Matches(keyMsg, m.keys.Help):
			m.showFullHelp = !m.showFullHelp
			m.help.ShowAll = m.showFullHelp
			m.layout()
			return m, nil

		case key.Matches(keyMsg, m.keys.Tab):
			m.tab = (m.tab + 1) % tabID(len(tabTitles))
			m.clearChecks()
			m.applyTab()
			return m, nil

		case key.Matches(keyMsg, m.keys.ShiftTab):
			m.tab = (m.tab + tabID(len(tabTitles)) - 1) % tabID(len(tabTitles))
			m.clearChecks()
			m.applyTab()
			return m, nil

		case key.Matches(keyMsg, m.keys.Check):
			if m.showChecks() {
				m.toggleChecked()
				m.status = m.checkedStatus()
			}
			return m, nil

		case key.Matches(keyMsg, m.keys.CheckAll):
			if m.showChecks() {
				m.toggleCheckedAll()
				m.status = m.checkedStatus()
			}
			return m, nil

		case key.Matches(keyMsg, m.keys.Method):
			// In the Installed tab m repins the package under the cursor.
			// Everywhere else it changes the global default, which is what a
			// fresh install uses — the same "the tab decides" rule as the
			// checkboxes.
			if m.tab == tabInstalled {
				return m.requestSwitchMethod()
			}
			m.method = otherMethod(m.method)
			m.status = "Global install method: " + m.method
			return m, nil

		case key.Matches(keyMsg, m.keys.Path):
			// Guarded rather than always bound: p is otherwise a free key, and
			// silently rewriting an rc file that is already correct is the kind
			// of thing that stacks duplicate exports.
			if m.needsShellPath() {
				return m, addShellPathCmd(m.config)
			}
			return m, nil

		case key.Matches(keyMsg, m.keys.Refresh):
			m.screen = screenLoading
			m.status = "Refreshing registry…"
			return m, tea.Batch(m.spinner.Tick, loadRegistryCmd(m.config, true))

		case key.Matches(keyMsg, m.keys.Install):
			return m.requestAction(actionInstall)

		case key.Matches(keyMsg, m.keys.Update):
			return m.requestAction(actionUpdate)

		case key.Matches(keyMsg, m.keys.Remove):
			return m.requestAction(actionRemove)

		case key.Matches(keyMsg, m.keys.Filter):
			// Filtering is a list operation, so it takes the focus with it.
			m.focus = focusList
		}
	}

	// Anything the detail pane did not claim falls through to the list, which
	// keeps the selection moving even while the details have the focus.
	if m.focus == focusDetail {
		var cmd tea.Cmd
		m.detail, cmd = m.detail.Update(msg)
		return m, cmd
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	m.syncFilterChecks()
	m.ensureListVisible()
	m.refreshDetail()
	return m, cmd
}

// syncFilterChecks drops the marks when the filter changes. A filter changes
// what is on screen, and a mark you can no longer see is one you have forgotten
// about — which is the last thing you want before a batch remove.
func (m *Model) syncFilterChecks() {
	if filter := m.list.FilterValue(); filter != m.lastFilter {
		m.lastFilter = filter
		m.clearChecks()
	}
}

// updateDetailPane handles vi-style navigation and selection inside the detail
// pane. It reports whether it consumed the key.
func (m Model) updateDetailPane(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	// Bubble Tea coalesces input that arrives in one read into a single
	// KeyRunes message, so held-down keys and pasted text turn up as "jjjj"
	// rather than four separate messages. Single-character motions have to see
	// them one at a time.
	if msg.Type == tea.KeyRunes && !msg.Alt && len(msg.Runes) > 1 {
		var handledAny bool
		for _, r := range msg.Runes {
			next, _, handled := m.updateDetailPane(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			if handled {
				m, handledAny = next, true
			}
		}
		return m, nil, handledAny
	}

	page := m.detail.Height
	if page < 1 {
		page = 1
	}

	switch msg.String() {
	// --- movement ---
	case "j", "down":
		m.moveCursor(pos{m.cursor.line + 1, m.cursor.col})
	case "k", "up":
		m.moveCursor(pos{m.cursor.line - 1, m.cursor.col})
	case "l", "right":
		m.moveCursor(pos{m.cursor.line, m.cursor.col + 1})
	case "h", "left":
		// Walking off the left edge leaves the pane. Without this the only way
		// back would be esc, and ← is what brought the focus here.
		if m.cursor.col == 0 {
			m.focus = focusList
			m.visual = visualNone
			m.syncDetail()
			return m, nil, true
		}
		m.moveCursor(pos{m.cursor.line, m.cursor.col - 1})
	case "0", "home":
		m.moveCursor(pos{m.cursor.line, 0})
	case "$", "end":
		m.moveCursor(pos{m.cursor.line, m.activeBuf().lineLen(m.cursor.line) - 1})
	case "g":
		m.moveCursor(pos{0, 0})
	case "G":
		m.moveCursor(pos{m.activeBuf().lines() - 1, 0})
	case "ctrl+d":
		m.moveCursor(pos{m.cursor.line + page/2, m.cursor.col})
	case "ctrl+u":
		m.moveCursor(pos{m.cursor.line - page/2, m.cursor.col})
	case "pgdown":
		m.moveCursor(pos{m.cursor.line + page, m.cursor.col})
	case "pgup":
		m.moveCursor(pos{m.cursor.line - page, m.cursor.col})

	// --- selection ---
	case "v":
		m.visual = toggleVisual(m.visual, visualChar)
		m.anchor = m.cursor
		m.status = visualStatus(m.visual)
		m.syncDetail()
	case "V":
		m.visual = toggleVisual(m.visual, visualLine)
		m.anchor = m.cursor
		m.status = visualStatus(m.visual)
		m.syncDetail()
	case "y":
		return m.yank()

	case "esc":
		// esc first cancels the selection, then leaves the pane — two steps, so
		// an accidental selection does not also lose the focus.
		if m.visual != visualNone {
			m.visual = visualNone
			m.status = ""
		} else {
			m.focus = focusList
		}
		m.syncDetail()

	default:
		return m, nil, false
	}

	return m, nil, true
}

// yank copies the selection to the clipboard and leaves visual mode.
func (m Model) yank() (Model, tea.Cmd, bool) {
	text := m.selectedText()
	mode := m.visual

	m.visual = visualNone
	m.syncActive()

	if text == "" {
		m.status = "Nothing to copy"
		return m, nil, true
	}

	if err := copyToClipboard(text); err != nil {
		m.err = nil
		m.status = "Copy failed: " + err.Error()
		return m, nil, true
	}

	m.status = yankSummary(text, mode)
	return m, nil, true
}

// toggleVisual switches into want, or back out when it is already active.
func toggleVisual(current, want visualMode) visualMode {
	if current == want {
		return visualNone
	}
	return want
}

// visualStatus is the hint shown while a selection is being made.
func visualStatus(mode visualMode) string {
	switch mode {
	case visualChar:
		return "VISUAL — move to extend, y to copy, esc to cancel"
	case visualLine:
		return "VISUAL LINE — move to extend, y to copy, esc to cancel"
	default:
		return ""
	}
}

// checkedStatus describes the current marks for the status line.
func (m Model) checkedStatus() string {
	n := m.checkedCount()
	if n == 0 {
		return "Nothing marked"
	}

	verb := strings.ToLower(tabAction(m.tab).label())
	return fmt.Sprintf("%d marked — %s to %s them", n, actionKey(tabAction(m.tab)), verb)
}

// actionKey is the key that runs an action, for use in messages.
func actionKey(a action) string {
	switch a {
	case actionInstall:
		return "i"
	case actionUpdate:
		return "u"
	case actionRemove:
		return "x"
	default:
		return ""
	}
}

// requestAction validates an action against the selection and opens the
// confirmation dialog.
//
// Marks only drive the tab's own action: every other key keeps acting on the
// row under the cursor, so pressing x in the Updates tab removes the package
// you are looking at rather than everything you happened to mark.
func (m Model) requestAction(a action) (tea.Model, tea.Cmd) {
	if m.showChecks() && m.checkedCount() > 0 && a == tabAction(m.tab) {
		return m.requestBatch(a)
	}

	entry, ok := m.selected()
	if !ok {
		m.status = "Nothing selected"
		return m, nil
	}

	// What a package offers depends on whether it is installed: install applies
	// only to what is not, update and remove only to what is. Reinstalling over
	// an existing install is deliberately not offered — it leaves the previous
	// version's binaries and man pages behind, because only update knows to
	// clean them up from the old manifest.
	switch a {
	case actionInstall:
		if entry.installed != nil {
			m.status = entry.pkg.Name + " is installed — u to update, x to remove"
			return m, nil
		}
	case actionUpdate:
		if entry.installed == nil {
			m.status = entry.pkg.Name + " is not installed — i to install"
			return m, nil
		}
		if !entry.hasUpdate() {
			m.status = entry.pkg.Name + " is already up to date"
			return m, nil
		}
	case actionRemove:
		if entry.installed == nil {
			m.status = entry.pkg.Name + " is not installed"
			return m, nil
		}
	}

	m.pending = a
	m.pendingItem = entry
	m.pendingBatch = nil
	m.pendingSkipped = nil
	m.screen = screenConfirm
	return m, nil
}

// requestSwitchMethod offers to repin the package under the cursor to the other
// ref. Only the Installed tab reaches this; elsewhere m changes the global
// default instead.
func (m Model) requestSwitchMethod() (tea.Model, tea.Cmd) {
	entry, ok := m.selected()
	if !ok {
		m.status = "Nothing selected"
		return m, nil
	}
	if entry.installed == nil {
		m.status = entry.pkg.Name + " is not installed"
		return m, nil
	}

	target := otherMethod(installedMethod(entry, m.method))
	if entry.pkg.Ref(target) == "" {
		m.status = fmt.Sprintf("%s has no %s in the registry", entry.pkg.Name, target)
		return m, nil
	}

	m.pending = actionSwitchMethod
	m.pendingItem = entry
	m.pendingMethod = target
	m.pendingBatch = nil
	m.pendingSkipped = nil
	m.screen = screenConfirm
	return m, nil
}

// requestBatch opens the confirmation dialog for the marked packages.
//
// Within a tab every entry is eligible by construction, so the skipped list is
// normally empty. It still gets built: the registry reloads while the interface
// is open, and a package can stop being eligible between the mark and the
// confirmation.
func (m Model) requestBatch(a action) (tea.Model, tea.Cmd) {
	var batch []packageItem
	var skipped []string

	for _, entry := range m.checkedItems() {
		if eligible(entry, a) {
			batch = append(batch, entry)
			continue
		}
		skipped = append(skipped, entry.pkg.Name)
	}

	if len(batch) == 0 {
		m.status = fmt.Sprintf("Nothing to %s among the marked packages",
			strings.ToLower(a.label()))
		return m, nil
	}

	m.pending = a
	m.pendingBatch = batch
	m.pendingSkipped = skipped
	m.screen = screenConfirm
	return m, nil
}

// eligible reports whether an action can run on a package.
func eligible(entry packageItem, a action) bool {
	switch a {
	case actionInstall:
		return entry.installed == nil
	case actionUpdate:
		return entry.installed != nil && entry.hasUpdate()
	case actionRemove:
		return entry.installed != nil
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// Confirm screen
// ---------------------------------------------------------------------------

// updateConfirm handles the yes/no modal.
func (m Model) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	switch {
	case key.Matches(keyMsg, m.keys.Cancel), key.Matches(keyMsg, m.keys.Quit):
		m.pending = actionNone
		m.screen = screenBrowse
		m.status = "Cancelled"
		return m, nil

	case key.Matches(keyMsg, m.keys.Confirm):
		return m.startOperation()
	}

	return m, nil
}

// startOperation kicks off the pending action in the background.
func (m Model) startOperation() (tea.Model, tea.Cmd) {
	act := m.pending
	method := m.method
	switchTo := m.pendingMethod

	batch := m.pendingBatch
	target := fmt.Sprintf("%d packages", len(batch))
	if len(batch) == 0 {
		batch = []packageItem{m.pendingItem}
		target = m.pendingItem.pkg.Name
	}

	op := func(in *pkg.Installer) error {
		var errs []error

		for _, entry := range batch {
			if len(batch) > 1 {
				in.Report(pkg.Event{Kind: pkg.EventInfo, Text: "── " + entry.pkg.Name + " ──"})
			}

			var err error
			switch act {
			case actionInstall:
				// The installer stamps the method onto the package it is given,
				// so it gets a copy rather than the cached registry entry.
				err = in.Install(clonePackage(entry.pkg), method)
			case actionUpdate:
				err = in.Update(clonePackage(entry.pkg), installedMethod(entry, method))
			case actionRemove:
				// Remove works from the installed manifest, the only record of
				// what was actually put on disk.
				err = in.Remove(entry.installed)
			case actionSwitchMethod:
				// Update is the rebuild: it clears what the current pin put on
				// disk and installs again. The only difference here is the
				// method, which comes from the switch rather than the manifest.
				err = in.Update(clonePackage(entry.pkg), switchTo)
			}

			if err != nil {
				// One failure does not stop the rest: abandoning a batch
				// half-way leaves a state nobody asked for and the log no
				// longer says what happened to the packages after it.
				//
				// Only a batch reports the failure inline, to mark where in the
				// log it happened. For a single package the joined error below
				// is the same text, and printing both showed it twice.
				if len(batch) > 1 {
					in.Report(pkg.Event{Kind: pkg.EventError, Text: entry.pkg.Name + ": " + err.Error()})
				}
				errs = append(errs, fmt.Errorf("%s: %w", entry.pkg.Name, err))
			}
		}

		return errors.Join(errs...)
	}

	m.stream = newOpStream(m.config, op)
	m.logLines = nil
	m.runAction = act
	m.runTarget = target
	m.runFailed = false
	m.runDone = false
	m.pending = actionNone
	m.pendingBatch = nil
	m.pendingSkipped = nil
	m.pendingMethod = ""
	m.clearChecks()
	m.screen = screenRun
	m.logView.SetContent("")

	return m, tea.Batch(m.spinner.Tick, waitForEventCmd(m.stream))
}

// installedMethod keeps an update on the method the package was installed with,
// falling back to the currently selected method.
func installedMethod(entry packageItem, fallback string) string {
	if entry.installed != nil && entry.installed.InstallMethod != "" {
		return entry.installed.InstallMethod
	}
	return fallback
}

// clonePackage copies a registry package before handing it to the installer,
// which mutates InstallMethod on the value it is given. Without the copy the
// cached registry entry would be modified in place.
func clonePackage(p *pkg.Package) *pkg.Package {
	copied := *p
	return &copied
}

// ---------------------------------------------------------------------------
// Run screen
// ---------------------------------------------------------------------------

// maxLogLines caps the retained log so a long compile cannot grow the retained
// output without bound.
const maxLogLines = 4000

// handleOpEvents appends a batch of progress events and keeps the view pinned
// to the bottom.
func (m Model) handleOpEvents(msg opEventsMsg) (tea.Model, tea.Cmd) {
	for _, event := range msg.events {
		if event.Kind == pkg.EventError {
			m.runFailed = true
		}
		m.logLines = append(m.logLines, formatEvent(event, m.styles))
	}

	if len(m.logLines) > maxLogLines {
		m.logLines = m.logLines[len(m.logLines)-maxLogLines:]
	}

	m.syncLog()

	if msg.finished {
		return m.handleOpFinished(opFinishedMsg{err: msg.err})
	}
	return m, waitForEventCmd(m.stream)
}

// syncLog re-renders the log viewport, wrapping long lines so nothing is cut
// off at the pane border, and scrolls to the newest output.
func (m *Model) syncLog() {
	width := m.logView.Width
	if width < 10 {
		width = 10
	}
	wrapped := lipgloss.NewStyle().Width(width).Render(strings.Join(m.logLines, "\n"))
	m.logBuf = newDetailBuffer(wrapped)

	// While text is being selected the view must not jump: new output would
	// otherwise scroll the selection out from under the cursor mid-drag.
	if m.visual != visualNone {
		m.logView.SetContent(m.renderDetailBuffer())
		return
	}
	m.logView.SetContent(wrapped)
	m.logView.GotoBottom()
}

// handleOpFinished records the outcome and reloads the installed set.
func (m Model) handleOpFinished(msg opFinishedMsg) (tea.Model, tea.Cmd) {
	m.runDone = true
	m.stream = nil

	if msg.err != nil {
		m.runFailed = true
		m.logLines = append(m.logLines, m.styles.Err.Render(m.styles.Icons.Error+" "+msg.err.Error()))
		m.syncLog()
	}

	// Refresh the installed set so badges reflect what just happened.
	installed, err := pkg.InstalledMap(m.config)
	if err == nil {
		m.installed = installed
		m.refreshBroken()
		m.applyTab()
	}

	return m, nil
}

// updateRun handles the streaming log screen.
func (m Model) updateRun(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, isKey := msg.(tea.KeyMsg)
	if !isKey {
		var cmd tea.Cmd
		m.logView, cmd = m.logView.Update(msg)
		return m, cmd
	}

	// Held keys and pasted text arrive as one multi-rune message; motions have
	// to see them one at a time. Same reason as in the detail pane.
	if keyMsg.Type == tea.KeyRunes && !keyMsg.Alt && len(keyMsg.Runes) > 1 {
		var out tea.Model = m
		for _, r := range keyMsg.Runes {
			out, _ = out.(Model).updateRun(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}
		return out, nil
	}

	page := m.logView.Height
	if page < 1 {
		page = 1
	}

	switch keyMsg.String() {
	// --- selection ---
	//
	// The build log is the one thing a user needs to hand to someone else when
	// an install fails, and until now it could only be photographed. `y` with
	// nothing selected takes the WHOLE log, because that is the case that
	// matters; v and V are there for when only part of it is wanted.
	case "y":
		if m.visual == visualNone {
			return m.yankWholeLog()
		}
		next, cmd, _ := m.yank()
		return next, cmd
	case "v":
		m.visual = toggleVisual(m.visual, visualChar)
		m.anchor = m.cursor
		m.status = visualStatus(m.visual)
		m.syncActive()
		return m, nil
	case "V":
		m.visual = toggleVisual(m.visual, visualLine)
		m.anchor = m.cursor
		m.status = visualStatus(m.visual)
		m.syncActive()
		return m, nil

	// --- motions ---
	case "j":
		m.moveCursor(pos{m.cursor.line + 1, m.cursor.col})
		return m, nil
	case "k":
		m.moveCursor(pos{m.cursor.line - 1, m.cursor.col})
		return m, nil
	case "l":
		m.moveCursor(pos{m.cursor.line, m.cursor.col + 1})
		return m, nil
	case "h":
		m.moveCursor(pos{m.cursor.line, m.cursor.col - 1})
		return m, nil
	case "0", "home":
		m.moveCursor(pos{m.cursor.line, 0})
		return m, nil
	case "$", "end":
		m.moveCursor(pos{m.cursor.line, m.activeBuf().lineLen(m.cursor.line) - 1})
		return m, nil
	case "g":
		m.moveCursor(pos{0, 0})
		return m, nil
	case "G":
		m.moveCursor(pos{m.activeBuf().lines() - 1, 0})
		return m, nil
	case "ctrl+d":
		m.moveCursor(pos{m.cursor.line + page/2, m.cursor.col})
		return m, nil
	case "ctrl+u":
		m.moveCursor(pos{m.cursor.line - page/2, m.cursor.col})
		return m, nil

	case "esc", "q", "enter":
		// esc cancels a selection before it leaves the screen, so an accidental
		// selection does not also throw away the log.
		if keyMsg.String() == "esc" && m.visual != visualNone {
			m.visual = visualNone
			m.status = ""
			m.syncActive()
			return m, nil
		}
		// Refuse to leave while the operation is still writing output;
		// otherwise the log would keep growing behind the browse screen.
		if !m.runDone {
			m.status = "Operation still running…"
			return m, nil
		}
		m.visual = visualNone
		m.screen = screenBrowse
		m.status = m.runSummary()
		return m, nil
	}

	var cmd tea.Cmd
	m.logView, cmd = m.logView.Update(msg)
	return m, cmd
}

// yankWholeLog copies the entire build log, which is what someone reporting a
// failed install actually needs. It takes the plain text rather than what is on
// screen, so the copy carries no escape sequences.
func (m Model) yankWholeLog() (tea.Model, tea.Cmd) {
	lines := make([]string, 0, m.logBuf.lines())
	for _, line := range m.logBuf.plain {
		lines = append(lines, strings.TrimRight(line, " "))
	}
	text := strings.TrimRight(strings.Join(lines, "\n"), "\n")

	if text == "" {
		m.status = "Nothing to copy"
		return m, nil
	}
	if err := copyToClipboard(text); err != nil {
		m.status = "Copy failed: " + err.Error()
		return m, nil
	}
	m.status = fmt.Sprintf("Copied the whole log (%d lines)", len(lines))
	return m, nil
}

// runSummary describes the finished operation for the status line.
func (m Model) runSummary() string {
	if m.runFailed {
		return fmt.Sprintf("%s of %s failed", m.runAction.label(), m.runTarget)
	}
	return fmt.Sprintf("%s %s successfully", m.runTarget, m.runAction.past())
}

// formatEvent renders one installer event as a styled log line.
func formatEvent(e pkg.Event, s Styles) string {
	icons := s.Icons

	switch e.Kind {
	case pkg.EventStep:
		return s.Step.Render(fmt.Sprintf("%s [%d/%d] ", icons.Step, e.Step, e.Total)) + e.Text
	case pkg.EventOutput:
		return s.Muted.Render("  "+icons.Output+" ") + e.Text
	case pkg.EventWarn:
		return s.Warn.Render(icons.Warn + " " + e.Text)
	case pkg.EventError:
		return s.Err.Render(icons.Error + " " + e.Text)
	case pkg.EventHint:
		// The cause is highlighted and the suggested fix is not, so the eye
		// lands on what broke before what to type.
		lines := strings.Split(e.Text, "\n")
		out := s.Warn.Render(icons.Warn + " " + lines[0])
		for _, line := range lines[1:] {
			out += "\n" + s.Muted.Render("  "+line)
		}
		return out
	case pkg.EventDone:
		return s.OK.Render(icons.Done + " " + e.Text)
	default:
		return s.Muted.Render(icons.Separator+" ") + e.Text
	}
}
