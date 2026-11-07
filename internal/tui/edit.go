package tui

import (
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/tridennote/cli/internal/api"
	"github.com/tridennote/cli/internal/kanban"
	"github.com/tridennote/cli/internal/markdown"
	"github.com/tridennote/cli/internal/tree"
)

type mode int

const (
	modeBrowse mode = iota
	modeEdit
	modeNewNote
	modeBoard
)

type pendingAction int

const (
	pendingEditBuiltin pendingAction = iota
	pendingEditExternal
	pendingCopy
	pendingDiagram
	pendingBoard
)

type pendingOpen struct {
	id     string
	action pendingAction
}

type noteSavedMsg struct {
	id   string
	text string
	note api.Note
	err  error
}

type noteCreatedMsg struct {
	node api.TreeNode
	err  error
}

type externalDoneMsg struct {
	id       string
	path     string
	baseline string
	err      error
}

var unsafeFileChars = regexp.MustCompile(`[^\w\- ]+`)

func newEditor() textarea.Model {
	ta := textarea.New()
	ta.ShowLineNumbers = true
	ta.Prompt = " "
	ta.CharLimit = 0
	ta.MaxHeight = 0
	ta.MaxWidth = 0
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle().Background(lipgloss.Color("#22223a"))
	ta.FocusedStyle.LineNumber = faintStyle
	ta.FocusedStyle.CursorLineNumber = accentStyle
	ta.BlurredStyle.LineNumber = faintStyle
	return ta
}

func newNameInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "Untitled"
	ti.Prompt = ""
	ti.CharLimit = 200
	ti.TextStyle = lipgloss.NewStyle().Foreground(text)
	return ti
}

func (m Model) noteText(id string) (string, bool) {
	note, ok := m.notes[id]
	if !ok {
		return "", false
	}
	if len(note.Blocks) == 0 {
		return "", true
	}
	return markdown.FromBlocks(note.Blocks), true
}

func (m Model) nodeName(id string) string {
	for _, n := range m.visible {
		if n.ID == id {
			return n.Name
		}
	}
	if note, ok := m.notes[id]; ok {
		return note.Title
	}
	return "note"
}

func (m *Model) requestEdit(id string, external bool) tea.Cmd {
	if _, ok := m.notes[id]; ok {
		if external {
			return m.launchExternal(id)
		}
		m.beginEditor(id)
		return textarea.Blink
	}
	action := pendingEditBuiltin
	if external {
		action = pendingEditExternal
	}
	m.pending = &pendingOpen{id: id, action: action}
	m.flash = "Loading note…"
	return m.open(id)
}

func (m *Model) requestCopy(id string) tea.Cmd {
	content, ok := m.noteText(id)
	if !ok {
		m.pending = &pendingOpen{id: id, action: pendingCopy}
		m.flash = "Loading note…"
		return m.open(id)
	}
	if err := copyToClipboard(content); err != nil {
		m.flash = "Could not copy: " + err.Error()
		return nil
	}
	m.flash = m.icons().Copy + " Copied " + m.nodeName(id) + ".md to the clipboard"
	return nil
}

func (m *Model) runPending(p pendingOpen) tea.Cmd {
	switch p.action {
	case pendingCopy:
		return m.requestCopy(p.id)
	case pendingDiagram:
		return m.requestDiagrams(p.id)
	case pendingBoard:
		return m.requestBoard(p.id)
	case pendingEditExternal:
		return m.requestEdit(p.id, true)
	default:
		return m.requestEdit(p.id, false)
	}
}

func (m *Model) beginEditor(id string) {
	content, _ := m.noteText(id)
	m.openID = id
	m.editID = id
	m.editBaseline = content
	m.confirmDiscard = false
	m.mode = modeEdit
	m.focus = focusViewer
	m.editor.SetValue(content)
	m.editor.Focus()
	m.layout()
}

func (m *Model) closeEditor() {
	m.mode = modeBrowse
	m.editor.Blur()
	m.editID = ""
	m.confirmDiscard = false
	m.refreshViewer(false)
}

func (m Model) dirty() bool {
	return m.mode == modeEdit && m.editor.Value() != m.editBaseline
}

func (m *Model) save(id, content string) tea.Cmd {
	previous := m.notes[id].Blocks
	blocks := markdown.Reconcile(previous, markdown.ToBlocks(content))
	client := m.client
	m.saving = true
	m.flash = "Saving…"
	return func() tea.Msg {
		note, err := client.SaveNote(id, blocks)
		return noteSavedMsg{id: id, text: content, note: note, err: err}
	}
}

func (m *Model) launchExternal(id string) tea.Cmd {
	command, editorName := externalEditorCommand(m.settings.Editor)
	if command == nil {
		m.flash = "No external editor found — pick one in settings (,)"
		return nil
	}
	baseline, _ := m.noteText(id)
	content := baseline
	if m.mode == modeEdit && m.editID == id {
		baseline = m.editBaseline
		content = m.editor.Value()
	}
	name := strings.TrimSpace(unsafeFileChars.ReplaceAllString(m.nodeName(id), ""))
	if name == "" {
		name = "note"
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
	m.flash = m.icons().Edit + " Editing in " + editorName + " — close the file to come back"
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return externalDoneMsg{id: id, path: path, baseline: baseline, err: err}
	})
}

func (m *Model) startNewNote(board bool) tea.Cmd {
	if len(m.workspaces) == 0 {
		return nil
	}
	m.newBoard = board
	m.newParent = nil
	if node := m.selected(); node != nil {
		switch {
		case node.IsFolder:
			m.newParent = node
		case node.Parent != nil:
			m.newParent = node.Parent
		}
	}
	m.mode = modeNewNote
	m.nameInput.SetValue("")
	return m.nameInput.Focus()
}

func (m *Model) createNote(name string) tea.Cmd {
	var parentID *string
	if m.newParent != nil {
		id := m.newParent.ID
		parentID = &id
	}
	workspaceID := m.workspaces[m.workspace].ID
	client := m.client
	m.flash = "Creating " + fileLabel(name) + "…"
	return func() tea.Msg {
		node, err := client.CreateNote(workspaceID, parentID, name)
		return noteCreatedMsg{node: node, err: err}
	}
}

func fileLabel(name string) string {
	if kanban.IsBoardName(name) {
		return kanban.DisplayName(name) + " board"
	}
	return name + ".md"
}

func (m Model) findNode(id string) *tree.Node {
	var find func([]*tree.Node) *tree.Node
	find = func(nodes []*tree.Node) *tree.Node {
		for _, n := range nodes {
			if n.ID == id {
				return n
			}
			if found := find(n.Children); found != nil {
				return found
			}
		}
		return nil
	}
	return find(m.roots)
}

func (m *Model) selectByID(id string) {
	target := m.findNode(id)
	if target == nil {
		return
	}
	for p := target.Parent; p != nil; p = p.Parent {
		p.Expanded = true
	}
	m.relayoutTree(target)
}

func (m *Model) handleEditKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+s":
		if m.saving {
			return nil
		}
		if !m.dirty() {
			m.flash = "No changes to save"
			return nil
		}
		return m.save(m.editID, m.editor.Value())
	case "ctrl+o":
		return m.launchExternal(m.editID)
	case "esc", "ctrl+c":
		if m.dirty() && !m.confirmDiscard {
			m.confirmDiscard = true
			m.flash = "Unsaved changes — ctrl+s to save, esc again to discard"
			return nil
		}
		m.flash = ""
		m.closeEditor()
		return nil
	}
	m.confirmDiscard = false
	var cmd tea.Cmd
	m.editor, cmd = m.editor.Update(msg)
	return cmd
}

func (m *Model) handleNewNoteKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.mode = modeBrowse
		m.nameInput.Blur()
		return nil
	case "enter":
		name := strings.TrimSpace(m.nameInput.Value())
		name = strings.TrimSpace(strings.TrimSuffix(name, ".md"))
		if m.newBoard || kanban.IsBoardName(name) {
			name = strings.TrimSpace(kanban.DisplayName(name))
			if name == "" {
				name = "Untitled"
			}
			name += kanban.Extension
		}
		if name == "" {
			name = "Untitled"
		}
		m.mode = modeBrowse
		m.nameInput.Blur()
		return m.createNote(name)
	}
	var cmd tea.Cmd
	m.nameInput, cmd = m.nameInput.Update(msg)
	return cmd
}

func (m *Model) handleEditMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case noteSavedMsg:
		m.saving = false
		if msg.err != nil {
			m.flash = ""
			m.err = msg.err
			return nil, true
		}
		m.err = nil
		m.notes[msg.id] = msg.note
		m.dropRendered(msg.id)
		if m.mode == modeEdit && m.editID == msg.id {
			m.editBaseline = msg.text
			m.confirmDiscard = false
		}
		m.flash = m.icons().Saved + " Saved"
		m.refreshViewer(false)
		return nil, true

	case noteCreatedMsg:
		if msg.err != nil {
			m.flash = ""
			m.err = msg.err
			return nil, true
		}
		m.err = nil
		m.selectAfterLoad = msg.node.ID
		if kanban.IsBoardName(msg.node.Name) {
			m.notes[msg.node.ID] = api.Note{Title: msg.node.Name, Blocks: kanban.New().Blocks()}
			m.enterBoard(msg.node.ID)
			m.flash = "Created " + fileLabel(msg.node.Name) + " — a to add a card"
			return tea.Batch(m.loadTree(), m.saveBoard(msg.node.ID)), true
		}
		m.notes[msg.node.ID] = api.Note{Title: msg.node.Name}
		m.beginEditor(msg.node.ID)
		m.flash = "Created " + msg.node.Name + ".md — ctrl+s to save"
		return tea.Batch(m.loadTree(), textarea.Blink), true

	case externalDoneMsg:
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
		if m.mode == modeEdit && m.editID == msg.id {
			m.editor.SetValue(content)
		}
		if content == msg.baseline {
			m.flash = "No changes"
			return nil, true
		}
		return m.save(msg.id, content), true
	}
	return nil, false
}

func (m *Model) dropRendered(id string) {
	for key := range m.rendered {
		if strings.HasPrefix(key, id+":") {
			delete(m.rendered, key)
		}
	}
}

func (m Model) editorView() string {
	marker := ""
	if m.dirty() {
		marker = accentStyle.Render(" ●")
	}
	name := ansi.Truncate(" "+m.withIcon(m.icons().Edit, m.nodeName(m.editID)+".md"), max(4, m.viewer.Width-2), "…")
	return titleStyle.Render(name) + marker + "\n" + m.editor.View()
}

func (m Model) newNotePrompt() string {
	where := "vault root"
	if m.newParent != nil {
		where = m.newParent.Name + "/"
	}
	what := "New note"
	if m.newBoard {
		what = "New board"
	}
	return accentStyle.Render(what+" in "+where+": ") + m.nameInput.View() +
		faintStyle.Render("   enter create · esc cancel")
}
