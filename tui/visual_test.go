package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// bufferModel returns a browse model whose detail buffer is a known set of
// plain lines, so selection can be asserted exactly.
func bufferModel(t *testing.T, lines ...string) Model {
	t.Helper()

	m := browseModel(t)
	m.detailBuf = newDetailBuffer(strings.Join(lines, "\n"))
	m.focus = focusDetail
	m.cursor, m.anchor, m.visual = pos{}, pos{}, visualNone
	return m
}

func TestNewDetailBufferStripsStyling(t *testing.T) {
	styled := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ff0000")).Render("Homepage") +
		"   https://example.com"

	b := newDetailBuffer(styled + "\nsecond line")

	if b.lines() != 2 {
		t.Fatalf("lines = %d, want 2", b.lines())
	}
	// The plain text has to line up with the styled text character for
	// character, or every column offset in a selection would be wrong.
	if b.plain[0] != "Homepage   https://example.com" {
		t.Errorf("plain[0] = %q, want the styling stripped", b.plain[0])
	}
	if b.lineLen(0) != len([]rune("Homepage   https://example.com")) {
		t.Errorf("lineLen(0) = %d, want the rune count of the plain line", b.lineLen(0))
	}
}

func TestPosBefore(t *testing.T) {
	tests := []struct {
		a, b pos
		want bool
	}{
		{pos{0, 0}, pos{0, 1}, true},
		{pos{0, 5}, pos{1, 0}, true},
		{pos{1, 0}, pos{0, 9}, false},
		{pos{2, 3}, pos{2, 3}, false},
	}
	for _, tt := range tests {
		if got := tt.a.before(tt.b); got != tt.want {
			t.Errorf("%v.before(%v) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestClampKeepsTheCursorInside(t *testing.T) {
	b := newDetailBuffer("abc\nlonger line\n")

	tests := []struct {
		in   pos
		want pos
	}{
		{pos{-5, -5}, pos{0, 0}},
		{pos{99, 0}, pos{2, 0}}, // last line is the empty tail
		{pos{0, 99}, pos{0, 3}}, // one past the last character
		{pos{1, 4}, pos{1, 4}},  // already valid
	}
	for _, tt := range tests {
		if got := b.clamp(tt.in); got != tt.want {
			t.Errorf("clamp(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestYankWithoutVisualTakesTheCursorLine(t *testing.T) {
	m := bufferModel(t, "first line", "second line", "third line")
	m.cursor = pos{1, 4}

	// With no selection, y behaves like vi's yy.
	if got := m.selectedText(); got != "second line" {
		t.Errorf("selectedText() = %q, want the cursor line", got)
	}
}

func TestLinewiseSelection(t *testing.T) {
	m := bufferModel(t, "alpha", "beta", "gamma", "delta")
	m.cursor = pos{1, 2}
	m.anchor = pos{1, 2}
	m.visual = visualLine

	// One line to start with.
	if got := m.selectedText(); got != "beta" {
		t.Errorf("selectedText() = %q, want beta", got)
	}

	// Extended downwards, whole lines regardless of the columns.
	m.cursor = pos{2, 0}
	if got := m.selectedText(); got != "beta\ngamma" {
		t.Errorf("selectedText() = %q, want beta\\ngamma", got)
	}

	// Extended upwards: the anchor is below the cursor and the range still
	// comes out in reading order.
	m.cursor = pos{0, 4}
	if got := m.selectedText(); got != "alpha\nbeta" {
		t.Errorf("selectedText() = %q, want alpha\\nbeta", got)
	}
}

func TestCharwiseSelectionOnOneLine(t *testing.T) {
	m := bufferModel(t, "Homepage      https://github.com/atuinsh/atuin")

	// The URL starts at column 14 and runs to the end of the line.
	m.anchor = pos{0, 14}
	m.cursor = pos{0, m.detailBuf.lineLen(0) - 1}
	m.visual = visualChar

	want := "https://github.com/atuinsh/atuin"
	if got := m.selectedText(); got != want {
		t.Errorf("selectedText() = %q, want %q", got, want)
	}
}

func TestCharwiseSelectionIsInclusive(t *testing.T) {
	m := bufferModel(t, "abcdef")
	m.anchor = pos{0, 1}
	m.cursor = pos{0, 3}
	m.visual = visualChar

	// vi includes the character under the cursor.
	if got := m.selectedText(); got != "bcd" {
		t.Errorf("selectedText() = %q, want bcd", got)
	}
}

func TestCharwiseSelectionBackwards(t *testing.T) {
	m := bufferModel(t, "abcdef")
	m.anchor = pos{0, 4}
	m.cursor = pos{0, 1}
	m.visual = visualChar

	if got := m.selectedText(); got != "bcde" {
		t.Errorf("selectedText() = %q, want bcde when selecting right to left", got)
	}
}

func TestCharwiseSelectionAcrossLines(t *testing.T) {
	m := bufferModel(t, "alpha", "beta", "gamma")
	m.anchor = pos{0, 3}
	m.cursor = pos{2, 1}
	m.visual = visualChar

	// Partial first line, whole middle lines, partial last line.
	want := "ha\nbeta\nga"
	if got := m.selectedText(); got != want {
		t.Errorf("selectedText() = %q, want %q", got, want)
	}
}

func TestSelectionHandlesMultiByteText(t *testing.T) {
	m := bufferModel(t, "пакет мениджър")
	m.anchor = pos{0, 0}
	m.cursor = pos{0, 4}
	m.visual = visualChar

	// Columns count runes, so a Cyrillic word is not cut mid-character.
	if got := m.selectedText(); got != "пакет" {
		t.Errorf("selectedText() = %q, want пакет", got)
	}
}

func TestLinewiseSelectionTrimsTrailingSpaces(t *testing.T) {
	m := bufferModel(t, "value       ", "next")
	m.anchor, m.cursor = pos{0, 0}, pos{0, 0}
	m.visual = visualLine

	// Detail lines are padded to the pane width; the padding is not content.
	if got := m.selectedText(); got != "value" {
		t.Errorf("selectedText() = %q, want the padding trimmed", got)
	}
}

func TestBufferDropsRenderingPadding(t *testing.T) {
	// A pane-width line as lipgloss renders it: content plus padding out to the
	// right edge.
	padded := lipgloss.NewStyle().Width(40).Render("Homepage      https://example.com")

	b := newDetailBuffer(padded)

	want := "Homepage      https://example.com"
	if b.plain[0] != want {
		t.Errorf("plain[0] = %q, want the padding dropped", b.plain[0])
	}
	// $ has to land on the last real character, not on a blank column.
	if b.lineLen(0) != len([]rune(want)) {
		t.Errorf("lineLen(0) = %d, want %d", b.lineLen(0), len([]rune(want)))
	}
}

func TestCharwiseSelectionStopsAtTheRealEndOfLine(t *testing.T) {
	padded := lipgloss.NewStyle().Width(60).Render("Homepage      https://github.com/atuinsh/atuin")

	m := browseModel(t)
	m.detailBuf = newDetailBuffer(padded)
	m.focus = focusDetail
	m.cursor = pos{0, 14}
	m = applyMsg(t, m, keyMsg("v"))
	m = applyMsg(t, m, keyMsg("$"))

	want := "https://github.com/atuinsh/atuin"
	if got := m.selectedText(); got != want {
		t.Errorf("selectedText() = %q, want %q with no trailing padding", got, want)
	}
}

func TestSelectionRangeIsEmptyWithoutVisualMode(t *testing.T) {
	m := bufferModel(t, "a", "b")
	if _, _, ok := m.selectionRange(); ok {
		t.Error("selectionRange() reported a selection with no visual mode active")
	}
}

func TestRenderDetailBufferMarksTheSelection(t *testing.T) {
	m := bufferModel(t, "alpha", "beta")
	m.anchor, m.cursor = pos{0, 0}, pos{0, 0}
	m.visual = visualLine

	out := m.renderDetailBuffer()
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "beta") {
		t.Errorf("rendered buffer lost content:\n%s", out)
	}
	if strings.Count(out, "\n") != 1 {
		t.Errorf("rendered buffer has %d newlines, want the line count preserved", strings.Count(out, "\n"))
	}
}

func TestRenderDetailBufferShowsAnEmptySelectedLine(t *testing.T) {
	m := bufferModel(t, "", "beta")
	m.anchor, m.cursor = pos{0, 0}, pos{0, 0}
	m.visual = visualLine

	// An empty line still has to show that it is inside the selection, so it is
	// rendered as a highlighted space rather than nothing at all.
	lines := strings.Split(m.renderDetailBuffer(), "\n")
	if strings.TrimSpace(lines[0]) != "" && len(lines[0]) == 0 {
		t.Errorf("the empty selected line rendered as %q", lines[0])
	}
	if len(lines[0]) == 0 {
		t.Error("the empty selected line is invisible")
	}
}

func TestCursorMovement(t *testing.T) {
	m := bufferModel(t, "alpha", "beta", "gamma")

	m = applyMsg(t, m, keyMsg("j"))
	if m.cursor.line != 1 {
		t.Errorf("cursor.line = %d after j, want 1", m.cursor.line)
	}

	m = applyMsg(t, m, keyMsg("l"))
	if m.cursor.col != 1 {
		t.Errorf("cursor.col = %d after l, want 1", m.cursor.col)
	}

	m = applyMsg(t, m, keyMsg("k"))
	if m.cursor.line != 0 {
		t.Errorf("cursor.line = %d after k, want 0", m.cursor.line)
	}

	m = applyMsg(t, m, keyMsg("$"))
	if m.cursor.col != 4 {
		t.Errorf("cursor.col = %d after $, want the last character of alpha", m.cursor.col)
	}

	m = applyMsg(t, m, keyMsg("0"))
	if m.cursor.col != 0 {
		t.Errorf("cursor.col = %d after 0, want 0", m.cursor.col)
	}

	m = applyMsg(t, m, keyMsg("G"))
	if m.cursor.line != m.detailBuf.lines()-1 {
		t.Errorf("cursor.line = %d after G, want the last line", m.cursor.line)
	}

	m = applyMsg(t, m, keyMsg("g"))
	if m.cursor.line != 0 {
		t.Errorf("cursor.line = %d after g, want the first line", m.cursor.line)
	}
}

func TestCoalescedKeysMoveOncePerRune(t *testing.T) {
	m := bufferModel(t, "one", "two", "three", "four", "five")

	// Bubble Tea delivers held-down keys as a single multi-rune message. Each
	// rune has to count, or the cursor stalls exactly when it is being driven
	// hardest.
	m = applyMsg(t, m, tea_KeyRunes("jjj"))
	if m.cursor.line != 3 {
		t.Errorf("cursor.line = %d after a coalesced \"jjj\", want 3", m.cursor.line)
	}
}

func TestLeftAtColumnZeroLeavesThePane(t *testing.T) {
	m := bufferModel(t, "alpha", "beta")
	m.cursor = pos{0, 2}

	m = applyMsg(t, m, keyMsg("h"))
	if m.focus != focusDetail || m.cursor.col != 1 {
		t.Fatalf("h at column 2 should move left, got focus=%v col=%d", m.focus, m.cursor.col)
	}

	m = applyMsg(t, m, keyMsg("h"))
	if m.focus != focusDetail || m.cursor.col != 0 {
		t.Fatalf("h at column 1 should move left, got focus=%v col=%d", m.focus, m.cursor.col)
	}

	// Walking off the left edge is how you get back to the list.
	m = applyMsg(t, m, keyMsg("h"))
	if m.focus != focusList {
		t.Errorf("focus = %v after h at column 0, want the list", m.focus)
	}
}

func TestVisualModeToggles(t *testing.T) {
	m := bufferModel(t, "alpha", "beta")

	m = applyMsg(t, m, keyMsg("v"))
	if m.visual != visualChar {
		t.Errorf("visual = %v after v, want charwise", m.visual)
	}
	if !strings.Contains(m.status, "VISUAL") {
		t.Errorf("status = %q, want the mode announced", m.status)
	}

	// A second v leaves visual mode, as in vi.
	m = applyMsg(t, m, keyMsg("v"))
	if m.visual != visualNone {
		t.Errorf("visual = %v after a second v, want none", m.visual)
	}

	m = applyMsg(t, m, keyMsg("V"))
	if m.visual != visualLine {
		t.Errorf("visual = %v after V, want linewise", m.visual)
	}

	// Switching straight from linewise to charwise.
	m = applyMsg(t, m, keyMsg("v"))
	if m.visual != visualChar {
		t.Errorf("visual = %v after V then v, want charwise", m.visual)
	}
}

func TestVisualAnchorsAtTheCursor(t *testing.T) {
	m := bufferModel(t, "alpha", "beta", "gamma")
	m = applyMsg(t, m, keyMsg("j"))
	m = applyMsg(t, m, keyMsg("V"))

	if m.anchor != (pos{1, 0}) {
		t.Errorf("anchor = %v, want it set where the selection started", m.anchor)
	}

	m = applyMsg(t, m, keyMsg("j"))
	if got := m.selectedText(); got != "beta\ngamma" {
		t.Errorf("selectedText() = %q, want the selection extended from the anchor", got)
	}
}

func TestEscapeCancelsThenLeaves(t *testing.T) {
	m := bufferModel(t, "alpha", "beta")
	m = applyMsg(t, m, keyMsg("V"))

	m = applyMsg(t, m, keyMsg("esc"))
	if m.visual != visualNone {
		t.Errorf("visual = %v after esc, want the selection cancelled", m.visual)
	}
	if m.focus != focusDetail {
		t.Error("the first esc left the pane; it should only cancel the selection")
	}

	m = applyMsg(t, m, keyMsg("esc"))
	if m.focus != focusList {
		t.Errorf("focus = %v after a second esc, want the list", m.focus)
	}
}

func TestYankSummary(t *testing.T) {
	tests := []struct {
		text string
		mode visualMode
		want string
	}{
		{"hello", visualChar, "5 character(s)"},
		{"a\nb", visualChar, "2 line(s)"},
		{"one line", visualLine, "1 line(s)"},
	}
	for _, tt := range tests {
		if got := yankSummary(tt.text, tt.mode); !strings.Contains(got, tt.want) {
			t.Errorf("yankSummary(%q) = %q, want it to mention %q", tt.text, got, tt.want)
		}
	}
}

func TestSwitchingPackageClearsTheSelection(t *testing.T) {
	m := browseModel(t)
	m.focus = focusDetail
	m = applyMsg(t, m, keyMsg("V"))
	m = applyMsg(t, m, keyMsg("j"))

	if m.visual == visualNone {
		t.Fatal("no selection to clear")
	}

	// A different package means a different buffer, so an offset into the old
	// one would select something arbitrary.
	m.focus = focusList
	m = applyMsg(t, m, keyMsg("j"))

	if m.visual != visualNone {
		t.Errorf("visual = %v after the selection moved, want it cleared", m.visual)
	}
	if m.cursor != (pos{}) {
		t.Errorf("cursor = %v, want it reset for the new buffer", m.cursor)
	}
}

// runModel is a model sitting on the run screen with a finished build log,
// which is the state a user is in when they want to copy a failure.
func runModel(t *testing.T, lines ...string) Model {
	t.Helper()

	m := NewWithStyles(nil, DefaultStyles(), nil)
	m.screen = screenRun
	m.runDone = true
	m.logView.Width, m.logView.Height = 80, 10
	m.logLines = lines
	m.syncLog()
	return m
}

func TestRunScreenSelectionUsesTheLogBuffer(t *testing.T) {
	m := runModel(t, "first line", "second line", "third line")

	// The selection code reads whichever pane is on screen; on screenRun that
	// has to be the log, not the detail pane.
	if got := m.activeBuf().lines(); got < 3 {
		t.Fatalf("activeBuf() has %d lines, want at least 3", got)
	}
	if !strings.Contains(strings.Join(m.activeBuf().plain, "\n"), "second line") {
		t.Error("the log buffer does not contain the log")
	}
}

func TestRunScreenYankCopiesTheWholeLog(t *testing.T) {
	m := runModel(t, "step 1 ok", "step 2 failed", "exit status 1")

	next, _ := m.updateRun(keyMsg("y"))
	got := next.(Model)

	if !strings.Contains(got.status, "whole log") {
		t.Errorf("status = %q, want it to report the whole log was copied", got.status)
	}
	// 3 lines of input, so the summary must count them rather than say "1".
	if !strings.Contains(got.status, "3 lines") {
		t.Errorf("status = %q, want a line count of 3", got.status)
	}
}

func TestRunScreenVisualSelectionAndYank(t *testing.T) {
	m := runModel(t, "alpha", "bravo", "charlie")

	next, _ := m.updateRun(keyMsg("V"))
	m = next.(Model)
	if m.visual != visualLine {
		t.Fatalf("visual = %v, want visualLine", m.visual)
	}

	next, _ = m.updateRun(keyMsg("j"))
	m = next.(Model)
	if m.cursor.line != 1 {
		t.Fatalf("cursor.line = %d, want 1 after j", m.cursor.line)
	}

	// The selection spans the two lines the cursor moved over.
	if got := m.selectedText(); !strings.Contains(got, "alpha") || !strings.Contains(got, "bravo") {
		t.Errorf("selectedText() = %q, want both selected lines", got)
	}

	next, _ = m.updateRun(keyMsg("y"))
	m = next.(Model)
	if m.visual != visualNone {
		t.Error("the selection survived the yank")
	}
}

func TestRunScreenEscCancelsSelectionBeforeLeaving(t *testing.T) {
	m := runModel(t, "one", "two")

	next, _ := m.updateRun(keyMsg("v"))
	m = next.(Model)

	// First esc drops the selection and stays put.
	next, _ = m.updateRun(keyMsg("esc"))
	m = next.(Model)
	if m.visual != visualNone {
		t.Error("esc did not cancel the selection")
	}
	if m.screen != screenRun {
		t.Error("esc left the run screen while cancelling a selection")
	}

	// Second esc leaves.
	next, _ = m.updateRun(keyMsg("esc"))
	if next.(Model).screen != screenBrowse {
		t.Error("a second esc did not return to the list")
	}
}

func TestRunScreenWillNotLeaveWhileRunning(t *testing.T) {
	m := runModel(t, "compiling…")
	m.runDone = false

	next, _ := m.updateRun(keyMsg("esc"))
	got := next.(Model)
	if got.screen != screenRun {
		t.Error("esc left the run screen while the operation was still running")
	}
	if !strings.Contains(got.status, "still running") {
		t.Errorf("status = %q, want it to say why", got.status)
	}
}

// New output must not yank the ground out from under an active selection.
func TestNewOutputDoesNotScrollAnActiveSelection(t *testing.T) {
	m := runModel(t, "line one", "line two")

	next, _ := m.updateRun(keyMsg("V"))
	m = next.(Model)
	at := m.cursor.line

	m.logLines = append(m.logLines, "line three", "line four")
	m.syncLog()

	if m.cursor.line != at {
		t.Errorf("cursor moved from %d to %d when output arrived", at, m.cursor.line)
	}
	if m.visual != visualLine {
		t.Error("new output cancelled the selection")
	}
}

// Held keys arrive coalesced; motions have to see them one at a time.
func TestRunScreenHandlesCoalescedMotions(t *testing.T) {
	m := runModel(t, "a", "b", "c", "d", "e")

	next, _ := m.updateRun(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("jjj")})
	if got := next.(Model).cursor.line; got != 3 {
		t.Errorf("cursor.line = %d after \"jjj\", want 3", got)
	}
}
