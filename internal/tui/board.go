package tui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/tridennote/cli/internal/api"
	"github.com/tridennote/cli/internal/kanban"
)

type boardPrompt int

const (
	promptNone boardPrompt = iota
	promptAddCard
	promptEditCard
	promptAddColumn
)

type boardSavedMsg struct {
	id   string
	note api.Note
	err  error
}

const (
	boardColumnGap   = 2
	boardColumnMin   = 18
	boardHeaderLines = 2
)

var (
	priorityStyles = map[string]lipgloss.Style{
		"critical": lipgloss.NewStyle().Foreground(danger).Bold(true),
		"high":     lipgloss.NewStyle().Foreground(lipgloss.Color("#fb923c")).Bold(true),
		"medium":   lipgloss.NewStyle().Foreground(lipgloss.Color("#facc15")),
		"low":      lipgloss.NewStyle().Foreground(muted),
	}
	tagStyle      = lipgloss.NewStyle().Foreground(violet)
	assigneeStyle = lipgloss.NewStyle().Foreground(mint)
	doneStyle     = lipgloss.NewStyle().Foreground(faint).Strikethrough(true)
	selectedBar   = lipgloss.NewStyle().Foreground(violet).Bold(true)
	columnRule    = lipgloss.NewStyle().Foreground(indigo)
)

func newBoardInput() textinput.Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 500
	ti.TextStyle = lipgloss.NewStyle().Foreground(text)
	return ti
}

func (m Model) isBoard(id string) bool {
	if id == "" {
		return false
	}
	if note, ok := m.notes[id]; ok && kanban.IsBoardName(note.Title) {
		return true
	}
	if node := m.findNode(id); node != nil {
		return node.IsBoard
	}
	return false
}

func (m Model) showsBoard(id string) bool {
	return !m.rawBoards && m.isBoard(id)
}

func (m Model) boardTitle(id string) string {
	return kanban.DisplayName(m.nodeName(id))
}

func (m *Model) toggleBoardView() {
	if !m.isBoard(m.openID) && !m.isBoard(m.targetNote()) {
		m.flash = "v switches .kanban notes between board and markdown"
		return
	}
	m.rawBoards = !m.rawBoards
	if m.rawBoards {
		m.flash = "Showing boards as markdown · v for the board"
	} else {
		m.flash = "Showing boards as boards · v for markdown"
	}
	m.refreshViewer(true)
}

func (m *Model) requestBoard(id string) tea.Cmd {
	if _, ok := m.notes[id]; !ok {
		m.pending = &pendingOpen{id: id, action: pendingBoard}
		m.flash = "Loading board…"
		return m.open(id)
	}
	m.enterBoard(id)
	return nil
}

func (m *Model) enterBoard(id string) {
	m.openID = id
	m.boardID = id
	m.board = kanban.Parse(m.notes[id].Blocks)
	m.boardCol, m.boardRow = 0, 0
	m.boardPrompt = promptNone
	m.boardConfirm = false
	if m.mode != modeBoard {
		m.boardFocus = m.focus
	}
	m.mode = modeBoard
	m.focus = focusViewer
	m.rawBoards = false
}

func (m *Model) leaveBoard() {
	m.mode = modeBrowse
	m.boardPrompt = promptNone
	m.boardInput.Blur()
	m.boardConfirm = false
	m.focus = m.boardFocus
	if m.sidebarHidden {
		m.focus = focusViewer
	}
	m.refreshViewer(false)
}

func (m *Model) clampBoardCursor() {
	m.boardCol = max(0, min(m.boardCol, len(m.board.Columns)-1))
	if len(m.board.Columns) == 0 {
		m.boardRow = 0
		return
	}
	m.boardRow = max(0, min(m.boardRow, len(m.board.Columns[m.boardCol].Cards)-1))
}

func (m Model) currentCard() (kanban.Card, bool) {
	if m.boardCol < 0 || m.boardCol >= len(m.board.Columns) {
		return kanban.Card{}, false
	}
	cards := m.board.Columns[m.boardCol].Cards
	if m.boardRow < 0 || m.boardRow >= len(cards) {
		return kanban.Card{}, false
	}
	return cards[m.boardRow], true
}

func (m *Model) persistBoard() tea.Cmd {
	note := m.notes[m.boardID]
	note.Blocks = m.board.Blocks()
	m.notes[m.boardID] = note
	m.dropRendered(m.boardID)
	return m.saveBoard(m.boardID)
}

func (m *Model) saveBoard(id string) tea.Cmd {
	if m.saving {
		if m.pendingSaves == nil {
			m.pendingSaves = map[string]bool{}
		}
		m.pendingSaves[id] = true
		return nil
	}
	blocks := m.notes[id].Blocks
	client := m.client
	m.saving = true
	return func() tea.Msg {
		note, err := client.SaveNote(id, blocks)
		return boardSavedMsg{id: id, note: note, err: err}
	}
}

func (m *Model) handleBoardMsg(msg tea.Msg) (tea.Cmd, bool) {
	saved, ok := msg.(boardSavedMsg)
	if !ok {
		return nil, false
	}
	m.saving = false
	if saved.err != nil {
		m.flash = ""
		m.err = saved.err
		return nil, true
	}
	m.err = nil
	for id := range m.pendingSaves {
		delete(m.pendingSaves, id)
		return m.saveBoard(id), true
	}
	m.notes[saved.id] = saved.note
	m.dropRendered(saved.id)
	if m.mode == modeBoard && m.boardID == saved.id {
		m.board = kanban.Parse(saved.note.Blocks)
		m.clampBoardCursor()
	} else {
		m.refreshViewer(false)
	}
	if m.flash == "" || m.flash == "Saving…" {
		m.flash = m.icons().Saved + " Saved"
	}
	return nil, true
}

func (m *Model) startBoardPrompt(kind boardPrompt, value, placeholder string) tea.Cmd {
	m.boardPrompt = kind
	m.boardInput.Placeholder = placeholder
	m.boardInput.SetValue(value)
	m.boardInput.CursorEnd()
	return m.boardInput.Focus()
}

func (m *Model) handleBoardKey(msg tea.KeyMsg) tea.Cmd {
	if m.boardPrompt != promptNone {
		return m.handleBoardPromptKey(msg)
	}
	key := msg.String()
	if m.boardConfirm {
		m.boardConfirm = false
		m.flash = ""
		if key == "d" || key == "y" {
			card, _ := m.currentCard()
			m.board.DeleteCard(m.boardCol, m.boardRow)
			m.clampBoardCursor()
			m.flash = "Deleted " + card.Title
			return m.persistBoard()
		}
		return nil
	}
	m.flash = ""
	_, hasCard := m.currentCard()
	switch key {
	case "esc", "q", "ctrl+c":
		m.leaveBoard()
	case "?":
		m.modal = modalHelp
	case "v":
		m.leaveBoard()
		m.toggleBoardView()
	case "h", "left":
		m.boardCol--
		m.clampBoardCursor()
	case "l", "right":
		m.boardCol++
		m.clampBoardCursor()
	case "k", "up":
		m.boardRow--
		m.clampBoardCursor()
	case "j", "down":
		m.boardRow++
		m.clampBoardCursor()
	case "g", "home":
		m.boardRow = 0
	case "G", "end":
		m.boardRow = math.MaxInt
		m.clampBoardCursor()
	case "H", "shift+left", "L", "shift+right":
		if !hasCard {
			return nil
		}
		to := m.boardCol - 1
		if key == "L" || key == "shift+right" {
			to = m.boardCol + 1
		}
		if to < 0 || to >= len(m.board.Columns) {
			return nil
		}
		m.boardCol, m.boardRow = m.board.MoveCard(m.boardCol, m.boardRow, to, m.boardRow)
		return m.persistBoard()
	case "K", "shift+up", "J", "shift+down":
		if !hasCard {
			return nil
		}
		to := m.boardRow - 1
		if key == "J" || key == "shift+down" {
			to = m.boardRow + 1
		}
		if to < 0 || to >= len(m.board.Columns[m.boardCol].Cards) {
			return nil
		}
		m.boardCol, m.boardRow = m.board.MoveCard(m.boardCol, m.boardRow, m.boardCol, to)
		return m.persistBoard()
	case " ", "x":
		if !hasCard {
			return nil
		}
		m.board.ToggleDone(m.boardCol, m.boardRow)
		return m.persistBoard()
	case "a", "n":
		if len(m.board.Columns) == 0 {
			m.flash = "Add a column first (c)"
			return nil
		}
		return m.startBoardPrompt(promptAddCard, "", "Title !high #tag @who due:2026-12-31")
	case "e", "enter":
		card, ok := m.currentCard()
		if !ok {
			return nil
		}
		return m.startBoardPrompt(promptEditCard, card.Text(), "")
	case "d":
		card, ok := m.currentCard()
		if !ok {
			return nil
		}
		m.boardConfirm = true
		m.flash = "Delete \"" + card.Title + "\"? d again to confirm, any other key cancels"
	case "c":
		return m.startBoardPrompt(promptAddColumn, "", "Column name")
	}
	return nil
}

func (m *Model) handleBoardPromptKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.boardPrompt = promptNone
		m.boardInput.Blur()
		return nil
	case "enter":
		kind := m.boardPrompt
		value := strings.TrimSpace(m.boardInput.Value())
		m.boardPrompt = promptNone
		m.boardInput.Blur()
		if value == "" {
			return nil
		}
		switch kind {
		case promptAddCard:
			m.boardRow = m.board.AddCard(m.boardCol, value)
		case promptEditCard:
			m.board.SetCardText(m.boardCol, m.boardRow, value)
		case promptAddColumn:
			at := m.boardCol + 1
			if len(m.board.Columns) == 0 {
				at = 0
			}
			m.boardCol = m.board.InsertColumn(at, value)
			m.boardRow = 0
		}
		return m.persistBoard()
	}
	var cmd tea.Cmd
	m.boardInput, cmd = m.boardInput.Update(msg)
	return cmd
}

func (m Model) boardPromptView() string {
	label := "New card: "
	switch m.boardPrompt {
	case promptEditCard:
		label = "Edit card: "
	case promptAddColumn:
		label = "New column: "
	}
	return accentStyle.Render(label) + m.boardInput.View() + faintStyle.Render("   enter save · esc cancel")
}

func (m Model) boardHints() string {
	pairs := [][2]string{
		{"hjkl", "focus"},
		{"HJKL", "move"},
		{"x", "done"},
		{"a", "add"},
		{"e", "edit"},
		{"d", "delete"},
		{"c", "column"},
		{"?", "keys"},
		{"esc", "back"},
	}
	var hints []string
	for _, p := range pairs {
		hints = append(hints, keyStyle.Render(p[0])+" "+p[1])
	}
	return strings.Join(hints, "  ")
}

func (m Model) boardView() string {
	return m.renderBoard(m.board, m.boardTitle(m.boardID), m.viewer.Width, m.viewer.Height, true)
}

func (m Model) renderBoardPreview(note api.Note, id string, width int) string {
	return m.renderBoard(kanban.Parse(note.Blocks), m.boardTitle(id), width, 0, false)
}

func (m Model) renderBoard(board kanban.Board, title string, width, height int, active bool) string {
	header := accentStyle.Render(m.withIcon(m.icons().Board, title))
	hint := "⏎ open board · v markdown"
	if active {
		hint = "board mode · ? keys · esc back"
	}
	lines := []string{ansi.Truncate(header+faintStyle.Render("   "+hint), width, "…")}
	if len(board.Preamble) > 0 && !active {
		lines = append(lines, faintStyle.Render(ansi.Truncate("Notes above the board are in the markdown view (v)", width, "…")))
	}
	lines = append(lines, "")

	if len(board.Columns) == 0 {
		lines = append(lines, mutedStyle.Render("No columns yet. Add a ## heading, or press ⏎ then c to add one."))
		return strings.Join(lines, "\n")
	}

	n := len(board.Columns)
	shown := max(1, min(n, (width+boardColumnGap)/(boardColumnMin+boardColumnGap)))
	colWidth := max(8, (width-boardColumnGap*(shown-1))/shown)
	start := 0
	if active {
		start = max(0, min(m.boardCol-shown+1, n-shown))
		if m.boardCol < start {
			start = m.boardCol
		}
	}
	if !active {
		for first := 0; first < n; first += shown {
			if first > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, m.renderColumnRow(board, first, min(n, first+shown), colWidth, 0)...)
		}
		return strings.Join(lines, "\n")
	}
	if shown < n {
		more := fmt.Sprintf("columns %d–%d of %d", start+1, start+shown, n)
		lines[len(lines)-1] = faintStyle.Render(ansi.Truncate(more, width, "…"))
	}
	bodyHeight := 0
	if height > 0 {
		bodyHeight = max(1, height-len(lines)-boardHeaderLines)
	}
	lines = append(lines, m.renderColumnRow(board, start, start+shown, colWidth, bodyHeight)...)
	return strings.Join(lines, "\n")
}

func (m Model) renderColumnRow(board kanban.Board, from, to, colWidth, bodyHeight int) []string {
	var rendered [][]string
	tallest := 0
	for i := from; i < to; i++ {
		focused := m.mode == modeBoard && bodyHeight > 0 && i == m.boardCol
		col := m.renderColumn(board, i, colWidth, bodyHeight, focused)
		rendered = append(rendered, col)
		tallest = max(tallest, len(col))
	}
	gap := strings.Repeat(" ", boardColumnGap)
	var lines []string
	for row := 0; row < tallest; row++ {
		var parts []string
		for _, col := range rendered {
			cell := ""
			if row < len(col) {
				cell = col[row]
			}
			parts = append(parts, padRight(cell, colWidth))
		}
		lines = append(lines, strings.TrimRight(strings.Join(parts, gap), " "))
	}
	return lines
}

func (m Model) renderColumn(board kanban.Board, index, width, bodyHeight int, focused bool) []string {
	col := board.Columns[index]
	name := ansi.Truncate(col.Name, max(1, width-4), "…")
	count := fmt.Sprintf(" %d", len(col.Cards))
	headStyle, rule := titleStyle, faintStyle
	if focused {
		headStyle, rule = accentStyle, columnRule
	}
	out := []string{headStyle.Render(name) + faintStyle.Render(count), rule.Render(strings.Repeat("─", width))}

	var body []string
	selStart, selEnd := -1, -1
	if len(col.Cards) == 0 {
		body = append(body, faintStyle.Render("  no cards"))
	}
	today := time.Now().Format("2006-01-02")
	for r, card := range col.Cards {
		selected := focused && r == m.boardRow
		if r > 0 {
			body = append(body, "")
		}
		if selected {
			selStart = len(body)
		}
		body = append(body, m.renderCard(card, width, selected, today)...)
		if selected {
			selEnd = len(body)
		}
	}

	if bodyHeight > 0 && len(body) > bodyHeight {
		offset := 0
		if selEnd > bodyHeight {
			offset = min(selEnd-bodyHeight, selStart)
		}
		offset = max(0, min(offset, len(body)-bodyHeight))
		end := offset + bodyHeight
		visible := append([]string{}, body[offset:end]...)
		if offset > 0 {
			visible[0] = faintStyle.Render(fmt.Sprintf("  ↑ %d more lines", offset))
		}
		if end < len(body) {
			visible[len(visible)-1] = faintStyle.Render(fmt.Sprintf("  ↓ %d more lines", len(body)-end))
		}
		body = visible
	}
	return append(out, body...)
}

func (m Model) renderCard(card kanban.Card, width int, selected bool, today string) []string {
	const prefix = 4
	inner := max(4, width-prefix)
	bar := " "
	if selected {
		bar = selectedBar.Render("▌")
	}
	mark := faintStyle.Render("○")
	cardTitle := lipgloss.NewStyle().Foreground(text)
	if selected {
		cardTitle = cardTitle.Bold(true)
	}
	if card.Done {
		mark = successStyle.Render("✓")
		cardTitle = doneStyle
	}

	title := card.Title
	if title == "" {
		title = "(untitled)"
	}
	var lines []string
	for i, line := range strings.Split(ansi.Wrap(title, inner, ""), "\n") {
		lead := bar + " " + mark + " "
		if i > 0 {
			lead = bar + "   "
		}
		lines = append(lines, lead+cardTitle.Render(line))
	}

	var pieces []string
	if card.Priority != "" {
		style, ok := priorityStyles[card.Priority]
		if !ok {
			style = mutedStyle
		}
		pieces = append(pieces, style.Render("!"+card.Priority))
	}
	for _, tag := range card.Tags {
		pieces = append(pieces, tagStyle.Render("#"+tag))
	}
	if card.Assignee != "" {
		pieces = append(pieces, assigneeStyle.Render("@"+card.Assignee))
	}
	if card.Due != "" {
		style := mutedStyle
		if !card.Done && card.Due < today {
			style = errorStyle
		}
		pieces = append(pieces, style.Render("due "+card.Due))
	}
	if len(card.Body) > 0 {
		pieces = append(pieces, faintStyle.Render(fmt.Sprintf("≡ %d", len(card.Body))))
	}
	current := ""
	for _, piece := range pieces {
		piece = ansi.Truncate(piece, inner, "…")
		if current != "" && lipgloss.Width(current)+1+lipgloss.Width(piece) > inner {
			lines = append(lines, bar+"   "+current)
			current = ""
		}
		if current != "" {
			current += " "
		}
		current += piece
	}
	if current != "" {
		lines = append(lines, bar+"   "+current)
	}
	return lines
}
