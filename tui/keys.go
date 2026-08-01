// Package tui implements clipack's Bubble Tea terminal user interface.
//
// The UI is a single root model (see model.go) that switches between a small
// number of screens:
//
//	setup   - first-run wizard asking where packages should live
//	browse  - package list on the left, package details on the right
//	confirm - a modal asking to confirm install / update / remove
//	run     - a streaming log of a running operation
//
// All package operations are delegated to pkg.Installer, which reports progress
// as pkg.Event values. Those events are pushed onto a channel and pulled into
// the Bubble Tea update loop, so the UI never blocks on a build.
package tui

import "github.com/charmbracelet/bubbles/key"

// keyMap collects every binding the UI reacts to. Bindings are declared once
// here so the help view stays in sync with the actual behaviour.
type keyMap struct {
	Up       key.Binding
	Down     key.Binding
	Tab      key.Binding
	ShiftTab key.Binding
	// FocusLeft and FocusRight move between the package list and the detail
	// pane. Move, Pane and Tabs are display-only aggregates: the collapsed help
	// has to fit on one line, so it shows "↑↓ move" rather than an entry each
	// for up and down.
	FocusLeft  key.Binding
	FocusRight key.Binding
	Move       key.Binding
	Pane       key.Binding
	Tabs       key.Binding
	// Visual and Yank are the detail pane's vi-style selection keys.
	Visual key.Binding
	Yank   key.Binding
	// Check and CheckAll mark packages for a batch operation.
	Check    key.Binding
	CheckAll key.Binding
	Install  key.Binding
	Update   key.Binding
	Remove   key.Binding
	Refresh  key.Binding
	Method   key.Binding
	Filter   key.Binding
	Confirm  key.Binding
	Cancel   key.Binding
	Help     key.Binding
	Quit     key.Binding
}

// defaultKeys returns the standard binding set.
func defaultKeys() keyMap {
	return keyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Tab: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "next view"),
		),
		ShiftTab: key.NewBinding(
			key.WithKeys("shift+tab"),
			key.WithHelp("shift+tab", "prev view"),
		),
		FocusLeft: key.NewBinding(
			key.WithKeys("left", "h"),
			key.WithHelp("←/h", "list"),
		),
		FocusRight: key.NewBinding(
			key.WithKeys("right", "l"),
			key.WithHelp("→/l", "details"),
		),
		// Display only: the bindings above do the matching.
		Move: key.NewBinding(
			key.WithKeys("up", "down", "j", "k"),
			key.WithHelp("↑↓", "move"),
		),
		Pane: key.NewBinding(
			key.WithKeys("left", "right", "h", "l"),
			key.WithHelp("←→", "pane"),
		),
		Tabs: key.NewBinding(
			key.WithKeys("tab", "shift+tab"),
			key.WithHelp("tab", "tabs"),
		),
		// Display only: updateDetailPane matches these directly, because they
		// apply solely while the detail pane has the focus.
		Visual: key.NewBinding(
			key.WithKeys("v", "V"),
			key.WithHelp("v/V", "select"),
		),
		Yank: key.NewBinding(
			key.WithKeys("y"),
			key.WithHelp("y", "copy"),
		),
		Check: key.NewBinding(
			key.WithKeys(" "),
			key.WithHelp("space", "mark"),
		),
		CheckAll: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "mark all"),
		),
		Install: key.NewBinding(
			key.WithKeys("i", "enter"),
			key.WithHelp("i/enter", "install"),
		),
		Update: key.NewBinding(
			key.WithKeys("u"),
			key.WithHelp("u", "update"),
		),
		Remove: key.NewBinding(
			key.WithKeys("x"),
			key.WithHelp("x", "remove"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "refresh"),
		),
		Method: key.NewBinding(
			key.WithKeys("m"),
			key.WithHelp("m", "version/commit"),
		),
		Filter: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "filter"),
		),
		Confirm: key.NewBinding(
			key.WithKeys("y", "enter"),
			key.WithHelp("y", "confirm"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc", "n"),
			key.WithHelp("esc", "cancel"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "help"),
		),
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
	}
}

// ShortHelp implements help.KeyMap for the collapsed help line.
//
// Every binding the interface has is listed. Which of them actually appear is
// decided by Model.contextualKeys, which disables the ones that do not apply to
// the selected package or the focused pane — help skips disabled bindings.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		k.Move, k.Pane, k.Tabs,
		k.Check, k.CheckAll,
		k.Install, k.Update, k.Remove,
		k.Visual, k.Yank,
		k.Filter, k.Refresh, k.Method,
		k.Help, k.Quit,
	}
}

// FullHelp implements help.KeyMap for the expanded help view.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.FocusLeft, k.FocusRight},
		{k.Tab, k.ShiftTab, k.Filter, k.Refresh},
		{k.Check, k.CheckAll, k.Method},
		{k.Install, k.Update, k.Remove},
		{k.Visual, k.Yank, k.Help, k.Quit},
	}
}
