package tui

import (
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/tridennote/cli/internal/kanban"
)

const maxSuggestions = 6

type suggestion struct {
	insert string
	label  string
	detail string
}

type workspaceMetaMsg struct {
	workspaceID string
	people      []kanban.Person
	tags        []string
}

func (m *Model) loadWorkspaceMeta() tea.Cmd {
	if len(m.workspaces) == 0 {
		return nil
	}
	id := m.workspaces[m.workspace].ID
	if m.metaWorkspace == id {
		return nil
	}
	m.metaWorkspace = id
	m.people, m.tags = nil, nil
	client := m.client
	return func() tea.Msg {
		members, _ := client.Members(id)
		tags, _ := client.Tags(id)
		return workspaceMetaMsg{workspaceID: id, people: kanban.People(members), tags: tags}
	}
}

func (m *Model) handleMetaMsg(msg tea.Msg) bool {
	meta, ok := msg.(workspaceMetaMsg)
	if !ok {
		return false
	}
	if len(m.workspaces) > 0 && m.workspaces[m.workspace].ID == meta.workspaceID {
		m.people = meta.people
		m.tags = meta.tags
	}
	return true
}

func (m Model) composerToken() (int, string) {
	value := []rune(m.boardInput.Value())
	pos := min(m.boardInput.Position(), len(value))
	start := pos
	for start > 0 && value[start-1] != ' ' {
		start--
	}
	return start, string(value[start:pos])
}

func dateKey(t time.Time) string {
	return t.Format("2006-01-02")
}

func dueChoices(now time.Time) []suggestion {
	daysToMonday := (8 - int(now.Weekday())) % 7
	if daysToMonday == 0 {
		daysToMonday = 7
	}
	choices := []struct {
		label string
		at    time.Time
	}{
		{"today", now},
		{"tomorrow", now.AddDate(0, 0, 1)},
		{"next monday", now.AddDate(0, 0, daysToMonday)},
		{"in a week", now.AddDate(0, 0, 7)},
		{"in two weeks", now.AddDate(0, 0, 14)},
	}
	out := make([]suggestion, 0, len(choices))
	for _, c := range choices {
		key := dateKey(c.at)
		out = append(out, suggestion{insert: "due:" + key, label: "due:" + key, detail: c.label})
	}
	return out
}

func (m Model) boardTagSuggestions() []string {
	seen := map[string]bool{}
	var out []string
	add := func(tag string) {
		if key := strings.ToLower(tag); !seen[key] {
			seen[key] = true
			out = append(out, tag)
		}
	}
	for _, col := range m.board.Columns {
		for _, card := range col.Cards {
			for _, tag := range card.Tags {
				add(tag)
			}
		}
	}
	for _, tag := range m.tags {
		add(tag)
	}
	return out
}

func (m Model) suggestionsFor(token string) []suggestion {
	lower := strings.ToLower(token)
	var out []suggestion
	switch {
	case strings.HasPrefix(token, "@"):
		query := lower[1:]
		for _, p := range m.people {
			if query == "" || strings.Contains(p.Handle, query) || strings.Contains(strings.ToLower(p.Name), query) || strings.Contains(strings.ToLower(p.Email), query) {
				out = append(out, suggestion{insert: "@" + p.Handle, label: "@" + p.Handle, detail: p.Name})
			}
		}
	case strings.HasPrefix(token, "#"):
		query := lower[1:]
		for _, tag := range m.boardTagSuggestions() {
			if t := strings.ToLower(tag); strings.Contains(t, query) && t != query {
				out = append(out, suggestion{insert: "#" + tag, label: "#" + tag})
			}
		}
	case strings.HasPrefix(token, "!"):
		query := lower[1:]
		for _, p := range kanban.Priorities {
			if strings.HasPrefix(p, query) && p != query {
				out = append(out, suggestion{insert: "!" + p, label: "!" + p})
			}
		}
	case strings.HasPrefix(lower, "due:") || (len(lower) >= 2 && strings.HasPrefix("due:", lower)):
		for _, c := range dueChoices(time.Now()) {
			if strings.HasPrefix(c.insert, lower) && c.insert != lower {
				out = append(out, c)
			}
		}
	}
	if len(out) > maxSuggestions {
		out = out[:maxSuggestions]
	}
	return out
}

func (m Model) visibleSuggestions() []suggestion {
	if m.suggestHidden || !m.composing() {
		return nil
	}
	_, token := m.composerToken()
	return m.suggestionsFor(token)
}

func (m Model) composing() bool {
	return m.boardPrompt == promptAddCard || m.boardPrompt == promptEditCard || (m.mode == modeCardDraft && m.draftTitleFocus)
}

func (m *Model) acceptSuggestion(s suggestion) {
	value := []rune(m.boardInput.Value())
	start, token := m.composerToken()
	end := start + len([]rune(token))
	rest := strings.TrimLeft(string(value[end:]), " ")
	next := string(value[:start]) + s.insert + " "
	m.boardInput.SetValue(next + rest)
	m.boardInput.SetCursor(len([]rune(next)))
	m.suggestIndex = 0
}

func (m *Model) startComposer(kind boardPrompt, value string) tea.Cmd {
	m.suggestIndex = 0
	m.suggestHidden = false
	m.boardInput.Width = max(10, m.composerWidth()-6)
	return tea.Batch(m.startBoardPrompt(kind, value, "Card title"), m.loadWorkspaceMeta())
}

func (m Model) composerWidth() int {
	return max(36, min(72, m.width-6))
}

func (m *Model) handleComposerKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	suggestions := m.visibleSuggestions()
	key := msg.String()
	if len(suggestions) > 0 {
		m.suggestIndex = min(m.suggestIndex, len(suggestions)-1)
		switch key {
		case "up", "ctrl+p":
			m.suggestIndex = (m.suggestIndex - 1 + len(suggestions)) % len(suggestions)
			return nil, true
		case "down", "ctrl+n":
			m.suggestIndex = (m.suggestIndex + 1) % len(suggestions)
			return nil, true
		case "tab", "enter":
			m.acceptSuggestion(suggestions[m.suggestIndex])
			return nil, true
		case "esc":
			m.suggestHidden = true
			return nil, true
		}
	}
	switch key {
	case "tab":
		return m.composeInFullEditor(), true
	case "esc", "ctrl+c", "enter":
		return nil, false
	}
	before := m.boardInput.Value()
	var cmd tea.Cmd
	m.boardInput, cmd = m.boardInput.Update(msg)
	if m.boardInput.Value() != before {
		m.suggestHidden = false
		m.suggestIndex = 0
	}
	return cmd, true
}

func (m Model) composerView() string {
	width := m.composerWidth()
	inner := width - 4
	column := ""
	if m.boardCol >= 0 && m.boardCol < len(m.board.Columns) {
		column = m.board.Columns[m.boardCol].Name
	}
	title := "New card"
	if m.boardPrompt == promptEditCard {
		title = "Edit card"
		if card, ok := m.currentCard(); ok && card.Number > 0 {
			title += " #" + strconv.Itoa(card.Number)
		}
	}
	header := accentStyle.Render(m.withIcon(m.icons().Board, title)) + faintStyle.Render(" · "+column)
	lines := []string{ansi.Truncate(header, inner, "…"), "", accentStyle.Render("› ") + m.highlightedInput(m.boardInput, inner-2)}

	suggestions := m.visibleSuggestions()
	if len(suggestions) > 0 {
		lines = append(lines, "")
		lines = append(lines, m.suggestionRows(suggestions, "", inner)...)
	}

	legend := []string{m.legend()}
	keys := keyStyle.Render("⏎") + faintStyle.Render(" save  ") +
		keyStyle.Render("tab") + faintStyle.Render(" full editor  ") +
		keyStyle.Render("esc") + faintStyle.Render(" cancel")
	if len(suggestions) > 0 {
		keys = keyStyle.Render("↑↓") + faintStyle.Render(" choose  ") +
			keyStyle.Render("⏎ tab") + faintStyle.Render(" insert  ") +
			keyStyle.Render("esc") + faintStyle.Render(" hide")
	}
	lines = append(lines, "", strings.Join(legend, "  "), keys)
	return modalStyle.Width(width).Render(strings.Join(lines, "\n"))
}

func (m Model) suggestionRows(suggestions []suggestion, indent string, width int) []string {
	rows := make([]string, 0, len(suggestions))
	for i, s := range suggestions {
		pointer := indent + "  "
		label := s.label
		if style, ok := m.tokenStyle(s.insert); ok {
			label = style.Render(s.label)
		}
		if i == m.suggestIndex {
			pointer = indent + accentStyle.Render("› ")
			label = lipgloss.NewStyle().Bold(true).Render(label)
		} else if !strings.HasPrefix(s.insert, "#") {
			label = lipgloss.NewStyle().Faint(true).Render(label)
		}
		row := pointer + label
		if s.detail != "" {
			row += "  " + faintStyle.Render(s.detail)
		}
		rows = append(rows, ansi.Truncate(row, width, "…"))
	}
	return rows
}

func (m Model) legend() string {
	return priorityStyles["high"].Render("!priority") + "  " +
		renderTag("tag") + "  " +
		assigneeStyle.Render("@person") + "  " +
		dueStyle.Render("due:") + faintStyle.Render("date")
}
