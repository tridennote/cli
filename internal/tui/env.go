package tui

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/tridennote/cli/internal/api"
	"github.com/tridennote/cli/internal/envfile"
)

const envMask = "••••••••••••"
const envUploadBatch = 50

type envPane int

const (
	envPaneFiles envPane = iota
	envPaneVars
)

type envPrompt int

const (
	envPromptNone envPrompt = iota
	envPromptImport
	envPromptPasteName
	envPromptFilter
	envPromptCopy
	envPromptDownload
	envPromptDelete
	envPromptOverwrite
)

type envScreen struct {
	workspaceID string
	files       []api.EnvFileSummary
	loaded      bool
	fileCursor  int
	fileOffset  int
	pane        envPane
	file        *api.EnvFile
	loadingFile string
	varCursor   int
	varOffset   int
	selected    map[string]bool
	revealed    map[string]bool
	revealAll   bool
	filter      string
	prompt      envPrompt
	input       textinput.Model
	clip        string
	writePath   string
	writeBody   string
	busy        bool
	selectAfter string
}

type envListMsg struct {
	workspaceID string
	files       []api.EnvFileSummary
	err         error
}

type envFileMsg struct {
	id   string
	file api.EnvFile
	err  error
}

type envUploadMsg struct {
	workspaceID string
	result      api.EnvUploadResult
	skipped     int
	err         error
}

type envDeleteMsg struct {
	workspaceID string
	name        string
	err         error
}

func newEnvInput() textinput.Model {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 1024
	ti.TextStyle = lipgloss.NewStyle().Foreground(text)
	return ti
}

func (m *Model) openEnv() tea.Cmd {
	if len(m.workspaces) == 0 {
		return nil
	}
	m.mode = modeEnv
	m.flash = ""
	m.err = nil
	m.env = envScreen{
		workspaceID: m.workspaces[m.workspace].ID,
		selected:    map[string]bool{},
		revealed:    map[string]bool{},
		input:       newEnvInput(),
	}
	return m.loadEnvList()
}

func (m *Model) closeEnv() {
	m.mode = modeBrowse
	m.env = envScreen{}
	m.flash = ""
	m.err = nil
}

func (m Model) loadEnvList() tea.Cmd {
	client, id := m.client, m.env.workspaceID
	return func() tea.Msg {
		files, err := client.EnvFiles(id)
		return envListMsg{workspaceID: id, files: files, err: err}
	}
}

func (m *Model) loadEnvFile() tea.Cmd {
	summary := m.envSelectedSummary()
	if summary == nil {
		m.env.file = nil
		return nil
	}
	if (m.env.file != nil && m.env.file.ID == summary.ID) || m.env.loadingFile == summary.ID {
		return nil
	}
	m.env.loadingFile = summary.ID
	client, ws, id := m.client, m.env.workspaceID, summary.ID
	return func() tea.Msg {
		file, err := client.EnvFile(ws, id)
		return envFileMsg{id: id, file: file, err: err}
	}
}

func (m Model) uploadEnv(sources []envfile.Source, skipped int, mode string) tea.Cmd {
	client, ws := m.client, m.env.workspaceID
	return func() tea.Msg {
		var result api.EnvUploadResult
		for start := 0; start < len(sources); start += envUploadBatch {
			var batch []api.EnvUpload
			for _, s := range sources[start:min(len(sources), start+envUploadBatch)] {
				batch = append(batch, api.EnvUpload{Name: s.Name, Content: s.Content})
			}
			part, err := client.UploadEnvFiles(ws, batch, mode)
			if err != nil {
				return envUploadMsg{workspaceID: ws, err: err}
			}
			result.Files = part.Files
			result.SavedIDs = append(result.SavedIDs, part.SavedIDs...)
		}
		return envUploadMsg{workspaceID: ws, result: result, skipped: skipped}
	}
}

func (m Model) importEnvPath(target string) tea.Cmd {
	return func() tea.Msg {
		sources, skipped, err := envfile.Collect(target)
		if err != nil {
			return envUploadMsg{workspaceID: m.env.workspaceID, err: err}
		}
		return m.uploadEnv(sources, skipped, "replace")()
	}
}

func (m *Model) handleEnvMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case envListMsg:
		if m.mode != modeEnv || msg.workspaceID != m.env.workspaceID {
			return nil, true
		}
		m.env.loaded = true
		if msg.err != nil {
			_, cmd := m.handleAPIError(msg.err)
			return cmd, true
		}
		m.setEnvFiles(msg.files)
		return m.loadEnvFile(), true
	case envFileMsg:
		if m.mode != modeEnv || m.env.loadingFile != msg.id {
			return nil, true
		}
		m.env.loadingFile = ""
		if msg.err != nil {
			_, cmd := m.handleAPIError(msg.err)
			return cmd, true
		}
		file := msg.file
		if m.env.file == nil || m.env.file.ID != file.ID {
			m.env.selected = map[string]bool{}
			m.env.revealed = map[string]bool{}
			m.env.varCursor, m.env.varOffset = 0, 0
			m.env.filter = ""
		}
		m.env.file = &file
		m.clampEnvVarCursor()
		return nil, true
	case envUploadMsg:
		if m.mode != modeEnv || msg.workspaceID != m.env.workspaceID {
			return nil, true
		}
		m.env.busy = false
		if msg.err != nil {
			_, cmd := m.handleAPIError(msg.err)
			return cmd, true
		}
		if len(msg.result.SavedIDs) > 0 {
			m.env.selectAfter = msg.result.SavedIDs[0]
		}
		m.env.file = nil
		m.setEnvFiles(msg.result.Files)
		saved := len(msg.result.SavedIDs)
		m.flash = fmt.Sprintf("%s Saved %s", m.icons().Saved, plural(saved, "env file"))
		if msg.skipped > 0 {
			m.flash += fmt.Sprintf(" · skipped %d", msg.skipped)
		}
		return m.loadEnvFile(), true
	case envDeleteMsg:
		if m.mode != modeEnv || msg.workspaceID != m.env.workspaceID {
			return nil, true
		}
		m.env.busy = false
		if msg.err != nil {
			_, cmd := m.handleAPIError(msg.err)
			return cmd, true
		}
		m.flash = "Deleted " + msg.name
		m.env.file = nil
		m.env.pane = envPaneFiles
		return m.loadEnvList(), true
	}
	return nil, false
}

func (m *Model) setEnvFiles(files []api.EnvFileSummary) {
	keep := m.env.selectAfter
	if keep == "" {
		if s := m.envSelectedSummary(); s != nil {
			keep = s.ID
		}
	}
	m.env.selectAfter = ""
	m.env.files = files
	m.env.fileCursor = 0
	for i, f := range files {
		if f.ID == keep {
			m.env.fileCursor = i
		}
	}
	m.ensureEnvFileVisible()
}

func (m Model) envSelectedSummary() *api.EnvFileSummary {
	if m.env.fileCursor < 0 || m.env.fileCursor >= len(m.env.files) {
		return nil
	}
	return &m.env.files[m.env.fileCursor]
}

func (m Model) envVisibleVars() []envfile.Var {
	if m.env.file == nil {
		return nil
	}
	q := strings.ToLower(strings.TrimSpace(m.env.filter))
	if q == "" {
		return m.env.file.Vars
	}
	var out []envfile.Var
	for _, v := range m.env.file.Vars {
		if strings.Contains(strings.ToLower(v.Key), q) {
			out = append(out, v)
		}
	}
	return out
}

func (m Model) envChosenVars() ([]envfile.Var, string) {
	if m.env.file == nil {
		return nil, ""
	}
	if n := len(m.env.selected); n > 0 {
		var out []envfile.Var
		for _, v := range m.env.file.Vars {
			if m.env.selected[v.Key] {
				out = append(out, v)
			}
		}
		return out, fmt.Sprintf("%d selected", n)
	}
	visible := m.envVisibleVars()
	if strings.TrimSpace(m.env.filter) != "" {
		return visible, fmt.Sprintf("%d shown", len(visible))
	}
	return visible, fmt.Sprintf("all %d", len(visible))
}

func (m Model) envListRows() int {
	return max(1, m.paneHeight()-4)
}

func (m Model) envVarRows() int {
	return max(1, m.paneHeight()-5)
}

func (m *Model) ensureEnvFileVisible() {
	rows := m.envListRows()
	if m.env.fileCursor < m.env.fileOffset {
		m.env.fileOffset = m.env.fileCursor
	}
	if m.env.fileCursor >= m.env.fileOffset+rows {
		m.env.fileOffset = m.env.fileCursor - rows + 1
	}
}

func (m *Model) clampEnvVarCursor() {
	n := len(m.envVisibleVars())
	m.env.varCursor = max(0, min(m.env.varCursor, n-1))
	rows := m.envVarRows()
	if m.env.varCursor < m.env.varOffset {
		m.env.varOffset = m.env.varCursor
	}
	if m.env.varCursor >= m.env.varOffset+rows {
		m.env.varOffset = m.env.varCursor - rows + 1
	}
	m.env.varOffset = max(0, min(m.env.varOffset, max(0, n-rows)))
}

func (m *Model) startEnvPrompt(p envPrompt, placeholder, value string) tea.Cmd {
	m.env.prompt = p
	m.env.input.Placeholder = placeholder
	m.env.input.SetValue(value)
	m.env.input.CursorEnd()
	return m.env.input.Focus()
}

func (m *Model) endEnvPrompt() {
	m.env.prompt = envPromptNone
	m.env.input.Blur()
}

func (m *Model) handleEnvKey(msg tea.KeyMsg) tea.Cmd {
	if m.env.prompt != envPromptNone {
		return m.handleEnvPromptKey(msg)
	}
	key := msg.String()
	m.flash = ""
	m.err = nil
	switch key {
	case "ctrl+c":
		return tea.Quit
	case "?":
		m.modal = modalHelp
		return nil
	case "q":
		m.closeEnv()
		return m.schedulePreview()
	case "i":
		return m.startEnvPrompt(envPromptImport, "path to a .env file or a folder", "")
	case "p":
		clip, err := clipboard.ReadAll()
		if err != nil || len(envfile.Parse(clip)) == 0 {
			m.flash = "Copy some KEY=value lines first, then press p"
			return nil
		}
		m.env.clip = clip
		return m.startEnvPrompt(envPromptPasteName, "name, e.g. .env.production", ".env")
	case "r":
		m.env.file = nil
		return m.loadEnvList()
	case "w":
		m.openWorkspaces()
		return nil
	}
	if m.env.pane == envPaneFiles {
		return m.handleEnvFilesKey(key)
	}
	return m.handleEnvVarsKey(key)
}

func (m *Model) handleEnvFilesKey(key string) tea.Cmd {
	n := len(m.env.files)
	move := func(to int) tea.Cmd {
		if n == 0 {
			return nil
		}
		m.env.fileCursor = wrapIndex(to, n)
		m.ensureEnvFileVisible()
		return m.loadEnvFile()
	}
	switch key {
	case "esc":
		m.closeEnv()
		return m.schedulePreview()
	case "up", "k", "ctrl+p":
		return move(m.env.fileCursor - 1)
	case "down", "j", "ctrl+n":
		return move(m.env.fileCursor + 1)
	case "g", "home":
		return move(0)
	case "G", "end":
		return move(n - 1)
	case "enter", "l", "right", "tab", " ":
		if m.env.file != nil {
			m.env.pane = envPaneVars
		}
	case "x", "delete":
		if s := m.envSelectedSummary(); s != nil {
			m.env.prompt = envPromptDelete
		}
	}
	return nil
}

func (m *Model) handleEnvVarsKey(key string) tea.Cmd {
	visible := m.envVisibleVars()
	n := len(visible)
	var current *envfile.Var
	if m.env.varCursor < n {
		current = &visible[m.env.varCursor]
	}
	switch key {
	case "esc":
		if m.env.filter != "" {
			m.env.filter = ""
			m.clampEnvVarCursor()
			return nil
		}
		m.env.pane = envPaneFiles
	case "h", "left", "tab", "shift+tab":
		m.env.pane = envPaneFiles
	case "up", "k", "ctrl+p":
		if n > 0 {
			m.env.varCursor = wrapIndex(m.env.varCursor-1, n)
		}
	case "down", "j", "ctrl+n":
		if n > 0 {
			m.env.varCursor = wrapIndex(m.env.varCursor+1, n)
		}
	case "g", "home":
		m.env.varCursor = 0
	case "G", "end":
		m.env.varCursor = n - 1
	case "pgdown", "ctrl+d":
		m.env.varCursor = min(n-1, m.env.varCursor+m.envVarRows()/2)
	case "pgup", "ctrl+u":
		m.env.varCursor = max(0, m.env.varCursor-m.envVarRows()/2)
	case " ", "x":
		if current != nil {
			if m.env.selected[current.Key] {
				delete(m.env.selected, current.Key)
			} else {
				m.env.selected[current.Key] = true
			}
			if n > 0 {
				m.env.varCursor = min(n-1, m.env.varCursor+1)
			}
		}
	case "a":
		all := n > 0
		for _, v := range visible {
			all = all && m.env.selected[v.Key]
		}
		for _, v := range visible {
			if all {
				delete(m.env.selected, v.Key)
			} else {
				m.env.selected[v.Key] = true
			}
		}
	case "s", "enter":
		if current != nil && !m.env.revealAll {
			m.env.revealed[current.Key] = !m.env.revealed[current.Key]
		}
	case "S":
		m.env.revealAll = !m.env.revealAll
		m.env.revealed = map[string]bool{}
	case "y":
		if current != nil {
			m.copyEnv(current.Value, "Copied "+current.Key)
		}
	case "Y":
		if current != nil {
			m.copyEnv(envfile.Render([]envfile.Var{*current}, envfile.FormatEnv), "Copied "+current.Key+"=…")
		}
	case "c":
		if chosen, _ := m.envChosenVars(); len(chosen) > 0 {
			m.env.prompt = envPromptCopy
		}
	case "d":
		if chosen, _ := m.envChosenVars(); len(chosen) > 0 {
			m.env.prompt = envPromptDownload
		}
	case "/", "f":
		return m.startEnvPrompt(envPromptFilter, "filter keys", m.env.filter)
	}
	m.clampEnvVarCursor()
	return nil
}

func (m *Model) copyEnv(text, label string) {
	if err := copyToClipboard(text); err != nil {
		m.err = errors.New("couldn't copy to the clipboard")
		return
	}
	m.flash = m.icons().Copy + " " + label
}

func formatForKey(key string) (envfile.Format, bool) {
	switch key {
	case "1", "e":
		return envfile.FormatEnv, true
	case "2", "y":
		return envfile.FormatYAML, true
	case "3", "t":
		return envfile.FormatTXT, true
	case "4", "j":
		return envfile.FormatJSON, true
	}
	return "", false
}

func (m *Model) handleEnvPromptKey(msg tea.KeyMsg) tea.Cmd {
	key := msg.String()
	switch m.env.prompt {
	case envPromptCopy, envPromptDownload:
		if key == "esc" || key == "q" || key == "ctrl+c" {
			m.endEnvPrompt()
			return nil
		}
		format, ok := formatForKey(key)
		if !ok {
			return nil
		}
		prompt := m.env.prompt
		m.endEnvPrompt()
		chosen, scope := m.envChosenVars()
		body := envfile.Render(chosen, format)
		if prompt == envPromptCopy {
			m.copyEnv(body, fmt.Sprintf("Copied %s as %s", scope, format.Label()))
			return nil
		}
		name := envfile.DownloadName(m.env.file.Name, format)
		m.env.writePath, m.env.writeBody = name, body
		if _, err := os.Stat(name); err == nil {
			m.env.prompt = envPromptOverwrite
			return nil
		}
		m.writeEnvDownload()
		return nil
	case envPromptDelete:
		m.endEnvPrompt()
		summary := m.envSelectedSummary()
		if key != "y" || summary == nil {
			return nil
		}
		m.env.busy = true
		client, ws, id, name := m.client, m.env.workspaceID, summary.ID, summary.Name
		return func() tea.Msg {
			return envDeleteMsg{workspaceID: ws, name: name, err: client.DeleteEnvFile(ws, id)}
		}
	case envPromptOverwrite:
		m.endEnvPrompt()
		if key == "y" {
			m.writeEnvDownload()
		} else {
			m.flash = "Download cancelled"
		}
		return nil
	}

	switch key {
	case "esc", "ctrl+c":
		if m.env.prompt == envPromptFilter {
			m.env.filter = ""
			m.clampEnvVarCursor()
		}
		m.endEnvPrompt()
		return nil
	case "enter":
		value := strings.TrimSpace(m.env.input.Value())
		prompt := m.env.prompt
		m.endEnvPrompt()
		switch prompt {
		case envPromptImport:
			if value == "" {
				return nil
			}
			m.env.busy = true
			m.flash = "Uploading…"
			return m.importEnvPath(value)
		case envPromptPasteName:
			if value == "" {
				return nil
			}
			m.env.busy = true
			clip := m.env.clip
			m.env.clip = ""
			return m.uploadEnv([]envfile.Source{{Name: value, Content: clip}}, 0, "replace")
		}
		return nil
	}
	var cmd tea.Cmd
	m.env.input, cmd = m.env.input.Update(msg)
	if m.env.prompt == envPromptFilter {
		m.env.filter = m.env.input.Value()
		m.env.varCursor = 0
		m.clampEnvVarCursor()
	}
	return cmd
}

func (m *Model) writeEnvDownload() {
	path, body := m.env.writePath, m.env.writeBody
	m.env.writePath, m.env.writeBody = "", ""
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		m.err = err
		return
	}
	m.flash = m.icons().Saved + " Wrote ./" + path
}

func (m Model) envView() string {
	height := m.paneHeight()
	listWidth := m.treeWidth()
	if listWidth == 0 {
		listWidth = max(minTreeWidth, min(maxTreeWidth, m.width/3))
	}
	varsWidth := m.width - listWidth

	listStyle, varsStyle := paneFocused, paneUnfocused
	if m.env.pane == envPaneVars {
		listStyle, varsStyle = paneUnfocused, paneFocused
	}
	list := listStyle.Width(listWidth - 2).Height(height - 2).Render(m.envListView(listWidth - 4))
	vars := varsStyle.Width(varsWidth - 2).Height(height - 2).Render(m.envVarsView(varsWidth - 4))
	if m.settings.SidebarSide == "right" {
		return lipgloss.JoinHorizontal(lipgloss.Top, vars, list)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, list, vars)
}

func (m Model) envListView(width int) string {
	ws := ""
	if len(m.workspaces) > m.workspace {
		ws = m.workspaces[m.workspace].Label()
	}
	lines := []string{
		" " + accentStyle.Render("Env vault"),
		" " + faintStyle.Render(ansi.Truncate(ws, max(1, width-1), "…")),
	}
	switch {
	case !m.env.loaded:
		lines = append(lines, "", " "+m.miniLoader(loaderNote))
	case len(m.env.files) == 0:
		lines = append(lines, "",
			" "+mutedStyle.Render("No env files yet."),
			"",
			" "+keyStyle.Render("i")+mutedStyle.Render(" import file/folder"),
			" "+keyStyle.Render("p")+mutedStyle.Render(" paste from clipboard"),
		)
	default:
		end := min(len(m.env.files), m.env.fileOffset+m.envListRows())
		for i := m.env.fileOffset; i < end; i++ {
			f := m.env.files[i]
			count := fmt.Sprintf("%d", f.VarCount)
			name := ansi.Truncate(f.Name, max(1, width-lipgloss.Width(count)-3), "…")
			gap := max(1, width-1-lipgloss.Width(name)-lipgloss.Width(count))
			row := " " + name + strings.Repeat(" ", gap) + count
			switch {
			case i == m.env.fileCursor && m.env.pane == envPaneFiles:
				row = cursorStyle.Width(width).Render(row)
			case i == m.env.fileCursor:
				row = cursorDimmed.Width(width).Render(row)
			default:
				row = fileStyle.Render(" "+name) + strings.Repeat(" ", gap) + faintStyle.Render(count)
			}
			lines = append(lines, row)
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) envVarsView(width int) string {
	file := m.env.file
	if file == nil {
		body := mutedStyle.Render("Select an env file")
		if m.env.loadingFile != "" {
			body = m.miniLoader(loaderNote)
		} else if m.env.loaded && len(m.env.files) == 0 {
			body = mutedStyle.Render("Import a .env file to get started")
		}
		return lipgloss.Place(width, max(1, m.paneHeight()-2), lipgloss.Center, lipgloss.Center, body)
	}
	visible := m.envVarsWithin()
	eye := "hidden"
	if m.env.revealAll {
		eye = "revealed"
	}
	meta := fmt.Sprintf("%d vars · values %s", len(file.Vars), eye)
	if n := len(m.env.selected); n > 0 {
		meta = fmt.Sprintf("%d selected · ", n) + meta
	}
	title := titleStyle.Render(ansi.Truncate(file.Name, max(1, width-lipgloss.Width(meta)-3), "…"))
	gap := max(1, width-1-lipgloss.Width(title)-lipgloss.Width(meta))
	filterLine := faintStyle.Render(" / filter keys")
	if m.env.filter != "" || m.env.prompt == envPromptFilter {
		filterLine = " " + keyStyle.Render("/") + " " + m.env.filter
	}
	lines := []string{" " + title + strings.Repeat(" ", gap) + mutedStyle.Render(meta), filterLine, ""}

	keyWidth := 0
	for _, v := range file.Vars {
		keyWidth = max(keyWidth, lipgloss.Width(v.Key))
	}
	keyWidth = min(keyWidth, max(8, (width-6)*2/5))
	valueWidth := max(4, width-keyWidth-8)

	all := m.envVisibleVars()
	if len(all) == 0 {
		lines = append(lines, " "+mutedStyle.Render("No keys match"))
	}
	for i, v := range visible {
		idx := m.env.varOffset + i
		box := faintStyle.Render("○")
		if m.env.selected[v.Key] {
			box = successStyle.Render("●")
		}
		value := envMask
		shown := m.env.revealAll || m.env.revealed[v.Key]
		if shown {
			value = strings.NewReplacer("\r", "", "\n", "⏎").Replace(v.Value)
			if value == "" {
				value = "(empty)"
			}
		}
		keyCell := padRight(ansi.Truncate(v.Key, keyWidth, "…"), keyWidth)
		valueCell := ansi.Truncate(value, valueWidth, "…")
		if idx == m.env.varCursor && m.env.pane == envPaneVars {
			mark := "○"
			if m.env.selected[v.Key] {
				mark = "●"
			}
			lines = append(lines, cursorStyle.Width(width).Render(" "+mark+" "+keyCell+"  "+valueCell))
			continue
		}
		valueStyle := mutedStyle
		if !shown {
			valueStyle = faintStyle
		}
		lines = append(lines, " "+box+" "+fileStyle.Render(keyCell)+"  "+valueStyle.Render(valueCell))
	}
	return strings.Join(lines, "\n")
}

func (m Model) envVarsWithin() []envfile.Var {
	all := m.envVisibleVars()
	start := min(m.env.varOffset, len(all))
	return all[start:min(len(all), start+m.envVarRows())]
}

func (m Model) envPromptView() string {
	switch m.env.prompt {
	case envPromptImport:
		return keyStyle.Render("import ") + m.env.input.View() + faintStyle.Render("  ⏎ upload · esc cancel")
	case envPromptPasteName:
		return keyStyle.Render("save clipboard as ") + m.env.input.View() + faintStyle.Render("  ⏎ save · esc cancel")
	case envPromptFilter:
		return keyStyle.Render("/") + m.env.input.View() + faintStyle.Render("  ⏎ keep · esc clear")
	case envPromptCopy, envPromptDownload:
		verb := "Copy"
		if m.env.prompt == envPromptDownload {
			verb = "Download"
		}
		_, scope := m.envChosenVars()
		parts := []string{titleStyle.Render(verb + " " + scope + " as")}
		for i, f := range envfile.Formats {
			parts = append(parts, keyStyle.Render(fmt.Sprint(i+1))+" "+f.Label())
		}
		return strings.Join(parts, "  ") + faintStyle.Render("  esc cancel")
	case envPromptDelete:
		name := ""
		if s := m.envSelectedSummary(); s != nil {
			name = s.Name
		}
		return errorStyle.Render("Delete "+name+" for everyone in this workspace?") + "  " + keyStyle.Render("y") + " delete  " + keyStyle.Render("any key") + " cancel"
	case envPromptOverwrite:
		return errorStyle.Render("./"+m.env.writePath+" exists. Overwrite?") + "  " + keyStyle.Render("y") + " overwrite  " + keyStyle.Render("any key") + " cancel"
	}
	return ""
}

func (m Model) envHints() string {
	if m.env.busy {
		return m.miniLoader(loaderNote)
	}
	if m.env.pane == envPaneFiles {
		return strings.Join([]string{
			hint("⏎", "open", m.env.file != nil),
			hint("i", "import", true),
			hint("p", "paste", true),
			hint("x", "delete", len(m.env.files) > 0),
			hint("r", "refresh", true),
			hint("w", "workspace", len(m.workspaces) > 1),
			hint("?", "help", true),
			hint("q", "back to notes", true),
		}, "  ")
	}
	return strings.Join([]string{
		hint("s", "show", true),
		hint("S", "show all", true),
		hint("y", "copy value", true),
		hint("space", "select", true),
		hint("c", "copy as", true),
		hint("d", "download", true),
		hint("/", "filter", true),
		hint("esc", "files", true),
	}, "  ")
}
