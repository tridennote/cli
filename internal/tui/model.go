package tui

import (
	"errors"
	"runtime"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/tridennote/cli/internal/api"
	"github.com/tridennote/cli/internal/kanban"
	"github.com/tridennote/cli/internal/session"
	"github.com/tridennote/cli/internal/settings"
	"github.com/tridennote/cli/internal/tree"
	"github.com/tridennote/cli/internal/update"
)

type state int

const (
	stateBoot state = iota
	stateLogin
	stateLoading
	stateBrowse
)

type focus int

const (
	focusTree focus = iota
	focusViewer
)

type Model struct {
	client     *api.Client
	version    string
	updateInfo *update.Info
	state      state
	width      int
	height     int
	err        error

	frame     int
	animating bool

	device        *api.DeviceCode
	browserOpened bool

	user       *api.User
	workspaces []api.Workspace
	workspace  int
	roots      []*tree.Node
	visible    []*tree.Node
	cursor     int
	offset     int
	focus      focus

	viewer      viewport.Model
	notes       map[string]api.Note
	rendered    map[string]string
	openID      string
	loadingNote string
	previewSeq  int

	mode            mode
	editor          textarea.Model
	editID          string
	editBaseline    string
	confirmDiscard  bool
	saving          bool
	pending         *pendingOpen
	nameInput       textinput.Model
	newParent       *tree.Node
	newBoard        bool
	selectAfterLoad string
	flash           string

	settings           settings.Settings
	modal              modal
	settingsCursor     int
	editorAvailability map[string]bool
	sidebarHidden      bool
	renderingDiagrams  bool
	externals          []*tree.Node
	csvs               map[string]csvTable
	loadingCSV         string
	csvID              string
	csvTop             int
	csvLeft            int
	csvSelRow          int
	csvSelCol          int
	csvPrompt          csvPromptKind
	csvInput           textinput.Model
	csvQuery           string
	csvMatches         [][2]int
	csvRows            []int
	csvOrigin          [2]int
	csvNotice          string
	viewerPlaceholder  string
	diagramNoteID      string
	diagrams           []diagramEntry
	diagramCursor      int
	diagramPicked      []bool
	workspaceCursor    int
	loadingWorkspace   string

	board        kanban.Board
	boardID      string
	boardCol     int
	boardRow     int
	boardFocus   focus
	boardPrompt  boardPrompt
	boardInput   textinput.Model
	boardConfirm bool
	rawBoards    bool
	pendingSaves map[string]bool

	people          []kanban.Person
	tags            []string
	metaWorkspace   string
	suggestIndex    int
	suggestHidden   bool
	draftBoard      string
	draftCol        int
	draftTitleFocus bool
	returnBoard     string
	returnCol       int
	returnRow       int

	actionInput  textinput.Model
	actionCursor int
	addNoteID    string
	addBoards    []*tree.Node
	addStep      int
	addBoard     int
	addCol       int
}

func New(client *api.Client) Model {
	return Model{
		client:      client,
		viewer:      viewport.New(0, 0),
		editor:      newEditor(),
		nameInput:   newNameInput(),
		boardInput:  newBoardInput(),
		actionInput: newActionInput(),
		settings:    settings.Load(),
		notes:       map[string]api.Note{},
		rendered:    map[string]string{},
		csvs:        map[string]csvTable{},
		csvInput:    newCSVInput(),
	}
}

type sessionCheckedMsg struct {
	user *api.User
	err  error
}

type deviceStartedMsg struct {
	code api.DeviceCode
	err  error
}

type pollTickMsg struct{ deviceCode string }

type pollResultMsg struct {
	deviceCode string
	result     api.DevicePoll
	err        error
}

type workspacesMsg struct {
	workspaces []api.Workspace
	err        error
}

type treeMsg struct {
	workspaceID string
	rows        []api.TreeNode
	err         error
}

type previewTickMsg struct{ seq int }

type noteMsg struct {
	id   string
	note api.Note
	err  error
}

func (m Model) WithVersion(version string) Model {
	m.version = version
	return m
}

type updateMsg struct{ info *update.Info }

func (m Model) checkUpdate() tea.Cmd {
	version := m.version
	if version == "" {
		return nil
	}
	return func() tea.Msg {
		info, _ := update.Check(version, runtime.GOOS)
		return updateMsg{info: info}
	}
}

func (m *Model) copyUpdateCommand() {
	if m.updateInfo == nil {
		return
	}
	if err := copyToClipboard(m.updateInfo.Command); err != nil {
		m.flash = "Run this to update: " + m.updateInfo.Command
		return
	}
	m.flash = m.icons().Copy + " Copied the " + string(m.updateInfo.Method) + " update command — quit (q) and paste it in your shell"
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.checkSession(), m.checkUpdate())
}

func (m Model) checkSession() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		if client.Token == "" {
			return sessionCheckedMsg{err: api.ErrUnauthorized}
		}
		user, err := client.CurrentUser()
		return sessionCheckedMsg{user: user, err: err}
	}
}

func (m Model) startDevice() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		code, err := client.StartDeviceAuth()
		return deviceStartedMsg{code: code, err: err}
	}
}

func pollAfter(deviceCode string, interval int) tea.Cmd {
	if interval <= 0 {
		interval = 3
	}
	return tea.Tick(time.Duration(interval)*time.Second, func(time.Time) tea.Msg {
		return pollTickMsg{deviceCode: deviceCode}
	})
}

func (m Model) poll(deviceCode string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		result, err := client.PollDeviceAuth(deviceCode)
		return pollResultMsg{deviceCode: deviceCode, result: result, err: err}
	}
}

func (m Model) loadWorkspaces() tea.Cmd {
	client := m.client
	return func() tea.Msg {
		workspaces, err := client.Workspaces()
		return workspacesMsg{workspaces: workspaces, err: err}
	}
}

func (m Model) loadTree() tea.Cmd {
	if len(m.workspaces) == 0 {
		return nil
	}
	client := m.client
	id := m.workspaces[m.workspace].ID
	return func() tea.Msg {
		rows, err := client.Tree(id)
		return treeMsg{workspaceID: id, rows: rows, err: err}
	}
}

func (m Model) loadNote(id string) tea.Cmd {
	client := m.client
	return func() tea.Msg {
		note, err := client.Note(id)
		return noteMsg{id: id, note: note, err: err}
	}
}

func (m *Model) signedOut() tea.Cmd {
	_ = session.Clear()
	m.client.Token = ""
	m.user = nil
	m.device = nil
	m.browserOpened = false
	m.state = stateLogin
	m.roots, m.visible, m.externals = nil, nil, nil
	m.notes = map[string]api.Note{}
	m.rendered = map[string]string{}
	m.openID = ""
	m.mode = modeBrowse
	m.editID = ""
	m.pending = nil
	m.flash = ""
	return m.startDevice()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(animTickMsg); ok {
		m.frame++
		if m.needsAnimation() {
			return m, animTick()
		}
		m.animating = false
		return m, nil
	}
	next, cmd := m.update(msg)
	model := next.(Model)
	if !model.animating && model.needsAnimation() {
		model.animating = true
		cmd = tea.Batch(cmd, animTick())
	}
	return model, cmd
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if cmd, handled := m.handleEditMsg(msg); handled {
		return m, cmd
	}
	if cmd, handled := m.handleDiagramMsg(msg); handled {
		return m, cmd
	}
	if cmd, handled := m.handleBoardMsg(msg); handled {
		return m, cmd
	}
	if cmd, handled := m.handleCardMsg(msg); handled {
		return m, cmd
	}
	if m.handleMetaMsg(msg) {
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.layout()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case updateMsg:
		m.updateInfo = msg.info
		return m, nil

	case sessionCheckedMsg:
		if msg.err != nil {
			if errors.Is(msg.err, api.ErrUnauthorized) {
				return m, m.signedOut()
			}
			m.err = msg.err
			return m, nil
		}
		m.user = msg.user
		m.state = stateLoading
		return m, m.loadWorkspaces()

	case deviceStartedMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.device = &msg.code
		return m, pollAfter(msg.code.DeviceCode, msg.code.Interval)

	case pollTickMsg:
		if m.state != stateLogin || m.device == nil || m.device.DeviceCode != msg.deviceCode {
			return m, nil
		}
		return m, m.poll(msg.deviceCode)

	case pollResultMsg:
		if m.state != stateLogin || m.device == nil || m.device.DeviceCode != msg.deviceCode {
			return m, nil
		}
		if msg.err != nil {
			m.err = msg.err
			return m, pollAfter(msg.deviceCode, m.device.Interval*2)
		}
		m.err = nil
		switch msg.result.Status {
		case "authorized":
			m.client.Token = msg.result.SessionToken
			m.client.CookieName = msg.result.CookieName
			if err := session.Save(session.Session{Token: m.client.Token, CookieName: m.client.CookieName}); err != nil {
				m.err = err
			}
			m.state = stateBoot
			return m, m.checkSession()
		case "expired":
			m.device = nil
			m.browserOpened = false
			return m, m.startDevice()
		default:
			return m, pollAfter(msg.deviceCode, m.device.Interval)
		}

	case workspacesMsg:
		if msg.err != nil {
			return m.handleAPIError(msg.err)
		}
		m.workspaces = msg.workspaces
		m.workspace = 0
		for i, w := range m.workspaces {
			if w.IsDefault {
				m.workspace = i
			}
		}
		return m, m.loadTree()

	case treeMsg:
		if len(m.workspaces) == 0 || m.workspaces[m.workspace].ID != msg.workspaceID {
			return m, nil
		}
		switched := m.loadingWorkspace == msg.workspaceID
		m.loadingWorkspace = ""
		if msg.err != nil {
			m.refreshViewer(true)
			return m.handleAPIError(msg.err)
		}
		if switched {
			m.flash = "Switched to " + m.workspaces[m.workspace].Name
		}
		expanded := tree.ExpandedIDs(m.roots)
		selected := ""
		if node := m.selected(); node != nil {
			selected = node.ID
		}
		m.roots = tree.Build(msg.rows)
		m.externals = tree.Externals(msg.rows)
		tree.Restore(m.roots, expanded)
		m.visible = tree.Rows(m.roots, m.externals)
		m.cursor = 0
		for i, n := range m.visible {
			if selected != "" && n.ID == selected {
				m.cursor = i
			}
		}
		if m.selectAfterLoad != "" {
			m.selectByID(m.selectAfterLoad)
			m.selectAfterLoad = ""
		}
		m.state = stateBrowse
		m.err = nil
		m.layout()
		if m.mode == modeEdit || m.mode == modeBoard || m.mode == modeCardDraft {
			return m, nil
		}
		return m, m.schedulePreview()

	case previewTickMsg:
		if msg.seq != m.previewSeq || m.mode == modeEdit || m.mode == modeBoard || m.mode == modeCardDraft {
			return m, nil
		}
		node := m.selected()
		if m.viewerPlaceholder != "" || (node != nil && node.External) {
			m.refreshViewer(false)
		}
		if isCSV(node) {
			return m, m.showCSV(node)
		}
		if node == nil || node.IsFolder || node.External {
			return m, nil
		}
		return m, m.open(node.ID)

	case csvMsg:
		return m.handleCSV(msg)

	case noteMsg:
		if m.loadingNote == msg.id {
			m.loadingNote = ""
		}
		if msg.err != nil {
			return m.handleAPIError(msg.err)
		}
		m.notes[msg.id] = msg.note
		if m.openID == msg.id && m.mode != modeEdit {
			m.refreshViewer(true)
		}
		if m.pending != nil && m.pending.id == msg.id {
			pending := *m.pending
			m.pending = nil
			m.flash = ""
			return m, m.runPending(pending)
		}
		return m, nil
	}

	var cmd tea.Cmd
	if m.modal == modalActions {
		m.actionInput, cmd = m.actionInput.Update(msg)
		return m, cmd
	}
	if m.csvPrompt != csvPromptNone {
		m.csvInput, cmd = m.csvInput.Update(msg)
		return m, cmd
	}
	switch m.mode {
	case modeEdit:
		m.editor, cmd = m.editor.Update(msg)
	case modeNewNote:
		m.nameInput, cmd = m.nameInput.Update(msg)
	case modeCardDraft:
		if m.draftTitleFocus {
			m.boardInput, cmd = m.boardInput.Update(msg)
		} else {
			m.editor, cmd = m.editor.Update(msg)
		}
	case modeBoard:
		if m.boardPrompt != promptNone {
			m.boardInput, cmd = m.boardInput.Update(msg)
		}
	}
	return m, cmd
}

func (m Model) handleAPIError(err error) (tea.Model, tea.Cmd) {
	if errors.Is(err, api.ErrUnauthorized) {
		return m, m.signedOut()
	}
	m.err = err
	return m, nil
}

func (m *Model) open(id string) tea.Cmd {
	if m.openID != id {
		m.openID = id
		m.refreshViewer(true)
	}
	if _, cached := m.notes[id]; cached || m.loadingNote == id {
		return nil
	}
	m.loadingNote = id
	return m.loadNote(id)
}

func (m *Model) schedulePreview() tea.Cmd {
	m.previewSeq++
	seq := m.previewSeq
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return previewTickMsg{seq: seq} })
}

func (m Model) selected() *tree.Node {
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		return nil
	}
	return m.visible[m.cursor]
}

func (m *Model) moveCursor(to int) tea.Cmd {
	if len(m.visible) == 0 {
		return nil
	}
	to = max(0, min(to, len(m.visible)-1))
	if m.visible[to].Divider {
		if to > m.cursor {
			to++
		} else {
			to--
		}
		to = max(0, min(to, len(m.visible)-1))
	}
	if to == m.cursor {
		return nil
	}
	m.cursor = to
	m.ensureCursorVisible()
	return m.schedulePreview()
}

func (m *Model) relayoutTree(keep *tree.Node) {
	m.visible = tree.Rows(m.roots, m.externals)
	for i, n := range m.visible {
		if n == keep {
			m.cursor = i
		}
	}
	m.ensureCursorVisible()
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" && (m.state != stateBrowse || m.mode == modeBrowse) {
		return *m, tea.Quit
	}

	switch m.state {
	case stateLogin:
		switch key {
		case "u":
			m.copyUpdateCommand()
		case "q", "esc":
			return *m, tea.Quit
		case "enter", "o":
			if m.device != nil {
				m.browserOpened = true
				if err := openBrowser(m.device.VerificationURL); err != nil {
					m.err = err
				}
			}
		case "r":
			if m.err != nil {
				m.err = nil
				return *m, m.startDevice()
			}
		}
		return *m, nil
	case stateBrowse:
		return m.handleBrowseKey(key, msg)
	default:
		if key == "q" {
			return *m, tea.Quit
		}
		if key == "r" && m.err != nil {
			m.err = nil
			m.state = stateBoot
			return *m, m.checkSession()
		}
	}
	return *m, nil
}

func (m *Model) handleBrowseKey(key string, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case modeEdit:
		return *m, m.handleEditKey(msg)
	case modeNewNote:
		return *m, m.handleNewNoteKey(msg)
	case modeCardDraft:
		return *m, m.handleDraftKey(msg)
	}
	if m.modal != modalNone {
		return *m, m.handleModalKey(msg)
	}
	if m.csvPrompt != csvPromptNone {
		return *m, m.handleCSVPromptKey(msg)
	}
	if key == "ctrl+k" && !(m.mode == modeBoard && m.boardPrompt != promptNone) {
		return *m, m.openActions()
	}
	if key == "u" && m.updateInfo != nil && (m.mode == modeBrowse || (m.mode == modeBoard && m.boardPrompt == promptNone)) {
		m.copyUpdateCommand()
		return *m, nil
	}
	if m.mode == modeBoard {
		return *m, m.handleBoardKey(msg)
	}
	m.flash = ""
	if handled, cmd := m.handleCSVKey(key); handled {
		return *m, cmd
	}
	switch key {
	case ",":
		m.openSettings()
		return *m, nil
	case "?":
		m.modal = modalHelp
		return *m, nil
	case "b", "ctrl+b":
		m.sidebarHidden = !m.sidebarHidden
		if m.sidebarHidden {
			m.focus = focusViewer
		} else {
			m.focus = focusTree
		}
		m.layout()
		return *m, nil
	case "n":
		return *m, m.startNewNote(false)
	case "N":
		return *m, m.startNewNote(true)
	case "v":
		m.toggleBoardView()
		return *m, nil
	case "o":
		return *m, m.openTargetInBrowser()
	case "m":
		if m.externalSelected() {
			return *m, nil
		}
		target := m.targetNote()
		if target == "" {
			m.flash = "Select a note first"
			return *m, nil
		}
		return *m, m.requestDiagrams(target)
	case "e", "i", "y", "c":
		if m.externalSelected() {
			return *m, nil
		}
		target := m.targetNote()
		if target == "" {
			m.flash = "Select a note first, or press n for a new one"
			return *m, nil
		}
		if key == "y" || key == "c" {
			return *m, m.requestCopy(target)
		}
		external := key == "e" && m.settings.Editor != builtinEditor
		return *m, m.requestEdit(target, external)
	case "q":
		return *m, tea.Quit
	case "L":
		return *m, m.signedOut()
	case "r":
		m.notes = map[string]api.Note{}
		m.rendered = map[string]string{}
		m.csvs = map[string]csvTable{}
		return *m, m.loadTree()
	case "w":
		m.openWorkspaces()
		return *m, nil
	case "tab", "left", "right", "ctrl+h", "ctrl+l":
		m.switchPane()
		return *m, nil
	case "pgdown", "ctrl+d":
		m.viewer.HalfPageDown()
		return *m, nil
	case "pgup", "ctrl+u":
		m.viewer.HalfPageUp()
		return *m, nil
	}

	if m.focus == focusViewer {
		switch key {
		case "esc":
			if !m.sidebarHidden {
				m.focus = focusTree
			}
			return *m, nil
		case "h", "l":
			m.switchPane()
			return *m, nil
		case "enter":
			if m.showsBoard(m.openID) {
				return *m, m.requestBoard(m.openID)
			}
			return *m, nil
		case "g", "home":
			m.viewer.GotoTop()
			return *m, nil
		case "G", "end":
			m.viewer.GotoBottom()
			return *m, nil
		}
		var cmd tea.Cmd
		m.viewer, cmd = m.viewer.Update(msg)
		return *m, cmd
	}

	node := m.selected()
	switch key {
	case "up", "k":
		return *m, m.moveCursor(wrapIndex(m.cursor-1, len(m.visible)))
	case "down", "j":
		return *m, m.moveCursor(wrapIndex(m.cursor+1, len(m.visible)))
	case "g", "home":
		return *m, m.moveCursor(0)
	case "G", "end":
		return *m, m.moveCursor(len(m.visible) - 1)
	case "l":
		if node == nil {
			return *m, nil
		}
		if node.IsFolder {
			if !node.Expanded {
				node.Expanded = true
				m.relayoutTree(node)
				return *m, nil
			}
			if len(node.Children) > 0 {
				return *m, m.moveCursor(m.cursor + 1)
			}
			return *m, nil
		}
		if isCSV(node) {
			m.focus = focusViewer
			return *m, m.showCSV(node)
		}
		if node.External {
			return *m, m.openInBrowser(node)
		}
		return *m, m.open(node.ID)
	case "h":
		if node == nil {
			return *m, nil
		}
		if node.IsFolder && node.Expanded {
			node.Expanded = false
			m.relayoutTree(node)
			return *m, nil
		}
		if node.Parent != nil {
			node.Parent.Expanded = false
			m.relayoutTree(node.Parent)
			return *m, m.schedulePreview()
		}
		return *m, nil
	case "enter", " ":
		if node == nil {
			return *m, nil
		}
		if node.IsFolder {
			node.Expanded = !node.Expanded
			m.relayoutTree(node)
			return *m, nil
		}
		if isCSV(node) {
			m.focus = focusViewer
			return *m, m.showCSV(node)
		}
		if node.External {
			return *m, m.openInBrowser(node)
		}
		if m.showsBoard(node.ID) {
			return *m, m.requestBoard(node.ID)
		}
		return *m, m.open(node.ID)
	}
	return *m, nil
}

func (m Model) targetNote() string {
	if node := m.selected(); m.focus == focusTree && !m.sidebarHidden && node != nil && !node.IsFolder && !node.External && !node.IsCSV {
		return node.ID
	}
	return m.openID
}

func wrapIndex(i, n int) int {
	if n == 0 {
		return 0
	}
	return (i%n + n) % n
}

func (m *Model) switchPane() {
	if m.sidebarHidden {
		return
	}
	if m.focus == focusTree {
		m.focus = focusViewer
	} else {
		m.focus = focusTree
	}
}
