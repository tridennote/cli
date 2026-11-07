package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/tridennote/cli/internal/kanban"
	"github.com/tridennote/cli/internal/markdown"
)

func (m *Model) startCardDraft(title string) tea.Cmd {
	m.boardPrompt = promptNone
	m.draftBoard = m.boardID
	m.draftCol = m.boardCol
	m.draftTitleFocus = true
	m.confirmDiscard = false
	m.mode = modeCardDraft
	m.editor.SetValue("")
	m.editor.Blur()
	m.boardInput.SetValue(title)
	m.boardInput.CursorEnd()
	m.suggestIndex, m.suggestHidden = 0, false
	return m.boardInput.Focus()
}

func (m *Model) focusDraftBody() tea.Cmd {
	m.draftTitleFocus = false
	m.boardInput.Blur()
	m.editor.Focus()
	return textarea.Blink
}

func (m *Model) focusDraftTitle() tea.Cmd {
	m.draftTitleFocus = true
	m.editor.Blur()
	m.suggestHidden = true
	return m.boardInput.Focus()
}

func (m Model) draftDirty() bool {
	return strings.TrimSpace(m.boardInput.Value()) != "" || strings.TrimSpace(m.editor.Value()) != ""
}

func (m *Model) closeDraft() {
	m.mode = modeBoard
	m.draftTitleFocus = false
	m.confirmDiscard = false
	m.boardInput.Blur()
	m.editor.Blur()
	m.editor.SetValue("")
}

func (m *Model) saveDraft() tea.Cmd {
	meta := kanban.ParseMeta(strings.TrimSpace(m.boardInput.Value()))
	if meta.Title == "" {
		m.flash = "Give the card a title first"
		return m.focusDraftTitle()
	}
	body := markdown.ToBlocks(m.editor.Value())
	boardID, col := m.draftBoard, m.draftCol
	m.closeDraft()
	return m.createCardNoteThen(boardID, "", col, meta, body, false)
}

func (m *Model) handleDraftKey(msg tea.KeyMsg) tea.Cmd {
	key := msg.String()
	if key == "ctrl+s" {
		return m.saveDraft()
	}
	if key == "esc" || key == "ctrl+c" {
		if m.draftTitleFocus && len(m.visibleSuggestions()) > 0 {
			m.suggestHidden = true
			return nil
		}
		if m.draftDirty() && !m.confirmDiscard {
			m.confirmDiscard = true
			m.flash = "Discard this card? ctrl+s to create it, esc again to discard"
			return nil
		}
		m.flash = ""
		m.closeDraft()
		return nil
	}
	m.confirmDiscard = false
	if m.draftTitleFocus {
		if suggestions := m.visibleSuggestions(); len(suggestions) > 0 {
			if cmd, handled := m.handleComposerKey(msg); handled {
				return cmd
			}
		}
		switch key {
		case "enter", "tab", "down":
			return m.focusDraftBody()
		}
		before := m.boardInput.Value()
		var cmd tea.Cmd
		m.boardInput, cmd = m.boardInput.Update(msg)
		if m.boardInput.Value() != before {
			m.suggestHidden = false
			m.suggestIndex = 0
		}
		return cmd
	}
	if key == "shift+tab" {
		return m.focusDraftTitle()
	}
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(msg)
	return cmd
}

func (m Model) draftView() string {
	width := m.viewer.Width
	column := ""
	board := m.boardFor(m.draftBoard)
	if m.draftCol >= 0 && m.draftCol < len(board.Columns) {
		column = board.Columns[m.draftCol].Name
	}
	header := accentStyle.Render(m.withIcon(m.icons().Board, "New card")) +
		faintStyle.Render(" · "+m.boardTitle(m.draftBoard)+" · "+column)
	titleLabel, bodyLabel := accentStyle.Render("TITLE"), faintStyle.Render("DESCRIPTION")
	if !m.draftTitleFocus {
		titleLabel, bodyLabel = faintStyle.Render("TITLE"), accentStyle.Render("DESCRIPTION")
	}
	pointer := "  "
	if m.draftTitleFocus {
		pointer = accentStyle.Render("› ")
	}
	lines := []string{ansi.Truncate(header, width, "…"), "", titleLabel, pointer + m.highlightedInput(m.boardInput, width-2)}

	if m.draftTitleFocus {
		lines = append(lines, m.suggestionRows(m.visibleSuggestions(), "  ", width)...)
		lines = append(lines, "  "+m.legend())
	}
	lines = append(lines, "", bodyLabel)

	editor := m.editor
	editor.SetWidth(width)
	editor.SetHeight(max(3, m.viewer.Height-len(lines)))
	if m.draftTitleFocus && strings.TrimSpace(editor.Value()) == "" {
		lines = append(lines, faintStyle.Render("  Press ⏎ to write the description in markdown"))
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines, "\n") + "\n" + editor.View()
}

func (m Model) draftHints() string {
	if m.draftTitleFocus {
		return keyStyle.Render("⏎ tab") + " description  " + keyStyle.Render("ctrl+s") + " create card  " + keyStyle.Render("esc") + " cancel"
	}
	return keyStyle.Render("shift+tab") + " title  " + keyStyle.Render("ctrl+s") + " create card  " + keyStyle.Render("esc") + " cancel"
}
