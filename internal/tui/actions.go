package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/tridennote/cli/internal/kanban"
	"github.com/tridennote/cli/internal/tree"
)

type action struct {
	id    string
	label string
	key   string
}

func newActionInput() textinput.Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.Placeholder = "Type to filter actions"
	ti.CharLimit = 80
	ti.TextStyle = lipgloss.NewStyle().Foreground(text)
	return ti
}

func (m Model) externalEditorName() string {
	if _, name := externalEditorCommand(m.settings.Editor); name != "" {
		return name
	}
	return "code editor"
}

func (m Model) availableActions() []action {
	var out []action
	if m.mode == modeBoard {
		out = append(out,
			action{"board-add", "New card", "a"},
			action{"board-open", "Open card description", "o"},
			action{"board-external", "Edit card description in " + m.externalEditorName(), "E"},
			action{"board-column", "Add column", "c"},
			action{"board-markdown", "Show board as markdown", "v"},
			action{"board-leave", "Leave board", "esc"},
		)
	} else if node := m.tableNode(); node != nil {
		out = append(out, action{"browser", "Open " + node.Label() + " in browser", "o"})
	} else if m.browsingExternal() {
		out = append(out, action{"browser", "Open " + m.selected().Label() + " in browser", "o"})
	} else if target := m.targetNote(); target != "" {
		if m.isBoard(target) {
			out = append(out, action{"open-board", "Open board", "⏎"})
		} else {
			out = append(out,
				action{"add-to-board", "Add to kanban board…", ""},
				action{"edit-builtin", "Edit in built-in editor", "i"},
				action{"edit-external", "Edit in " + m.externalEditorName(), "e"},
				action{"copy", "Copy as markdown", "y"},
				action{"diagrams", "Open diagrams…", "m"},
				action{"browser", "Open in browser", "o"},
			)
		}
	}
	out = append(out,
		action{"new-note", "New note", "n"},
		action{"new-board", "New kanban board", "N"},
		action{"sidebar", "Toggle sidebar", "b"},
	)
	if len(m.workspaces) > 1 {
		out = append(out, action{"workspace", "Switch workspace…", "w"})
	}
	if m.updateInfo != nil {
		out = append(out, action{"update", "Copy update command (" + m.updateInfo.Latest + ")", "u"})
	}
	return append(out,
		action{"settings", "Settings", ","},
		action{"help", "Keyboard shortcuts", "?"},
	)
}

func (m Model) filteredActions() []action {
	query := strings.ToLower(strings.TrimSpace(m.actionInput.Value()))
	all := m.availableActions()
	if query == "" {
		return all
	}
	var out []action
	for _, a := range all {
		if strings.Contains(strings.ToLower(a.label), query) {
			out = append(out, a)
		}
	}
	return out
}

func (m *Model) openActions() tea.Cmd {
	m.modal = modalActions
	m.actionCursor = 0
	m.actionInput.SetValue("")
	return m.actionInput.Focus()
}

func (m *Model) closeActions() {
	m.modal = modalNone
	m.actionInput.Blur()
}

func (m *Model) handleActionsKey(msg tea.KeyMsg) tea.Cmd {
	actions := m.filteredActions()
	switch msg.String() {
	case "esc", "ctrl+c", "ctrl+k":
		m.closeActions()
		return nil
	case "up", "ctrl+p":
		if len(actions) > 0 {
			m.actionCursor = (m.actionCursor - 1 + len(actions)) % len(actions)
		}
		return nil
	case "down", "ctrl+n", "tab":
		if len(actions) > 0 {
			m.actionCursor = (m.actionCursor + 1) % len(actions)
		}
		return nil
	case "enter":
		if len(actions) == 0 {
			return nil
		}
		chosen := actions[min(m.actionCursor, len(actions)-1)]
		m.closeActions()
		return m.runAction(chosen)
	}
	before := m.actionInput.Value()
	var cmd tea.Cmd
	m.actionInput, cmd = m.actionInput.Update(msg)
	if m.actionInput.Value() != before {
		m.actionCursor = 0
	}
	return cmd
}

func keyFor(k string) tea.KeyMsg {
	switch k {
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "⏎":
		return tea.KeyMsg{Type: tea.KeyEnter}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

func (m *Model) runAction(a action) tea.Cmd {
	switch a.id {
	case "add-to-board":
		return m.openAddToBoard()
	case "open-board":
		return m.requestBoard(m.targetNote())
	case "edit-builtin":
		return m.requestEdit(m.targetNote(), false)
	case "edit-external":
		return m.requestEdit(m.targetNote(), true)
	}
	if m.mode == modeBoard {
		return m.handleBoardKey(keyFor(a.key))
	}
	_, cmd := m.handleBrowseKey(a.key, keyFor(a.key))
	return cmd
}

func (m Model) actionsView() string {
	width := max(36, min(56, m.width-6))
	inner := width - 4
	lines := []string{
		accentStyle.Render("Actions"),
		"",
		accentStyle.Render("› ") + m.actionInput.View(),
		"",
	}
	actions := m.filteredActions()
	if len(actions) == 0 {
		lines = append(lines, faintStyle.Render("  No matching actions"))
	}
	for i, a := range actions {
		pointer, label := "  ", mutedStyle.Render(a.label)
		if i == m.actionCursor {
			pointer, label = accentStyle.Render("› "), titleStyle.Render(a.label)
		}
		left := pointer + label
		hint := ""
		if a.key != "" {
			hint = keyStyle.Render(a.key)
		}
		gap := max(1, inner-lipgloss.Width(left)-lipgloss.Width(hint))
		lines = append(lines, ansi.Truncate(left+strings.Repeat(" ", gap)+hint, inner, "…"))
	}
	lines = append(lines, "", faintStyle.Render("↑↓ choose · ⏎ run · esc close"))
	return modalStyle.Width(width).Render(strings.Join(lines, "\n"))
}

func (m Model) boardNodes() []*tree.Node {
	var out []*tree.Node
	var walk func([]*tree.Node)
	walk = func(nodes []*tree.Node) {
		for _, n := range nodes {
			if n.IsFolder {
				walk(n.Children)
			} else if n.IsBoard {
				out = append(out, n)
			}
		}
	}
	walk(m.roots)
	return out
}

func (m *Model) ensureLoaded(id string) tea.Cmd {
	if _, ok := m.notes[id]; ok {
		return nil
	}
	return m.loadNote(id)
}

func (m *Model) openAddToBoard() tea.Cmd {
	noteID := m.targetNote()
	if noteID == "" || m.isBoard(noteID) {
		m.flash = "Open a note first, then add it to a board"
		return nil
	}
	boards := m.boardNodes()
	if len(boards) == 0 {
		m.flash = "No boards in this workspace yet — N creates one"
		return nil
	}
	m.modal = modalAddToBoard
	m.addNoteID = noteID
	m.addBoards = boards
	m.addBoard, m.addCol, m.addStep = 0, 0, 0
	if len(boards) == 1 {
		m.addStep = 1
	}
	cmds := make([]tea.Cmd, 0, len(boards))
	for _, b := range boards {
		cmds = append(cmds, m.ensureLoaded(b.ID))
	}
	return tea.Batch(cmds...)
}

func (m Model) addColumns() ([]kanban.Column, bool) {
	if m.addBoard >= len(m.addBoards) {
		return nil, false
	}
	note, ok := m.notes[m.addBoards[m.addBoard].ID]
	if !ok {
		return nil, false
	}
	return kanban.Parse(note.Blocks).Columns, true
}

func (m Model) alreadyOnBoard(boardID string) bool {
	note, ok := m.notes[boardID]
	return ok && len(kanban.Parse(note.Blocks).LinksTo(m.addNoteID)) > 0
}

func (m *Model) handleAddToBoardKey(msg tea.KeyMsg) tea.Cmd {
	key := msg.String()
	if m.addStep == 0 {
		switch key {
		case "esc", "q", "ctrl+c":
			m.modal = modalNone
		case "up", "k", "shift+tab":
			m.addBoard = (m.addBoard - 1 + len(m.addBoards)) % len(m.addBoards)
		case "down", "j", "tab":
			m.addBoard = (m.addBoard + 1) % len(m.addBoards)
		case "enter", "right", "l":
			m.addStep, m.addCol = 1, 0
		}
		return nil
	}
	columns, loaded := m.addColumns()
	switch key {
	case "esc", "q", "ctrl+c":
		if len(m.addBoards) > 1 {
			m.addStep = 0
		} else {
			m.modal = modalNone
		}
	case "left", "h", "shift+tab":
		if len(columns) > 0 {
			m.addCol = (m.addCol - 1 + len(columns)) % len(columns)
		}
	case "right", "l", "tab":
		if len(columns) > 0 {
			m.addCol = (m.addCol + 1) % len(columns)
		}
	case "enter":
		if !loaded || len(columns) == 0 {
			return nil
		}
		return m.linkNoteToBoard()
	}
	return nil
}

func (m *Model) linkNoteToBoard() tea.Cmd {
	boardNode := m.addBoards[m.addBoard]
	board := m.boardFor(boardNode.ID)
	if m.addCol >= len(board.Columns) {
		return nil
	}
	row := board.LinkNote(m.addCol, kanban.Meta{Title: m.nodeName(m.addNoteID)}, m.addNoteID)
	number := board.Columns[m.addCol].Cards[row].Number
	m.applyBoard(boardNode.ID, board)
	m.modal = modalNone
	m.flash = fmt.Sprintf("%s Added #%d to %s · %s", m.icons().Board, number, kanban.DisplayName(boardNode.Name), board.Columns[m.addCol].Name)
	return m.saveBoard(boardNode.ID)
}

func (m Model) addToBoardView() string {
	width := max(40, min(64, m.width-6))
	inner := width - 4
	title := accentStyle.Render(m.withIcon(m.icons().Board, "Add to kanban board"))
	lines := []string{title, faintStyle.Render(ansi.Truncate("Links "+m.nodeName(m.addNoteID)+".md as a card", inner, "…")), ""}

	if len(m.addBoards) > 1 {
		heading := faintStyle.Render("BOARD")
		if m.addStep == 0 {
			heading = accentStyle.Render("BOARD")
		}
		lines = append(lines, heading)
		for i, b := range m.addBoards {
			pointer, label := "  ", mutedStyle.Render(kanban.DisplayName(b.Name))
			if i == m.addBoard {
				pointer = faintStyle.Render("› ")
				label = titleStyle.Render(kanban.DisplayName(b.Name))
				if m.addStep == 0 {
					pointer = accentStyle.Render("› ")
				}
			}
			row := pointer + label
			if m.alreadyOnBoard(b.ID) {
				row += faintStyle.Render("  already added")
			}
			lines = append(lines, ansi.Truncate(row, inner, "…"))
		}
		lines = append(lines, "")
	} else {
		lines = append(lines, mutedStyle.Render(m.withIcon(m.icons().Board, kanban.DisplayName(m.addBoards[0].Name))), "")
	}

	heading := faintStyle.Render("COLUMN")
	if m.addStep == 1 {
		heading = accentStyle.Render("COLUMN")
	}
	lines = append(lines, heading)
	columns, loaded := m.addColumns()
	switch {
	case !loaded:
		lines = append(lines, faintStyle.Render("  Loading columns…"))
	case len(columns) == 0:
		lines = append(lines, faintStyle.Render("  This board has no columns yet"))
	default:
		var chips []string
		for i, col := range columns {
			label := fmt.Sprintf(" %s %d ", col.Name, len(col.Cards))
			switch {
			case i == m.addCol && m.addStep == 1:
				chips = append(chips, cursorStyle.Render(label))
			case i == m.addCol:
				chips = append(chips, titleStyle.Render("["+strings.TrimSpace(label)+"]"))
			default:
				chips = append(chips, mutedStyle.Render(label))
			}
		}
		lines = append(lines, ansi.Wrap(strings.Join(chips, " "), inner, ""))
	}

	hint := "↑↓ board · ⏎ choose column · esc cancel"
	if m.addStep == 1 {
		hint = "←→ column · ⏎ add card · esc back"
		if len(m.addBoards) == 1 {
			hint = "←→ column · ⏎ add card · esc cancel"
		}
	}
	lines = append(lines, "", faintStyle.Render(hint))
	return modalStyle.Width(width).Render(strings.Join(lines, "\n"))
}
