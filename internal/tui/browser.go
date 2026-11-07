package tui

import (
	"os/exec"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tridennote/cli/internal/tree"
)

var openBrowser = func(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

func (m Model) webURL(id string) string {
	return strings.TrimRight(m.client.BaseURL, "/") + "/note/" + id
}

func (m *Model) openInBrowser(node *tree.Node) tea.Cmd {
	if err := openBrowser(m.webURL(node.ID)); err != nil {
		m.flash = "Could not open the browser: " + err.Error()
		return nil
	}
	m.flash = "Opened " + node.Label() + " in the browser"
	return nil
}

// openTargetInBrowser opens the selected file, or the open note, in the web app.
func (m *Model) openTargetInBrowser() tea.Cmd {
	if node := m.tableNode(); node != nil {
		return m.openInBrowser(node)
	}
	if node := m.placeholderNode(); node != nil {
		return m.openInBrowser(node)
	}
	if node := m.selected(); m.focus == focusTree && !m.sidebarHidden && node != nil && !node.IsFolder && !node.Divider {
		return m.openInBrowser(node)
	}
	if node := m.findNode(m.openID); node != nil {
		return m.openInBrowser(node)
	}
	m.flash = "Select a file first"
	return nil
}

// externalSelected reports, with a hint, when a note-only key is pressed on a file the terminal can't show.
func (m *Model) externalSelected() bool {
	if m.tableNode() != nil {
		m.flash = m.selected().Label() + " is read-only here — press o to open it in the browser"
		return true
	}
	if !m.browsingExternal() {
		return false
	}
	m.flash = m.selected().Label() + " only opens in the browser — press ⏎ or o"
	return true
}
