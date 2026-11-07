package tui

import (
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/tridennote/cli/internal/api"
	"github.com/tridennote/cli/internal/kanban"
	"github.com/tridennote/cli/internal/markdown"
)

type cardNoteCreatedMsg struct {
	boardID string
	cardID  string
	col     int
	meta    kanban.Meta
	node    api.TreeNode
	note    api.Note
	open    bool
	err     error
}

type cardBodyDoneMsg struct {
	boardID  string
	cardID   string
	path     string
	baseline string
	err      error
}

func (m Model) boardFor(id string) kanban.Board {
	return kanban.Parse(m.notes[id].Blocks)
}

func (m *Model) applyBoard(id string, board kanban.Board) {
	note := m.notes[id]
	note.Blocks = board.Blocks()
	m.notes[id] = note
	m.dropRendered(id)
	if m.mode == modeBoard && m.boardID == id {
		m.board = kanban.Parse(note.Blocks)
		m.clampBoardCursor()
	}
}

func (m Model) parentIDOf(id string) *string {
	if node := m.findNode(id); node != nil && node.Parent != nil {
		parent := node.Parent.ID
		return &parent
	}
	return nil
}

func (m *Model) composeInFullEditor() tea.Cmd {
	value := strings.TrimSpace(m.boardInput.Value())
	kind := m.boardPrompt
	m.boardPrompt = promptNone
	m.boardInput.Blur()
	if kind == promptEditCard {
		var save tea.Cmd
		if value != "" {
			m.board.SetCardText(m.boardCol, m.boardRow, value)
			save = m.persistBoard()
		}
		card, ok := m.currentCard()
		if !ok {
			return save
		}
		return tea.Batch(save, m.openCardDescription(card, false))
	}
	if len(m.board.Columns) == 0 {
		return nil
	}
	return m.startCardDraft(value)
}

func (m *Model) createCardNote(boardID, cardID string, col int, meta kanban.Meta, blocks []api.Block) tea.Cmd {
	return m.createCardNoteThen(boardID, cardID, col, meta, blocks, true)
}

func (m *Model) createCardNoteThen(boardID, cardID string, col int, meta kanban.Meta, blocks []api.Block, open bool) tea.Cmd {
	if len(m.workspaces) == 0 {
		return nil
	}
	name := strings.TrimSpace(meta.Title)
	if name == "" {
		name = "Untitled"
	}
	meta.Title = name
	workspaceID := m.workspaces[m.workspace].ID
	parent := m.parentIDOf(boardID)
	client := m.client
	m.flash = "Creating " + name + ".md…"
	return func() tea.Msg {
		node, err := client.CreateNote(workspaceID, parent, name)
		note := api.Note{Title: node.Name}
		if err == nil && len(blocks) > 0 {
			note, err = client.SaveNote(node.ID, blocks)
		}
		return cardNoteCreatedMsg{boardID: boardID, cardID: cardID, col: col, meta: meta, node: node, note: note, open: open, err: err}
	}
}

func (m *Model) openCardDescription(card kanban.Card, external bool) tea.Cmd {
	if card.NoteID != "" && m.findNode(card.NoteID) != nil {
		if !external {
			m.rememberBoard()
		}
		return m.requestEdit(card.NoteID, external)
	}
	col, _, ok := m.board.Locate(card.ID)
	if !ok {
		return nil
	}
	if external {
		return m.launchCardBodyEditor(m.boardID, card)
	}
	return m.createCardNote(m.boardID, card.ID, col, card.Meta, kanban.BodyToNote(card.Body))
}

func (m *Model) rememberBoard() {
	if m.mode != modeBoard {
		return
	}
	m.returnBoard = m.boardID
	m.returnCol, m.returnRow = m.boardCol, m.boardRow
}

func (m *Model) restoreBoard() {
	id := m.returnBoard
	m.returnBoard = ""
	if id == "" {
		return
	}
	if _, ok := m.notes[id]; !ok {
		return
	}
	m.enterBoard(id)
	m.boardCol, m.boardRow = m.returnCol, m.returnRow
	m.clampBoardCursor()
}

func (m *Model) launchCardBodyEditor(boardID string, card kanban.Card) tea.Cmd {
	command, editorName := externalEditorCommand(m.settings.Editor)
	if command == nil {
		m.flash = "No code editor found — pick one in settings (,)"
		return nil
	}
	content := markdown.FromBlocks(kanban.BodyToNote(card.Body))
	name := strings.TrimSpace(unsafeFileChars.ReplaceAllString(card.Title, ""))
	if name == "" {
		name = "card"
	}
	file, err := os.CreateTemp("", name+"-*.md")
	if err != nil {
		m.flash = err.Error()
		return nil
	}
	_, writeErr := file.WriteString(content)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(file.Name())
		m.flash = "Could not prepare a temp file for the editor"
		return nil
	}
	path := file.Name()
	cmd := exec.Command(command[0], append(command[1:], path)...)
	m.flash = m.icons().Edit + " Editing the card description in " + editorName + " — close the file to come back"
	cardID := card.ID
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return cardBodyDoneMsg{boardID: boardID, cardID: cardID, path: path, baseline: content, err: err}
	})
}

func (m *Model) handleCardMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case cardNoteCreatedMsg:
		if msg.err != nil {
			m.flash = ""
			m.err = msg.err
			return nil, true
		}
		m.err = nil
		board := m.boardFor(msg.boardID)
		col, row := msg.col, -1
		if msg.cardID == "" {
			row = board.LinkNote(msg.col, msg.meta, msg.node.ID)
		} else if c, r, ok := board.Locate(msg.cardID); ok {
			board.SetCardNote(c, r, msg.node.ID)
			col, row = c, r
		}
		if row < 0 {
			return nil, true
		}
		m.applyBoard(msg.boardID, board)
		if m.mode == modeBoard && m.boardID == msg.boardID {
			m.boardCol, m.boardRow = col, row
		}
		msg.note.Title = msg.node.Name
		m.notes[msg.node.ID] = msg.note
		save := m.saveBoard(msg.boardID)
		if !msg.open {
			number := board.Columns[col].Cards[row].Number
			m.flash = m.icons().Saved + " Created #" + strconv.Itoa(number) + " " + msg.meta.Title + " with " + msg.node.Name + ".md"
			return tea.Batch(m.loadTree(), save), true
		}
		m.rememberBoard()
		m.beginEditor(msg.node.ID)
		m.flash = "Linked " + msg.node.Name + ".md to the card — ctrl+s save · esc back to the board"
		return tea.Batch(m.loadTree(), save, textarea.Blink), true

	case cardBodyDoneMsg:
		data, readErr := os.ReadFile(msg.path)
		_ = os.Remove(msg.path)
		if msg.err != nil {
			m.flash = "Editor exited with an error: " + msg.err.Error()
			return nil, true
		}
		if readErr != nil {
			m.flash = "Could not read the edited file"
			return nil, true
		}
		content := string(data)
		if content == msg.baseline {
			m.flash = "No changes"
			return nil, true
		}
		board := m.boardFor(msg.boardID)
		c, r, ok := board.Locate(msg.cardID)
		if !ok {
			m.flash = "That card no longer exists"
			return nil, true
		}
		board.SetCardBody(c, r, kanban.NoteToBody(markdown.ToBlocks(content)))
		m.applyBoard(msg.boardID, board)
		m.flash = "Saving…"
		return m.saveBoard(msg.boardID), true
	}
	return nil, false
}
