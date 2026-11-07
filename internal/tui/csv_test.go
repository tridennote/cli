package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// fakeCSV is wide and long enough to scroll both ways in the test terminal.
func fakeCSV() string {
	var b strings.Builder
	b.WriteString("\xEF\xBB\xBFmonth,trips,driver,pickup address,dropoff address,vehicle plate,status,notes\n")
	statuses := []string{"Completed", "Cancelled", "Completed", "Pending"}
	for i := 1; i <= 60; i++ {
		fmt.Fprintf(&b, "M%02d,%d,\"Aiman, B.\",Lot %d Jalan Ampang Kuala Lumpur,KLIA Terminal 1 Departure Hall,WXY %04d,%s,row%02d-end\n",
			i, 1000+i, i, i, statuses[i%len(statuses)], i)
	}
	return b.String()
}

func openCSV(t *testing.T) Model {
	t.Helper()
	m, srv := signedIn(t, &fakeServer{csv: true})
	t.Cleanup(srv.Close)
	// Rows: Projects/, Daily log.md, yearly.csv, divider, invoice.pdf, Whiteboard.excalidraw.
	m, _ = step(t, m, key("down"))
	m, cmd := step(t, m, key("down"))
	if node := m.selected(); node == nil || node.ID != "c1" {
		t.Fatalf("yearly.csv should sit in the note tree, got %+v", node)
	}
	m, cmd = step(t, m, cmd())
	if cmd == nil {
		t.Fatalf("previewing a csv should download it")
	}
	m, _ = step(t, m, cmd())
	return m
}

func TestCSVSitsInTreeAndRendersTable(t *testing.T) {
	opened := stubBrowser(t)
	m := openCSV(t)

	view := ansi.Strip(m.View())
	divider := strings.Index(view, "─ in browser")
	if at := strings.Index(view, "yearly.csv"); at < 0 || at > divider {
		t.Fatalf("yearly.csv should be listed above the browser-only files:\n%s", view)
	}
	for _, want := range []string{"#  month", " 1  M01", "pickup address", "Aiman, B.", "row 1 of 60", "1:1 month: M01", "⏎ scroll", "o browser"} {
		if !strings.Contains(view, want) {
			t.Fatalf("csv view missing %q:\n%s", want, view)
		}
	}

	m, _ = step(t, m, key("enter"))
	if m.focus != focusViewer || len(*opened) != 0 {
		t.Fatalf("enter should focus the table, not open the browser")
	}
	m, _ = step(t, m, key("e"))
	if !strings.Contains(m.flash, "read-only") {
		t.Fatalf("e on a csv should explain it can't be edited, flash = %q", m.flash)
	}
	m, _ = step(t, m, key("o"))
	if len(*opened) != 1 || (*opened)[0] != m.client.BaseURL+"/note/c1" {
		t.Fatalf("o should still open the csv in the browser: %v", *opened)
	}
}

func TestCSVCursorMovesAndScrolls(t *testing.T) {
	m := openCSV(t)
	m, _ = step(t, m, key("enter"))

	m, _ = step(t, m, key("j"))
	m, _ = step(t, m, key("l"))
	if m.csvSelRow != 1 || m.csvSelCol != 1 {
		t.Fatalf("cursor = %d,%d", m.csvSelRow, m.csvSelCol)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "2:2 trips: 1002") {
		t.Fatalf("footer should show the current cell's value:\n%s", view)
	}

	page := m.csvBodyHeight()
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyShiftDown})
	if m.csvSelRow != 1+page || m.csvTop != page {
		t.Fatalf("shift+down should skip a page: row %d top %d (page %d)", m.csvSelRow, m.csvTop, page)
	}
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyShiftUp})
	if m.csvSelRow != 1 || m.csvTop != 0 {
		t.Fatalf("shift+up should come back a page: row %d top %d", m.csvSelRow, m.csvTop)
	}

	m, _ = step(t, m, key("G"))
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "60  M60") || !strings.Contains(view, "pickup address") {
		t.Fatalf("G should reach the last row with the header still pinned:\n%s", view)
	}
	for range 10 {
		m, _ = step(t, m, key("k"))
	}
	if m.csvSelRow != 49 || m.csvTop > 49 || m.csvTop+page <= 49 {
		t.Fatalf("moving up should keep the cursor on screen: row %d top %d", m.csvSelRow, m.csvTop)
	}

	for range 3 {
		m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyShiftRight})
	}
	view = ansi.Strip(m.View())
	if m.csvSelCol != 7 || !strings.Contains(view, "row50-end") || !strings.Contains(view, "◂ more") {
		t.Fatalf("shift+right should page across to the last column (col %d):\n%s", m.csvSelCol, view)
	}
	if !strings.Contains(view, "50  M50") {
		t.Fatalf("row numbers and the narrow first column should stay pinned:\n%s", view)
	}

	m, _ = step(t, m, key("0"))
	for i := 0; i < 5 && m.focus == focusViewer; i++ {
		m, _ = step(t, m, key("left"))
	}
	if m.csvSelCol != 0 || m.focus != focusTree {
		t.Fatalf("left from the first column should return to the tree (col %d, focus %v)", m.csvSelCol, m.focus)
	}
}

func TestCSVGotoRow(t *testing.T) {
	m := openCSV(t)
	m, _ = step(t, m, key("enter"))

	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyCtrlG})
	if m.csvPrompt != csvPromptGoto || !strings.Contains(ansi.Strip(m.View()), "go to row ›") {
		t.Fatalf("ctrl+g should open the go-to prompt at the bottom")
	}
	m = typeText(t, m, "99")
	m, _ = step(t, m, key("enter"))
	if m.csvPrompt != csvPromptGoto || !strings.Contains(ansi.Strip(m.View()), "between 1 and 60") {
		t.Fatalf("out-of-range rows should be refused")
	}
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
	m = typeText(t, m, "42")
	m, _ = step(t, m, key("enter"))
	if m.csvPrompt != csvPromptNone || m.csvSelRow != 41 {
		t.Fatalf("enter should jump to row 42, got %d", m.csvSelRow+1)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "42  M42") {
		t.Fatalf("row 42 should be on screen:\n%s", view)
	}
}

func TestCSVFind(t *testing.T) {
	m := openCSV(t)
	m, _ = step(t, m, key("enter"))
	row := func() int { return m.cursorCell()[0] + 1 }

	m, _ = step(t, m, key("f"))
	if m.csvPrompt != csvPromptFind || !strings.Contains(ansi.Strip(m.View()), "find ›") {
		t.Fatalf("f should open the find bar at the bottom")
	}

	// "sta" completes to the column name, then "Can" to a value in that column.
	m = typeText(t, m, "sta")
	if got := m.csvInput.CurrentSuggestion(); got != "status: " {
		t.Fatalf("column name should autocomplete, suggestion = %q", got)
	}
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = typeText(t, m, "Can")
	if got := m.csvInput.CurrentSuggestion(); got != "status: Cancelled" {
		t.Fatalf("values of the column should autocomplete, suggestion = %q", got)
	}
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.csvQuery != "status: Cancelled" || len(m.csvMatches) != 15 || m.rowCount(m.csvs["c1"]) != 15 {
		t.Fatalf("query %q matched %d cells in %d rows, want 15", m.csvQuery, len(m.csvMatches), m.rowCount(m.csvs["c1"]))
	}
	if row() != 1 || m.csvSelCol != 6 {
		t.Fatalf("the first match should be selected, got %d,%d", row(), m.csvSelCol)
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{"15 of 60 rows", " 1  M01", " 5  M05", " 9  M09", "in status · 1 of 15 matches"} {
		if !strings.Contains(view, want) {
			t.Fatalf("filtered view missing %q:\n%s", want, view)
		}
	}
	for _, hidden := range []string{"M02", "M03", "M04", "Completed", "Pending"} {
		if strings.Contains(view, hidden) {
			t.Fatalf("rows that don't match should be hidden, found %q:\n%s", hidden, view)
		}
	}

	m, _ = step(t, m, key("enter"))
	if row() != 5 {
		t.Fatalf("enter should go to the next match, got row %d", row())
	}
	m, _ = step(t, m, key("esc"))
	if m.csvPrompt != csvPromptNone || m.csvQuery == "" {
		t.Fatalf("esc should close the bar but keep the filter")
	}
	m, _ = step(t, m, key("j"))
	if row() != 9 {
		t.Fatalf("j should move to the next shown row, got row %d", row())
	}
	m, _ = step(t, m, key("N"))
	if row() != 5 {
		t.Fatalf("N should go back a match, got row %d", row())
	}

	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyCtrlG})
	m = typeText(t, m, "2")
	m, _ = step(t, m, key("enter"))
	if !strings.Contains(ansi.Strip(m.View()), "Row 2 doesn't match") {
		t.Fatalf("going to a hidden row should explain why")
	}
	m, _ = step(t, m, key("esc"))

	m, _ = step(t, m, key("esc"))
	if m.csvQuery != "" || m.csvRows != nil || m.focus != focusViewer || row() != 5 {
		t.Fatalf("esc should show every row again and stay on row 5, got row %d", row())
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "M02") {
		t.Fatalf("all rows should be back:\n%s", view)
	}

	// A plain query searches every column.
	m, _ = step(t, m, key("f"))
	m = typeText(t, m, "row07")
	if len(m.csvMatches) != 1 || row() != 7 || m.csvSelCol != 7 {
		t.Fatalf("plain search should find row07-end, got %v at %d,%d", m.csvMatches, row(), m.csvSelCol)
	}
	m = typeText(t, m, "zzz")
	if view := ansi.Strip(m.View()); !strings.Contains(view, "No rows match row07zzz") || !strings.Contains(view, "0 of 60 rows") {
		t.Fatalf("a search with no hits should say so:\n%s", view)
	}
}

func TestCSVColumnsFitContent(t *testing.T) {
	table := parseCSV([]byte("id,description,amount\n1,a fairly long description of the trip,12.5\n2,short,3\n"))
	if table.widths[1] != len("a fairly long description of the trip") {
		t.Fatalf("columns should be as wide as their content, got %v", table.widths)
	}
	huge := parseCSV([]byte("a\n" + strings.Repeat("x", 200) + "\n"))
	if huge.widths[0] != csvMaxCell {
		t.Fatalf("very long cells should be capped at %d, got %d", csvMaxCell, huge.widths[0])
	}
}

func TestParseCSVToleratesRaggedRows(t *testing.T) {
	table := parseCSV([]byte("a,b,c\n1,2\n3,4,5,6\n"))
	if table.err != nil || len(table.rows) != 3 || table.cols() != 4 {
		t.Fatalf("ragged rows should parse: %+v", table)
	}
}
