package tui

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"
)

// visualMode is the vi-style selection mode of the detail pane.
type visualMode int

const (
	visualNone visualMode = iota
	// visualChar is `v`: the selection runs from one character to another.
	visualChar
	// visualLine is `V`: the selection covers whole lines.
	visualLine
)

// pos is a cursor position in the detail buffer. col counts runes, not bytes,
// so multi-byte text moves one visible character at a time.
type pos struct {
	line int
	col  int
}

// before reports whether p comes strictly before q in reading order.
func (p pos) before(q pos) bool {
	if p.line != q.line {
		return p.line < q.line
	}
	return p.col < q.col
}

// detailBuffer is the rendered detail pane: the styled lines that get drawn,
// and the plain text behind them.
//
// The plain lines are derived by stripping the ANSI sequences from the styled
// ones rather than rendered a second time. That guarantees the two stay exactly
// in step — one plain line per styled line, same character offsets — which is
// what selection and yanking depend on.
type detailBuffer struct {
	styled []string
	plain  []string
}

// newDetailBuffer splits a rendered pane into lines and derives the plain text.
//
// Trailing spaces are dropped: lipgloss pads every line out to the pane width,
// and that padding is a drawing artefact rather than content. Keeping it would
// put the cursor on blank columns, make `$` land in the padding, and yank a
// tail of spaces along with the text.
func newDetailBuffer(rendered string) detailBuffer {
	styled := strings.Split(rendered, "\n")
	plain := make([]string, len(styled))
	for i, line := range styled {
		plain[i] = strings.TrimRight(ansi.Strip(line), " ")
	}
	return detailBuffer{styled: styled, plain: plain}
}

// lines returns the number of lines in the buffer.
func (b detailBuffer) lines() int { return len(b.plain) }

// runes returns line i as runes, or nil when the line does not exist.
func (b detailBuffer) runes(i int) []rune {
	if i < 0 || i >= len(b.plain) {
		return nil
	}
	return []rune(b.plain[i])
}

// lineLen returns the length of line i in runes.
func (b detailBuffer) lineLen(i int) int { return len(b.runes(i)) }

// clamp pulls a position back inside the buffer.
func (b detailBuffer) clamp(p pos) pos {
	if b.lines() == 0 {
		return pos{}
	}
	if p.line < 0 {
		p.line = 0
	}
	if p.line >= b.lines() {
		p.line = b.lines() - 1
	}
	if p.col < 0 {
		p.col = 0
	}
	// The cursor may sit one past the last character on an empty line only.
	if max := b.lineLen(p.line); p.col > max {
		p.col = max
	}
	return p
}

// selectionRange normalises the cursor and anchor into an ordered pair.
// ok is false when nothing is selected.
//
// This and everything below it work on `activeBuf`, not on the detail pane
// specifically: the run screen's build log gets the same selection, the same
// motions and the same yank, from this one implementation.
func (m Model) selectionRange() (start, end pos, ok bool) {
	if m.visual == visualNone {
		return pos{}, pos{}, false
	}

	start, end = m.anchor, m.cursor
	if end.before(start) {
		start, end = end, start
	}
	if m.visual == visualLine {
		start.col = 0
		end.col = m.activeBuf().lineLen(end.line)
	}
	return start, end, true
}

// selectedText returns the text the current selection covers. With no visual
// mode active it returns the cursor line, which is what `y` yanks then — the
// same shorthand vi spells `yy`.
func (m Model) selectedText() string {
	buf := m.activeBuf()
	start, end, ok := m.selectionRange()
	if !ok {
		line := buf.runes(m.cursor.line)
		return strings.TrimRight(string(line), " ")
	}

	if m.visual == visualLine {
		var out []string
		for i := start.line; i <= end.line && i < buf.lines(); i++ {
			out = append(out, strings.TrimRight(buf.plain[i], " "))
		}
		return strings.Join(out, "\n")
	}

	// Character-wise: the first and last lines are partial, everything between
	// them is whole. The end column is inclusive, as in vi.
	if start.line == end.line {
		line := buf.runes(start.line)
		return string(sliceRunes(line, start.col, end.col+1))
	}

	var out []string
	first := buf.runes(start.line)
	out = append(out, string(sliceRunes(first, start.col, len(first))))
	for i := start.line + 1; i < end.line; i++ {
		out = append(out, buf.plain[i])
	}
	last := buf.runes(end.line)
	out = append(out, string(sliceRunes(last, 0, end.col+1)))

	return strings.Join(out, "\n")
}

// sliceRunes slices defensively, so a stale cursor cannot panic the UI.
func sliceRunes(r []rune, from, to int) []rune {
	if from < 0 {
		from = 0
	}
	if to > len(r) {
		to = len(r)
	}
	if from >= to {
		return nil
	}
	return r[from:to]
}

// renderDetailBuffer draws the buffer with the selection and the cursor applied.
// Selected text is drawn from the plain lines: a highlighted run reads as one
// block, and keeping the original colouring underneath it would fight the
// highlight rather than help.
func (m Model) renderDetailBuffer() string {
	b := m.activeBuf()
	if b.lines() == 0 {
		return ""
	}

	start, end, selecting := m.selectionRange()
	showCursor := m.screen == screenRun || m.focus == focusDetail

	out := make([]string, b.lines())
	for i, styled := range b.styled {
		switch {
		case selecting && i >= start.line && i <= end.line:
			out[i] = m.renderSelectedLine(i, start, end)
		case showCursor && !selecting && i == m.cursor.line:
			out[i] = m.renderCursorLine(i)
		default:
			out[i] = styled
		}
	}

	return strings.Join(out, "\n")
}

// renderSelectedLine draws the selected span of one line.
func (m Model) renderSelectedLine(i int, start, end pos) string {
	line := m.activeBuf().runes(i)

	from, to := 0, len(line)
	if m.visual == visualChar {
		if i == start.line {
			from = start.col
		}
		if i == end.line {
			to = end.col + 1
		}
	}
	if to > len(line) {
		to = len(line)
	}
	if from > to {
		from = to
	}

	selected := string(line[from:to])
	// An empty line still has to show that it is part of the selection.
	if selected == "" {
		selected = " "
	}

	return string(line[:from]) + m.styles.Selection.Render(selected) + string(line[to:])
}

// renderCursorLine draws the block cursor over a single character.
func (m Model) renderCursorLine(i int) string {
	line := m.activeBuf().runes(i)
	col := m.cursor.col

	if col >= len(line) {
		// Past the end of the line, or an empty one: the cursor is a block in
		// the first free column.
		return string(line) + m.styles.Selection.Render(" ")
	}

	return string(line[:col]) +
		m.styles.Selection.Render(string(line[col])) +
		string(line[col+1:])
}

// ---------------------------------------------------------------------------
// Clipboard
// ---------------------------------------------------------------------------

// copyToClipboard puts text on the system clipboard.
//
// The system clipboard is tried first, through whichever helper is installed
// (wl-copy, xclip, pbcopy…). When there is none — a bare SSH session is the
// usual case — it falls back to OSC 52, which asks the terminal emulator on the
// other end to do the copying instead.
func copyToClipboard(text string) error {
	systemErr := clipboard.WriteAll(text)
	if systemErr == nil {
		return nil
	}

	if err := osc52(text); err != nil {
		return fmt.Errorf("no clipboard available: %w", systemErr)
	}
	return nil
}

// osc52 writes the OSC 52 copy sequence to the terminal. It produces no visible
// output, so writing it between Bubble Tea frames is safe.
func osc52(text string) error {
	_, err := fmt.Fprintf(os.Stdout, "\x1b]52;c;%s\x07",
		base64.StdEncoding.EncodeToString([]byte(text)))
	return err
}

// yankSummary describes what was copied, for the status line.
func yankSummary(text string, mode visualMode) string {
	lines := strings.Count(text, "\n") + 1
	if mode == visualLine || lines > 1 {
		return fmt.Sprintf("Copied %d line(s)", lines)
	}
	return fmt.Sprintf("Copied %d character(s)", len([]rune(text)))
}
