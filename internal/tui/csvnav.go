package tui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type csvPromptKind int

const (
	csvPromptNone csvPromptKind = iota
	csvPromptFind
	csvPromptGoto
)

const csvMaxSuggestions = 300

func newCSVInput() textinput.Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 200
	ti.TextStyle = lipgloss.NewStyle().Foreground(text)
	ti.PlaceholderStyle = faintStyle
	ti.CompletionStyle = faintStyle
	return ti
}

// rowCount is how many body rows are shown: all of them, or those a search matched.
func (m Model) rowCount(table csvTable) int {
	if m.csvRows != nil {
		return len(m.csvRows)
	}
	return table.bodyRows()
}

// rowAt maps a position among the shown rows to the row's index in the file.
func (m Model) rowAt(pos int) int {
	if m.csvRows == nil {
		return pos
	}
	if pos < 0 || pos >= len(m.csvRows) {
		return -1
	}
	return m.csvRows[pos]
}

// posOf maps a file row to its position among the shown rows, or -1 when a search hides it.
func (m Model) posOf(row int) int {
	if m.csvRows == nil {
		return row
	}
	if i := sort.SearchInts(m.csvRows, row); i < len(m.csvRows) && m.csvRows[i] == row {
		return i
	}
	return -1
}

// shownTable is the loaded CSV the viewer is showing, if any.
func (m Model) shownTable() (csvTable, bool) {
	node := m.tableNode()
	if node == nil {
		return csvTable{}, false
	}
	table, ok := m.csvs[node.ID]
	return table, ok && len(table.rows) > 0
}

// handleCSVKey moves around the focused table; it reports false for keys the table doesn't use.
func (m *Model) handleCSVKey(key string) (bool, tea.Cmd) {
	table, ok := m.shownTable()
	if !ok || m.focus != focusViewer {
		return false, nil
	}
	m.followCSV(table)
	page := m.csvBodyHeight()
	switch key {
	case "down", "j":
		m.csvSelRow++
	case "up", "k":
		m.csvSelRow--
	case "right", "l":
		m.csvSelCol++
	case "left", "h":
		if m.csvSelCol == 0 {
			return false, nil
		}
		m.csvSelCol--
	case "shift+down", "pgdown", " ":
		m.csvSelRow += page
		m.csvTop += page
	case "shift+up", "pgup":
		m.csvSelRow -= page
		m.csvTop -= page
	case "ctrl+d":
		m.csvSelRow += page / 2
		m.csvTop += page / 2
	case "ctrl+u":
		m.csvSelRow -= page / 2
		m.csvTop -= page / 2
	case "shift+right", "shift+left":
		first, last, _ := table.span(m.csvLeft, table.avail(m.viewer.Width))
		step := max(1, last-first+1)
		if key == "shift+left" {
			step = -step
		}
		m.csvSelCol += step
		m.csvLeft += step
	case "g", "home":
		m.csvSelRow = 0
	case "G", "end":
		m.csvSelRow = m.rowCount(table) - 1
	case "0", "^":
		m.csvSelCol = 0
	case "$":
		m.csvSelCol = table.cols() - 1
	case "f", "/":
		return true, m.openCSVFind(table)
	case "ctrl+g":
		return true, m.openCSVGoto()
	case "n", "N":
		if m.csvQuery == "" {
			return false, nil
		}
		m.jumpToMatch(m.cursorCell(), key == "n", false)
	case "esc":
		if m.csvQuery == "" {
			return false, nil
		}
		row := max(0, m.rowAt(m.csvSelRow))
		m.csvQuery, m.csvMatches, m.csvRows = "", nil, nil
		m.csvSelRow = row
	case "enter":
		m.flash = "Cells are read-only in the terminal for now"
	default:
		return false, nil
	}
	m.followCSV(table)
	return true, nil
}

func (m *Model) openCSVFind(table csvTable) tea.Cmd {
	m.csvPrompt = csvPromptFind
	m.csvOrigin = [2]int{max(0, m.rowAt(m.csvSelRow)), m.csvSelCol}
	m.csvInput.Placeholder = "search, or Column: value"
	m.csvInput.ShowSuggestions = true
	m.csvInput.SetValue(m.csvQuery)
	m.csvInput.SetSuggestions(csvSuggestions(table, m.csvQuery))
	m.csvInput.CursorEnd()
	return m.csvInput.Focus()
}

func (m *Model) openCSVGoto() tea.Cmd {
	m.csvPrompt = csvPromptGoto
	m.csvNotice = ""
	m.csvInput.Placeholder = "row number"
	m.csvInput.ShowSuggestions = false
	m.csvInput.SetSuggestions(nil)
	m.csvInput.SetValue("")
	return m.csvInput.Focus()
}

func (m *Model) closeCSVPrompt() {
	m.csvPrompt = csvPromptNone
	m.csvNotice = ""
	m.csvInput.Blur()
}

func (m *Model) handleCSVPromptKey(msg tea.KeyMsg) tea.Cmd {
	table, ok := m.shownTable()
	if !ok {
		m.closeCSVPrompt()
		return nil
	}
	switch msg.String() {
	case "esc":
		m.closeCSVPrompt()
		return nil
	case "enter":
		if m.csvPrompt == csvPromptGoto {
			m.gotoRow(table)
			return nil
		}
		m.jumpToMatch(m.cursorCell(), true, false)
		m.followCSV(table)
		return nil
	}
	before := m.csvInput.Value()
	var cmd tea.Cmd
	m.csvInput, cmd = m.csvInput.Update(msg)
	if value := m.csvInput.Value(); value != before {
		if m.csvPrompt == csvPromptGoto {
			m.csvNotice = ""
			return cmd
		}
		m.csvInput.SetSuggestions(csvSuggestions(table, value))
		m.setCSVQuery(table, value)
		if m.csvRows == nil {
			m.csvSelRow, m.csvSelCol = m.csvOrigin[0], m.csvOrigin[1]
		} else {
			m.csvSelRow = 0
			m.jumpToMatch(m.csvOrigin, true, true)
		}
		m.followCSV(table)
	}
	return cmd
}

func (m *Model) gotoRow(table csvTable) {
	n, err := strconv.Atoi(strings.TrimSpace(m.csvInput.Value()))
	if err != nil || n < 1 || n > table.bodyRows() {
		m.csvNotice = fmt.Sprintf("Enter a row between 1 and %d", table.bodyRows())
		return
	}
	pos := m.posOf(n - 1)
	if pos < 0 {
		m.csvNotice = fmt.Sprintf("Row %d doesn't match %q — esc and clear the search to see it", n, m.csvQuery)
		return
	}
	m.csvSelRow = pos
	m.closeCSVPrompt()
	m.followCSV(table)
}

func (m *Model) setCSVQuery(table csvTable, value string) {
	m.csvQuery = strings.TrimSpace(value)
	col, needle := parseCSVQuery(table, m.csvQuery)
	m.csvMatches, m.csvRows = nil, nil
	if needle == "" {
		return
	}
	m.csvRows = []int{}
	for r, row := range table.rows[1:] {
		hit := false
		for c, v := range row {
			if (col < 0 || c == col) && strings.Contains(strings.ToLower(v), needle) {
				m.csvMatches = append(m.csvMatches, [2]int{r, c})
				hit = true
			}
		}
		if hit {
			m.csvRows = append(m.csvRows, r)
		}
	}
}

// cursorCell is the cursor's cell in file coordinates.
func (m Model) cursorCell() [2]int {
	return [2]int{m.rowAt(m.csvSelRow), m.csvSelCol}
}

// jumpToMatch moves the cursor to the match after (or before) the file cell at, wrapping around;
// inclusive also accepts a match on at itself.
func (m *Model) jumpToMatch(at [2]int, forward, inclusive bool) {
	if len(m.csvMatches) == 0 {
		return
	}
	after := func(a, b [2]int) bool { return a[0] > b[0] || (a[0] == b[0] && a[1] > b[1]) }
	pick := -1
	if forward {
		for i, match := range m.csvMatches {
			if after(match, at) || (inclusive && match == at) {
				pick = i
				break
			}
		}
		if pick < 0 {
			pick = 0
		}
	} else {
		for i := len(m.csvMatches) - 1; i >= 0; i-- {
			if after(at, m.csvMatches[i]) {
				pick = i
				break
			}
		}
		if pick < 0 {
			pick = len(m.csvMatches) - 1
		}
	}
	m.csvSelRow, m.csvSelCol = m.posOf(m.csvMatches[pick][0]), m.csvMatches[pick][1]
}

func (m Model) matchSummary() string {
	switch len(m.csvMatches) {
	case 0:
		return "no matches"
	case 1:
		return "1 match"
	}
	for i, match := range m.csvMatches {
		if match == m.cursorCell() {
			return fmt.Sprintf("%d of %d matches", i+1, len(m.csvMatches))
		}
	}
	return fmt.Sprintf("%d matches", len(m.csvMatches))
}

func (m Model) findStatus(table csvTable) string {
	col, needle := parseCSVQuery(table, m.csvQuery)
	var parts []string
	switch {
	case col >= 0 && needle == "":
		parts = append(parts, mutedStyle.Render("type a value to find in "+table.header(col)))
	case col >= 0:
		parts = append(parts, mutedStyle.Render("in "+table.header(col)+" · "+m.matchSummary()))
	case needle != "":
		parts = append(parts, mutedStyle.Render(m.matchSummary()))
	}
	parts = append(parts, faintStyle.Render("tab complete · ↑↓ suggestions · ⏎ next · esc done"))
	return strings.Join(parts, faintStyle.Render(" · "))
}

// parseCSVQuery splits "Column: value" into a column index and a lowercase needle;
// without a known column it searches every cell.
func parseCSVQuery(table csvTable, query string) (int, string) {
	if i := strings.Index(query, ":"); i >= 0 {
		if col := csvColumn(table, query[:i]); col >= 0 {
			return col, strings.ToLower(strings.TrimSpace(query[i+1:]))
		}
	}
	return -1, strings.ToLower(strings.TrimSpace(query))
}

func csvColumn(table csvTable, name string) int {
	name = strings.TrimSpace(name)
	for c := range table.cols() {
		if strings.EqualFold(table.header(c), name) {
			return c
		}
	}
	return -1
}

// csvSuggestions completes what's being typed: column names as "Column: ", then the
// values of that column once one is named, or any cell value otherwise, most common first.
func csvSuggestions(table csvTable, value string) []string {
	if i := strings.Index(value, ":"); i >= 0 {
		if col := csvColumn(table, value[:i]); col >= 0 {
			rest := value[i+1:]
			lead := rest[:len(rest)-len(strings.TrimLeftFunc(rest, unicode.IsSpace))]
			if rest == "" {
				lead = " "
			}
			prefix := value[:i+1] + lead
			var out []string
			for _, v := range commonValues(table, col) {
				out = append(out, prefix+v)
			}
			return out
		}
	}
	var out []string
	for c := range table.cols() {
		out = append(out, table.header(c)+": ")
	}
	return append(out, commonValues(table, -1)...)
}

func commonValues(table csvTable, col int) []string {
	counts := map[string]int{}
	var order []string
	for _, row := range table.rows[1:] {
		for c, v := range row {
			if v == "" || (col >= 0 && c != col) {
				continue
			}
			if counts[v] == 0 {
				order = append(order, v)
			}
			counts[v]++
		}
	}
	sort.SliceStable(order, func(i, j int) bool { return counts[order[i]] > counts[order[j]] })
	return order[:min(len(order), csvMaxSuggestions)]
}
