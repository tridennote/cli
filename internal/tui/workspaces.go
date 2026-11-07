package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (m *Model) openWorkspaces() {
	if len(m.workspaces) < 2 {
		m.flash = "This is your only workspace"
		return
	}
	m.modal = modalWorkspaces
	m.workspaceCursor = m.workspace
}

func (m *Model) handleWorkspacesKey(msg tea.KeyMsg) tea.Cmd {
	n := len(m.workspaces)
	switch key := msg.String(); key {
	case "esc", "q", "w":
		m.modal = modalNone
	case "up", "k", "ctrl+p":
		m.workspaceCursor = wrapIndex(m.workspaceCursor-1, n)
	case "down", "j", "ctrl+n", "tab":
		m.workspaceCursor = wrapIndex(m.workspaceCursor+1, n)
	case "g", "home":
		m.workspaceCursor = 0
	case "G", "end":
		m.workspaceCursor = n - 1
	case "enter", " ", "l":
		m.modal = modalNone
		return m.switchWorkspace(m.workspaceCursor)
	default:
		if len(key) == 1 && key[0] >= '1' && key[0] <= '9' && int(key[0]-'1') < n {
			m.modal = modalNone
			return m.switchWorkspace(int(key[0] - '1'))
		}
	}
	return nil
}

func (m *Model) switchWorkspace(i int) tea.Cmd {
	if i == m.workspace || i < 0 || i >= len(m.workspaces) {
		return nil
	}
	m.workspace = i
	m.roots, m.visible, m.externals = nil, nil, nil
	m.cursor, m.offset = 0, 0
	m.openID = ""
	m.loadingWorkspace = m.workspaces[i].ID
	m.err = nil
	m.refreshViewer(true)
	return m.loadTree()
}

func (m Model) workspacesView() string {
	width := max(36, min(56, m.width-6))
	inner := width - 4
	lines := []string{accentStyle.Render("Workspaces"), ""}
	for i, ws := range m.workspaces {
		pointer, label := "  ", mutedStyle.Render(ws.Name)
		if i == m.workspaceCursor {
			pointer, label = accentStyle.Render("› "), titleStyle.Render(ws.Name)
		}
		left := pointer + label
		hint := ""
		if i == m.workspace {
			hint = faintStyle.Render("current")
		} else if i < 9 {
			hint = keyStyle.Render(fmt.Sprint(i + 1))
		}
		gap := max(1, inner-lipgloss.Width(left)-lipgloss.Width(hint))
		lines = append(lines, ansi.Truncate(left+strings.Repeat(" ", gap)+hint, inner, "…"))
	}
	lines = append(lines, "", faintStyle.Render("↑↓ choose · ⏎ switch · esc close"))
	return modalStyle.Width(width).Render(strings.Join(lines, "\n"))
}
