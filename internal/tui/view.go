package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/tridennote/cli/internal/tree"
)

const (
	minTreeWidth = 26
	maxTreeWidth = 42
	treeHeader   = 2
)

var renderers = map[int]*glamour.TermRenderer{}

func rendererFor(width int) *glamour.TermRenderer {
	if r, ok := renderers[width]; ok {
		return r
	}
	r, err := glamour.NewTermRenderer(glamour.WithStandardStyle("dark"), glamour.WithWordWrap(width))
	if err != nil {
		return nil
	}
	renderers[width] = r
	return r
}

func (m Model) treeWidth() int {
	if m.sidebarHidden {
		return 0
	}
	if m.width < 60 {
		return max(18, m.width/2)
	}
	return max(minTreeWidth, min(maxTreeWidth, m.width/3))
}

func (m Model) paneHeight() int {
	return max(3, m.height-1)
}

func (m Model) treeRows() int {
	return max(1, m.paneHeight()-2-treeHeader)
}

func (m *Model) layout() {
	viewerWidth := m.width - m.treeWidth() - 2
	m.viewer.Width = max(10, viewerWidth-2)
	m.viewer.Height = max(1, m.paneHeight()-2)
	m.editor.SetWidth(m.viewer.Width)
	m.editor.SetHeight(max(1, m.viewer.Height-1))
	m.ensureCursorVisible()
	m.refreshViewer(false)
}

func (m *Model) ensureCursorVisible() {
	rows := m.treeRows()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+rows {
		m.offset = m.cursor - rows + 1
	}
	m.offset = max(0, min(m.offset, max(0, len(m.visible)-rows)))
}

func (m *Model) refreshViewer(resetScroll bool) {
	if m.viewer.Width <= 0 {
		return
	}
	var content string
	placeholder := m.placeholderNode()
	if id := placeholderID(placeholder); id != m.viewerPlaceholder {
		m.viewerPlaceholder = id
		resetScroll = true
	}
	switch note, ok := m.notes[m.openID]; {
	case placeholder != nil:
		content = m.externalViewer(placeholder)
	case m.openID == "":
		content = m.emptyViewer()
	case !ok:
		content = ""
	default:
		board := m.showsBoard(m.openID)
		key := fmt.Sprintf("%s:%d:%t", m.openID, m.viewer.Width, board)
		rendered, cached := m.rendered[key]
		if !cached {
			if board {
				rendered = m.renderBoardPreview(note, m.openID, m.viewer.Width)
			} else {
				rendered = m.renderNote(note, m.viewer.Width)
			}
			m.rendered[key] = rendered
		}
		content = rendered
	}
	m.viewer.SetContent(content)
	if resetScroll {
		m.viewer.GotoTop()
	}
}

// placeholderNode is the browser-only file under the tree cursor, which the viewer stands in for.
func (m Model) placeholderNode() *tree.Node {
	node := m.selected()
	if m.mode != modeBrowse || m.sidebarHidden || node == nil || !node.External {
		return nil
	}
	return node
}

func placeholderID(node *tree.Node) string {
	if node == nil {
		return ""
	}
	return node.ID
}

func fileKind(name string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	switch ext {
	case "excalidraw":
		return "Excalidraw drawing"
	case "pdf":
		return "PDF"
	case "png", "jpg", "jpeg", "gif", "webp", "svg", "avif", "heic":
		return "Image"
	case "":
		return "File"
	}
	return strings.ToUpper(ext) + " file"
}

func (m Model) externalViewer(node *tree.Node) string {
	name := titleStyle.Render(node.Label())
	if node.Dir != "" {
		name = faintStyle.Render(node.Dir+"/") + name
	}
	block := lipgloss.JoinVertical(lipgloss.Center,
		accentStyle.Bold(true).Render(m.icons().Logo),
		"",
		m.wordmark(),
		"",
		"",
		name,
		mutedStyle.Render(fileKind(node.Name)+" · can't be shown in the terminal"),
		"",
		keyStyle.Render("⏎")+mutedStyle.Render(" open in browser"),
	)
	return lipgloss.Place(m.viewer.Width, m.viewer.Height, lipgloss.Center, lipgloss.Center, block)
}

func (m Model) emptyViewer() string {
	side := m.settings.SidebarSide
	lines := []string{
		"",
		accentStyle.Render("  " + m.withIcon(m.icons().Logo, "tridennote")),
		"",
		mutedStyle.Render("  Pick a note from the tree on the " + side + "."),
		"",
		"  " + keyStyle.Render("↑ ↓") + mutedStyle.Render("  move"),
		"  " + keyStyle.Render("⏎  ") + mutedStyle.Render("  open note"),
		"  " + keyStyle.Render("e  ") + mutedStyle.Render("  edit · ") + keyStyle.Render("n") + mutedStyle.Render(" new · ") + keyStyle.Render("y") + mutedStyle.Render(" copy"),
		"  " + keyStyle.Render("N  ") + mutedStyle.Render("  new kanban board · ") + keyStyle.Render("v") + mutedStyle.Render(" board ⇄ markdown"),
		"  " + keyStyle.Render(",  ") + mutedStyle.Render("  settings · ") + keyStyle.Render("?") + mutedStyle.Render(" all shortcuts"),
	}
	if m.user != nil {
		lines = append(lines, "", faintStyle.Render("  Signed in as "+m.user.Email))
	}
	return strings.Join(lines, "\n")
}

func (m Model) View() string {
	if m.width == 0 {
		return ""
	}
	switch m.state {
	case stateLogin:
		return m.loginView()
	case stateBrowse:
		base := m.browseView()
		switch m.modal {
		case modalSettings:
			return overlay(base, m.settingsView(), m.width, m.height)
		case modalHelp:
			return overlay(base, m.helpView(), m.width, m.height)
		case modalActions:
			return overlay(base, m.actionsView(), m.width, m.height)
		case modalAddToBoard:
			return overlay(base, m.addToBoardView(), m.width, m.height)
		case modalDiagrams:
			return overlay(base, m.diagramsView(), m.width, m.height)
		case modalWorkspaces:
			return overlay(base, m.workspacesView(), m.width, m.height)
		}
		if m.mode == modeBoard && m.composing() {
			return overlay(base, m.composerView(), m.width, m.height)
		}
		return base
	default:
		return m.centered(m.loadingView())
	}
}

func (m Model) centered(body string) string {
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, body)
}

func (m Model) loadingView() string {
	if m.err != nil {
		return lipgloss.JoinVertical(lipgloss.Center,
			errorStyle.Render("✕ "+m.err.Error()),
			"",
			mutedStyle.Render("r retry · q quit"),
		)
	}
	if m.state == stateLoading {
		return m.loader(loaderVault)
	}
	return m.loader(loaderBoot)
}

func (m Model) loginView() string {
	logo := m.wordmark()
	parts := []string{logo, "", titleStyle.Render("You're not signed in"), ""}

	switch {
	case m.device == nil && m.err == nil:
		parts = append(parts, m.miniLoader(loaderCode))
	case m.device != nil:
		parts = append(parts,
			mutedStyle.Render("Press ")+keyStyle.Render("enter")+mutedStyle.Render(" to sign in with your browser"),
			"",
			codeStyle.Render(m.device.UserCode),
			"",
			mutedStyle.Render("Make sure the browser shows this same code."),
		)
		status := mutedStyle.Render("Waiting for you to press enter")
		if m.browserOpened {
			status = m.miniLoader(loaderAwaitAuth)
		}
		parts = append(parts, "", status)
	}

	if m.err != nil {
		parts = append(parts, "", errorStyle.Render("✕ "+m.err.Error()), mutedStyle.Render("r retry"))
	}

	card := cardStyle.Render(lipgloss.JoinVertical(lipgloss.Center, parts...))
	footer := []string{card}
	if m.device != nil {
		url := ansi.Truncate(m.device.VerificationURL, max(20, m.width-4), "…")
		footer = append(footer, "", faintStyle.Render("Browser didn't open? Visit "), faintStyle.Render(url))
	}
	if m.updateInfo != nil {
		footer = append(footer, "", m.updateBadge())
	}
	footer = append(footer, "", faintStyle.Render("q quit"))
	return m.centered(lipgloss.JoinVertical(lipgloss.Center, footer...))
}

func (m Model) browseView() string {
	paneHeight := m.paneHeight()
	treeWidth := m.treeWidth()

	viewerStyle, treeStyle := paneUnfocused, paneFocused
	if m.focus == focusViewer {
		viewerStyle, treeStyle = paneFocused, paneUnfocused
	}

	viewerPane := viewerStyle.
		Width(m.width - treeWidth - 2).
		Height(paneHeight - 2).
		Render(lipgloss.NewStyle().Padding(0, 1).Render(m.viewerContent()))

	var body string
	if m.sidebarHidden {
		body = viewerPane
	} else {
		treePane := treeStyle.
			Width(treeWidth - 2).
			Height(paneHeight - 2).
			Render(m.treeView(treeWidth - 2))
		if m.settings.SidebarSide == "left" {
			body = lipgloss.JoinHorizontal(lipgloss.Top, treePane, viewerPane)
		} else {
			body = lipgloss.JoinHorizontal(lipgloss.Top, viewerPane, treePane)
		}
	}
	return lipgloss.JoinVertical(lipgloss.Left, body, m.statusBar())
}

func (m Model) viewerContent() string {
	if m.mode == modeEdit {
		return m.editorView()
	}
	if m.mode == modeBoard {
		return m.boardView()
	}
	if m.mode == modeCardDraft {
		return m.draftView()
	}
	if m.loadingWorkspace != "" {
		return lipgloss.Place(m.viewer.Width, m.viewer.Height, lipgloss.Center, lipgloss.Center, m.loader(loaderWorkspace))
	}
	if node := m.tableNode(); node != nil {
		return m.csvView(node)
	}
	if m.noteLoading() {
		return "\n" + m.miniLoader(loaderNote)
	}
	return m.viewer.View()
}

func (m Model) treeView(width int) string {
	name := "Vault"
	if len(m.workspaces) > 0 {
		name = m.workspaces[m.workspace].Name
	}
	header := accentStyle.Render(ansi.Truncate(" "+m.withIcon(m.icons().Logo, name), width, "…"))
	lines := []string{header, faintStyle.Render(strings.Repeat("─", width))}

	if m.loadingWorkspace != "" {
		return strings.Join(append(lines, m.skeletonRows(width, min(m.treeRows(), 12))...), "\n")
	}
	if len(m.visible) == 0 {
		lines = append(lines, mutedStyle.Render(" No markdown notes yet"))
		return strings.Join(lines, "\n")
	}

	end := min(len(m.visible), m.offset+m.treeRows())
	for i := m.offset; i < end; i++ {
		node := m.visible[i]
		if node.Divider {
			lines = append(lines, m.treeDivider(width))
			continue
		}
		if node.External {
			lines = append(lines, m.externalRow(node, width, i == m.cursor))
			continue
		}
		indent := strings.Repeat("  ", node.Depth)
		ic := m.icons()
		var chevron, glyph, label string
		if node.IsFolder {
			chevron, glyph = "▸ ", ic.FolderClosed
			if node.Expanded {
				chevron, glyph = "▾ ", ic.FolderOpen
			}
			label = node.Label() + "/"
		} else {
			chevron, glyph = "  ", ic.Note
			if node.IsBoard {
				glyph = ic.Board
			}
			if node.IsCSV {
				glyph = ic.Table
			}
			label = node.Label()
		}
		row := ansi.Truncate(" "+indent+chevron+m.withIcon(glyph, label), width, "…")
		pad := strings.Repeat(" ", max(0, width-lipgloss.Width(row)))

		switch {
		case i == m.cursor && m.focus == focusTree:
			lines = append(lines, cursorStyle.Render(row+pad))
		case i == m.cursor:
			lines = append(lines, cursorDimmed.Render(row+pad))
		case node.IsFolder:
			lines = append(lines, folderStyle.Render(row))
		case node.ID == m.openID:
			lines = append(lines, successStyle.Render(row))
		default:
			lines = append(lines, fileStyle.Render(row))
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) treeDivider(width int) string {
	label := "─ in browser "
	return faintStyle.Render(ansi.Truncate(label+strings.Repeat("─", max(0, width-lipgloss.Width(label))), width, ""))
}

func (m Model) externalRow(node *tree.Node, width int, cursor bool) string {
	row := " " + m.withIcon(m.icons().External, node.Label())
	dir := ""
	if node.Dir != "" {
		dir = "  " + node.Dir
	}
	switch {
	case cursor:
		full := ansi.Truncate(row+dir, width, "…")
		style := cursorDimmed
		if m.focus == focusTree {
			style = cursorStyle
		}
		return style.Render(full + strings.Repeat(" ", max(0, width-lipgloss.Width(full))))
	case lipgloss.Width(row) >= width:
		return mutedStyle.Render(ansi.Truncate(row, width, "…"))
	default:
		return mutedStyle.Render(row) + faintStyle.Render(ansi.Truncate(dir, width-lipgloss.Width(row), "…"))
	}
}

func hint(key, label string, enabled bool) string {
	if !enabled {
		return faintStyle.Render(key + " " + label)
	}
	return keyStyle.Render(key) + " " + label
}

// browsingExternal reports whether the tree cursor is on a file that only opens in the browser.
func (m Model) browsingExternal() bool {
	node := m.selected()
	return m.focus == focusTree && !m.sidebarHidden && node != nil && node.External
}

func (m Model) statusBar() string {
	table := m.tableNode()
	external := m.browsingExternal() || table != nil
	hints := []string{
		hint("e", "edit", !external),
		hint("n", "new", true),
		hint("y", "copy", !external),
		hint("b", "sidebar", true),
		hint("ctrl+k", "actions", true),
		hint(",", "settings", true),
		hint("?", "help", true),
		hint("q", "quit", true),
	}
	switch {
	case table != nil && m.focus == focusViewer:
		hints = append([]string{hint("f", "find", true), hint("ctrl+g", "row", true), hint("o", "browser", true)}, hints...)
	case table != nil:
		hints = append([]string{hint("⏎", "scroll", true), hint("o", "browser", true)}, hints...)
	case external:
		hints = append([]string{hint("⏎", "browser", true)}, hints...)
	case m.showsBoard(m.openID):
		hints = append([]string{keyStyle.Render("⏎") + " board", keyStyle.Render("v") + " markdown"}, hints...)
	case m.isBoard(m.openID):
		hints = append([]string{keyStyle.Render("v") + " board"}, hints...)
	}
	left := strings.Join(hints, "  ")
	switch {
	case m.mode == modeNewNote:
		left = m.newNotePrompt()
	case m.mode == modeBoard && m.boardPrompt == promptAddColumn:
		left = m.boardPromptView()
	case m.renderingDiagrams:
		left = m.miniLoader(loaderDiagram)
	case m.err != nil:
		left = errorStyle.Render("✕ " + m.err.Error())
	case m.flash != "":
		left = successStyle.Render(m.flash)
	case m.mode == modeBoard:
		left = m.boardHints()
	case m.mode == modeCardDraft:
		left = m.draftHints()
	case m.mode == modeEdit:
		left = keyStyle.Render("ctrl+s") + " save  " + keyStyle.Render("ctrl+o") + " open in $EDITOR  " + keyStyle.Render("esc") + " close"
	}
	right := ""
	switch {
	case m.updateInfo != nil:
		right = m.updateBadge()
	case m.user != nil:
		right = faintStyle.Render(m.user.Email)
	}
	if right != "" {
		left = ansi.Truncate(left, max(0, m.width-3-lipgloss.Width(right)), "…")
	}
	gap := max(1, m.width-2-lipgloss.Width(left)-lipgloss.Width(right))
	line := left + strings.Repeat(" ", gap) + right
	return statusStyle.Render(ansi.Truncate(line, m.width-2, "…"))
}

func (m Model) updateBadge() string {
	return updateStyle.Render("⬆ "+m.updateInfo.Latest+" available") + " " + keyStyle.Render("u") + faintStyle.Render(" update")
}
