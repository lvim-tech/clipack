package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lvim-tech/clipack/pkg"
)

// indicatorModel puts the browse model on the run screen with a batch of the given
// size, without spawning a real installer.
func indicatorModel(t *testing.T, total int) Model {
	t.Helper()

	m := browseModel(t)
	m = applyMsg(t, m, tea.WindowSizeMsg{Width: 140, Height: 40})
	m.screen = screenRun
	m.runAction = actionInstall
	m.runTarget = "8 packages"
	m.runTotal = total
	return m
}

// TestRunIndicatorFollowsTheBuild is the "Installing 8 packages… from start to
// finish" complaint: the header now names the package, its position in the
// batch and the step it is at — the same events the log already renders, put
// where the eye rests.
func TestRunIndicatorFollowsTheBuild(t *testing.T) {
	m := indicatorModel(t, 8)

	m = applyMsg(t, m, opEventsMsg{events: []pkg.Event{
		{Kind: pkg.EventStep, Package: "kitty", Step: 2, Total: 4, Text: "cargo build --release"},
	}})

	view := m.View()
	for _, want := range []string{"kitty (1/8)", "step 2/4", "cargo build --release"} {
		if !strings.Contains(view, want) {
			t.Errorf("run view is missing %q", want)
		}
	}

	// The next package advances the counter and drops the stale step.
	m = applyMsg(t, m, opEventsMsg{events: []pkg.Event{
		{Kind: pkg.EventInfo, Package: "wezterm", Text: "Installing wezterm"},
	}})
	view = m.View()
	if !strings.Contains(view, "wezterm (2/8)") {
		t.Errorf("run view did not advance to the second package:\n%s", view)
	}
	if strings.Contains(view, "step 2/4") {
		t.Error("the previous package's step survived into the next one")
	}
}

// TestRunIndicatorShowsStepsForASinglePackage is the follow-up request: the
// step counter belongs on a one-package install too, just without the batch
// parentheses that would only ever say (1/1).
func TestRunIndicatorShowsStepsForASinglePackage(t *testing.T) {
	m := indicatorModel(t, 1)

	m = applyMsg(t, m, opEventsMsg{events: []pkg.Event{
		{Kind: pkg.EventStep, Package: "yazi", Step: 1, Total: 3, Text: "git clone https://github.com/sxyazi/yazi.git ."},
	}})

	view := m.View()
	if !strings.Contains(view, "yazi") || !strings.Contains(view, "step 1/3") {
		t.Errorf("run view is missing the single-package step counter:\n%s", view)
	}
	if strings.Contains(view, "(1/1)") {
		t.Error("a single package shows a batch counter that can only ever say (1/1)")
	}
}

// TestRunIndicatorKeepsOneLineForMultiLineSteps guards the kitty case: its
// build step is a commented block, and the indicator must show the command,
// not the first comment line, and never grow past one line.
func TestRunIndicatorKeepsOneLineForMultiLineSteps(t *testing.T) {
	m := indicatorModel(t, 1)

	m = applyMsg(t, m, opEventsMsg{events: []pkg.Event{
		{Kind: pkg.EventStep, Package: "kitty", Step: 2, Total: 2, Text: "PY=python3\nsetup.py linux-package"},
	}})

	// Only the indicator line: the log below legitimately shows every line of
	// the step, so the whole view is the wrong thing to assert on.
	header := strings.SplitN(m.View(), "\n", 2)[0]
	if !strings.Contains(header, "step 2/2: PY=python3") {
		t.Errorf("the indicator is missing the first line of the step:\n%s", header)
	}
	if strings.Contains(header, "setup.py linux-package") {
		t.Error("the indicator leaked the step's second line")
	}
}
