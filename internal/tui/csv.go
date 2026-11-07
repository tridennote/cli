package tui

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/tridennote/cli/internal/api"
	"github.com/tridennote/cli/internal/tree"
)

const (
	csvMaxBytes = 2 << 20
	csvMaxRows  = 5000
	csvMaxCell  = 48
	// csvChrome is the title, header, rule, footer rule and two footer lines around the body rows.
	csvChrome = 6
)

var (
	csvRowBg    = lipgloss.Color("#23243a")
	csvColBg    = lipgloss.Color("#1d1e2b")
	csvCursorBg = lipgloss.Color("#3a3f80")
	csvMatchFg  = lipgloss.Color("#fbbf24")
	csvCursor   = lipgloss.NewStyle().Foreground(lipgloss.Color("#ffffff")).Background(csvCursorBg).Bold(true)
)

// csvTable is a parsed file; rows are padded to the widest row and the first one is the header.
type csvTable struct {
	rows      [][]string
	widths    []int
	numeric   []bool
	truncated bool
	err       error
}

type csvMsg struct {
	id   string
	data []byte
	err  error
}

func isCSV(node *tree.Node) bool {
	return node != nil && node.IsCSV
}

// tableNode is the CSV under the tree cursor, which the viewer shows in place of the open note.
func (m Model) tableNode() *tree.Node {
	if node := m.selected(); m.mode == modeBrowse && isCSV(node) {
		return node
	}
	return nil
}

// showCSV starts a CSV at its first cell and fetches it if needed.
func (m *Model) showCSV(node *tree.Node) tea.Cmd {
	if m.csvID != node.ID {
		m.csvID = node.ID
		m.csvTop, m.csvLeft, m.csvSelRow, m.csvSelCol = 0, 0, 0, 0
		m.closeCSVPrompt()
		m.csvQuery, m.csvMatches, m.csvRows = "", nil, nil
	}
	if _, cached := m.csvs[node.ID]; cached || m.loadingCSV == node.ID {
		return nil
	}
	m.loadingCSV = node.ID
	client, id, attachment := m.client, node.ID, node.AttachmentID
	return func() tea.Msg {
		data, err := client.Attachment(attachment, csvMaxBytes)
		return csvMsg{id: id, data: data, err: err}
	}
}

func (m Model) csvLoading() bool {
	node := m.tableNode()
	if node == nil {
		return false
	}
	_, ok := m.csvs[node.ID]
	return !ok
}

func (m Model) handleCSV(msg csvMsg) (tea.Model, tea.Cmd) {
	if m.loadingCSV == msg.id {
		m.loadingCSV = ""
	}
	if errors.Is(msg.err, api.ErrUnauthorized) {
		return m.handleAPIError(msg.err)
	}
	table := csvTable{err: msg.err}
	if msg.err == nil {
		table = parseCSV(msg.data)
	}
	m.csvs[msg.id] = table
	return m, nil
}

func parseCSV(data []byte) csvTable {
	r := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(data, []byte("\xEF\xBB\xBF"))))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	var table csvTable
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			table.err = err
			break
		}
		if len(table.rows) == csvMaxRows+1 {
			table.truncated = true
			break
		}
		table.rows = append(table.rows, row)
	}
	cols := 0
	for _, row := range table.rows {
		cols = max(cols, len(row))
	}
	table.widths = make([]int, cols)
	table.numeric = make([]bool, cols)
	for c := range cols {
		table.numeric[c] = true
	}
	for r, row := range table.rows {
		cells := make([]string, cols)
		for c := range cols {
			if c < len(row) {
				cells[c] = strings.Join(strings.Fields(row[c]), " ")
			}
			table.widths[c] = max(table.widths[c], min(csvMaxCell, lipgloss.Width(cells[c])))
			if r > 0 && cells[c] != "" {
				if _, err := strconv.ParseFloat(strings.ReplaceAll(cells[c], ",", ""), 64); err != nil {
					table.numeric[c] = false
				}
			}
		}
		table.rows[r] = cells
	}
	for c := range cols {
		table.widths[c] = max(1, table.widths[c])
	}
	return table
}

func (t csvTable) cols() int { return len(t.widths) }

func (t csvTable) bodyRows() int { return max(0, len(t.rows)-1) }

func (t csvTable) header(c int) string {
	if len(t.rows) == 0 || c >= len(t.rows[0]) || t.rows[0][c] == "" {
		return "column " + strconv.Itoa(c+1)
	}
	return t.rows[0][c]
}

// gutter is the width of the row-number column.
func (t csvTable) gutter() int {
	return len(strconv.Itoa(max(1, t.bodyRows())))
}

// avail is the width left for cells once the row numbers are drawn.
func (t csvTable) avail(width int) int {
	return max(1, width-t.gutter()-2)
}

// frozen reports whether the first column stays pinned while scrolling sideways.
func (t csvTable) frozen(avail int) bool {
	return t.cols() > 1 && t.widths[0]+3 <= avail/3
}

// firstScrollable is the leftmost column that scrolls; a pinned first column never does.
func (t csvTable) firstScrollable(avail int) int {
	if t.frozen(avail) {
		return 1
	}
	return 0
}

// span lists the columns shown when scrolled to start; the last one may be cut short.
func (t csvTable) span(start, avail int) (first, last int, fits bool) {
	first = max(start, t.firstScrollable(avail))
	if t.frozen(avail) {
		avail -= t.widths[0] + 3
	}
	used := 0
	last = first
	for c := first; c < t.cols(); c++ {
		if c > first {
			used += 3
		}
		if room := avail - used; c > first && room < min(4, t.widths[c]) {
			return first, last, false
		}
		used += t.widths[c]
		last = c
		if used >= avail {
			break
		}
	}
	return first, last, used <= avail
}

// maxLeft is the furthest the table scrolls right: the first start from which every remaining column fits.
func (t csvTable) maxLeft(avail int) int {
	for c := t.firstScrollable(avail); c < t.cols(); c++ {
		if _, last, fits := t.span(c, avail); last == t.cols()-1 && fits {
			return c
		}
	}
	return max(t.firstScrollable(avail), t.cols()-1)
}

func (m Model) csvBodyHeight() int {
	return max(1, m.viewer.Height-csvChrome)
}

// followCSV clamps the cursor to the table and scrolls so it stays in view.
func (m *Model) followCSV(table csvTable) {
	m.csvSelRow = max(0, min(m.csvSelRow, m.rowCount(table)-1))
	m.csvSelCol = max(0, min(m.csvSelCol, table.cols()-1))

	height := m.csvBodyHeight()
	if m.csvSelRow < m.csvTop {
		m.csvTop = m.csvSelRow
	}
	if m.csvSelRow >= m.csvTop+height {
		m.csvTop = m.csvSelRow - height + 1
	}
	m.csvTop = max(0, min(m.csvTop, m.rowCount(table)-height))

	avail := table.avail(m.viewer.Width)
	first := table.firstScrollable(avail)
	m.csvLeft = max(first, min(m.csvLeft, table.maxLeft(avail)))
	if m.csvSelCol < first {
		return
	}
	if m.csvSelCol < m.csvLeft {
		m.csvLeft = m.csvSelCol
	}
	for m.csvLeft < m.csvSelCol {
		_, last, fits := table.span(m.csvLeft, avail)
		if m.csvSelCol < last || (m.csvSelCol == last && fits) {
			break
		}
		m.csvLeft++
	}
}

func (m Model) csvView(node *tree.Node) string {
	table, ok := m.csvs[node.ID]
	if !ok {
		return "\n" + m.miniLoader(loaderNote)
	}
	if table.err != nil && len(table.rows) == 0 {
		msg := "Could not read " + node.Label() + ": " + table.err.Error()
		if errors.Is(table.err, api.ErrTooLarge) {
			msg = node.Label() + " is too large to show here"
		}
		return "\n" + errorStyle.Render("✕ "+msg) + "\n\n" + keyStyle.Render("o") + mutedStyle.Render(" open in browser")
	}
	if len(table.rows) == 0 {
		return titleStyle.Render(node.Label()) + "\n\n" + mutedStyle.Render("Empty file")
	}
	if m.csvID != node.ID {
		m.csvTop, m.csvLeft, m.csvSelRow, m.csvSelCol = 0, 0, 0, 0
	}
	m.followCSV(table)
	return m.renderCSV(table, node.Label())
}

// renderCSV draws the visible window of the table: row numbers, a pinned header (and narrow
// first column), the cursor's row and column highlighted, and the footer.
func (m Model) renderCSV(table csvTable, title string) string {
	width, height := m.viewer.Width, m.csvBodyHeight()
	avail := table.avail(width)
	frozen := table.frozen(avail)
	first, last, fits := table.span(m.csvLeft, avail)
	gutter := table.gutter()
	focused := m.focus == focusViewer
	matches := make(map[[2]int]bool, len(m.csvMatches))
	for _, at := range m.csvMatches {
		matches[at] = true
	}

	// Rows are drawn by position among the shown rows; a search hides the rest.
	cell := func(pos, c int) string {
		header := pos < 0
		row := -1
		if !header {
			row = m.rowAt(pos)
		}
		raw := table.rows[row+1][c]
		w := table.widths[c]
		shown := ansi.Truncate(raw, w, "…")
		pad := strings.Repeat(" ", w-lipgloss.Width(shown))
		if table.numeric[c] && !header {
			shown = pad + shown
		} else {
			shown += pad
		}
		selRow, selCol := !header && pos == m.csvSelRow, c == m.csvSelCol
		style := lipgloss.NewStyle().Foreground(text)
		switch {
		case header && selCol && focused:
			style = csvCursor
		case header:
			style = accentStyle
		case selRow && selCol && focused:
			style = csvCursor
		case selRow && focused:
			style = style.Background(csvRowBg)
		case selCol && focused:
			style = style.Background(csvColBg)
		case frozen && c == 0:
			style = mutedStyle
		}
		if !header && matches[[2]int{row, c}] {
			style = style.Foreground(csvMatchFg).Bold(true)
		}
		return style.Render(shown)
	}
	sep := func(pos int, glyph string) string {
		style := faintStyle
		if pos >= 0 && pos == m.csvSelRow && focused {
			style = style.Background(csvRowBg)
		}
		return style.Render(glyph)
	}
	line := func(pos int) string {
		var b strings.Builder
		number := "#"
		if pos >= 0 {
			number = strconv.Itoa(m.rowAt(pos) + 1)
		}
		number = strings.Repeat(" ", gutter-len(number)) + number
		if pos >= 0 && pos == m.csvSelRow {
			b.WriteString(accentStyle.Render(number))
		} else {
			b.WriteString(faintStyle.Render(number))
		}
		b.WriteString("  ")
		if frozen {
			b.WriteString(cell(pos, 0) + sep(pos, " ┃ "))
		}
		for c := first; c <= last; c++ {
			if c > first {
				b.WriteString(sep(pos, " │ "))
			}
			b.WriteString(cell(pos, c))
		}
		return ansi.Truncate(b.String(), width, "")
	}

	var parts []string
	for c := first; c <= last; c++ {
		parts = append(parts, strings.Repeat("─", table.widths[c]))
	}
	rule := strings.Join(parts, "─┼─")
	if frozen {
		rule = strings.Repeat("─", table.widths[0]) + "─╂─" + rule
	}
	rule = strings.Repeat("─", gutter) + "──" + rule

	body := m.rowCount(table)
	position := fmt.Sprintf("row %d of %d · col %d of %d", min(m.rowAt(m.csvSelRow)+1, body), body, m.csvSelCol+1, table.cols())
	if m.csvRows != nil {
		position = fmt.Sprintf("%d of %d rows · col %d of %d", body, table.bodyRows(), m.csvSelCol+1, table.cols())
	}
	head := titleStyle.Render(ansi.Truncate(title, max(0, width-lipgloss.Width(position)-2), "…"))
	head += strings.Repeat(" ", max(2, width-lipgloss.Width(head)-lipgloss.Width(position))) + faintStyle.Render(position)

	lines := []string{ansi.Truncate(head, width, ""), line(-1), faintStyle.Render(ansi.Truncate(rule, width, ""))}
	end := min(body, m.csvTop+height)
	for pos := m.csvTop; pos < end; pos++ {
		lines = append(lines, line(pos))
	}
	filler := height - (end - m.csvTop)
	if body == 0 && m.csvRows != nil {
		lines = append(lines, strings.Repeat(" ", gutter+2)+mutedStyle.Render("No rows match "+m.csvQuery))
		filler--
	}
	for range filler {
		lines = append(lines, "")
	}

	var more []string
	if m.csvRows != nil {
		more = append(more, "showing rows matching "+m.csvQuery)
	}
	if first > table.firstScrollable(avail) {
		more = append(more, "◂ more")
	}
	if last < table.cols()-1 || !fits {
		more = append(more, "more ▸")
	}
	if table.truncated {
		more = append(more, fmt.Sprintf("first %d rows only", csvMaxRows))
	}
	if table.err != nil {
		more = append(more, "stopped at a malformed line")
	}
	divider := strings.Repeat("─", width)
	if len(more) > 0 {
		divider = "── " + strings.Join(more, " · ") + " " + divider
	}
	top, bottom := m.csvFooter(table, width)
	lines = append(lines,
		faintStyle.Render(ansi.Truncate(divider, width, "")),
		ansi.Truncate(top, width, "…"),
		ansi.Truncate(bottom, width, "…"),
	)
	return strings.Join(lines, "\n")
}

// csvFooter is the two lines under the table: the find or go-to prompt, or the cursor's full value and hints.
func (m Model) csvFooter(table csvTable, width int) (string, string) {
	switch m.csvPrompt {
	case csvPromptFind:
		return accentStyle.Render("find › ") + m.csvInput.View(), m.findStatus(table)
	case csvPromptGoto:
		status := faintStyle.Render(fmt.Sprintf("1–%d · ⏎ go · esc cancel", table.bodyRows()))
		if m.csvNotice != "" {
			status = errorStyle.Render(m.csvNotice)
		}
		return accentStyle.Render("go to row › ") + m.csvInput.View(), status
	}

	if m.rowCount(table) == 0 {
		return mutedStyle.Render("No rows to show"), faintStyle.Render("f edit search · esc clear")
	}
	row := m.rowAt(m.csvSelRow)
	label := keyStyle.Render(fmt.Sprintf("%d:%d", row+1, m.csvSelCol+1)) + " " + mutedStyle.Render(table.header(m.csvSelCol)+":") + " "
	value := faintStyle.Render("empty")
	if v := table.rows[row+1][m.csvSelCol]; v != "" {
		value = v
	}
	top := label + ansi.Truncate(value, max(0, width-lipgloss.Width(label)), "…")

	hints := "⏎ to explore the table"
	switch {
	case m.focus == focusViewer && m.csvQuery != "":
		hints = m.matchSummary() + " · n/N next/prev match · f edit search · esc show all rows"
	case m.focus == focusViewer:
		hints = "f find · ctrl+g go to row · shift+arrows page · esc back"
	}
	return top, faintStyle.Render(hints)
}
