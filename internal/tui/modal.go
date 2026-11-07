package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/tridennote/cli/internal/settings"
)

type modal int

const (
	modalNone modal = iota
	modalSettings
	modalHelp
	modalActions
	modalAddToBoard
	modalDiagrams
	modalWorkspaces
)

type settingRow struct {
	section string
	value   string
	label   string
}

func settingRows() []settingRow {
	var rows []settingRow
	for _, e := range editorCatalog {
		rows = append(rows, settingRow{section: "editor", value: e.ID, label: e.Name})
	}
	rows = append(rows,
		settingRow{section: "sidebar", label: "Sidebar"},
		settingRow{section: "icons", label: "Icons"},
	)
	return rows
}

var sectionTitles = map[string]string{
	"editor":  "EDITOR",
	"sidebar": "APPEARANCE",
	"icons":   "APPEARANCE",
}

type choice struct{ value, label string }

var cycleChoices = map[string][]choice{
	"sidebar": {{settings.SideLeft, "Left"}, {settings.SideRight, "Right"}},
	"icons":   {{settings.IconsNerd, "Nerd Font"}, {settings.IconsMinimal, "Minimal"}},
}

func (m Model) currentChoice(section string) string {
	if section == "sidebar" {
		return m.settings.SidebarSide
	}
	return m.settings.Icons
}

func (m *Model) cycleSetting(section string, delta int) {
	choices := cycleChoices[section]
	idx := 0
	for i, c := range choices {
		if c.value == m.currentChoice(section) {
			idx = i
		}
	}
	next := choices[(idx+delta+len(choices))%len(choices)].value
	if section == "sidebar" {
		m.settings.SidebarSide = next
	} else {
		m.settings.Icons = next
	}
	m.persistSettings()
}

func (m Model) choiceLabel(section string) string {
	for _, c := range cycleChoices[section] {
		if c.value == m.currentChoice(section) {
			return c.label
		}
	}
	return ""
}

func (m Model) settingSelected(row settingRow) bool {
	return row.section == "editor" && m.settings.Editor == row.value
}

func (m *Model) openSettings() {
	m.modal = modalSettings
	m.settingsCursor = 0
	m.editorAvailability = map[string]bool{}
	for i, row := range settingRows() {
		if row.section == "editor" {
			e, _ := findEditor(row.value)
			m.editorAvailability[row.value] = e.available()
		}
		if m.settingSelected(row) && row.section == "editor" {
			m.settingsCursor = i
		}
	}
}

func (m *Model) applySetting(row settingRow) {
	if row.section != "editor" {
		m.cycleSetting(row.section, 1)
		return
	}
	if !m.editorAvailability[row.value] {
		m.flash = row.label + " isn't installed (or not on your PATH)"
		return
	}
	m.settings.Editor = row.value
	m.persistSettings()
}

func (m *Model) persistSettings() {
	if err := settings.Save(m.settings); err != nil {
		m.flash = "Could not save settings: " + err.Error()
		return
	}
	m.flash = m.icons().Saved + " Saved"
	m.layout()
}

func (m *Model) handleModalKey(msg tea.KeyMsg) tea.Cmd {
	switch m.modal {
	case modalActions:
		return m.handleActionsKey(msg)
	case modalAddToBoard:
		return m.handleAddToBoardKey(msg)
	case modalDiagrams:
		return m.handleDiagramsKey(msg)
	case modalWorkspaces:
		return m.handleWorkspacesKey(msg)
	}
	key := msg.String()
	if m.modal == modalHelp {
		switch key {
		case "esc", "q", "?", "enter":
			m.modal = modalNone
		}
		return nil
	}
	rows := settingRows()
	switch key {
	case "esc", "q", ",":
		m.modal = modalNone
	case "up", "k":
		m.settingsCursor = (m.settingsCursor - 1 + len(rows)) % len(rows)
	case "down", "j":
		m.settingsCursor = (m.settingsCursor + 1) % len(rows)
	case "tab":
		current := rows[m.settingsCursor].section
		for i := 1; i <= len(rows); i++ {
			next := (m.settingsCursor + i) % len(rows)
			if rows[next].section != current {
				m.settingsCursor = next
				break
			}
		}
	case "left", "h":
		if row := rows[m.settingsCursor]; row.section != "editor" {
			m.cycleSetting(row.section, -1)
		}
	case "enter", " ", "right", "l":
		m.applySetting(rows[m.settingsCursor])
	}
	return nil
}

var modalStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(violet).
	Padding(1, 2)

func (m Model) settingsView() string {
	ic := m.icons()
	width := 44
	lines := []string{accentStyle.Render(m.withIcon(ic.Settings, "Settings")), ""}
	section := ""
	for i, row := range settingRows() {
		if title := sectionTitles[row.section]; title != section {
			if section != "" {
				lines = append(lines, "")
			}
			section = title
			lines = append(lines, faintStyle.Render(section))
		}
		pointer := "  "
		if i == m.settingsCursor {
			pointer = accentStyle.Render("› ")
		}
		if row.section != "editor" {
			label := mutedStyle.Render(row.label)
			if i == m.settingsCursor {
				label = titleStyle.Render(row.label)
			}
			value := faintStyle.Render("◂ ") + successStyle.Render(m.choiceLabel(row.section)) + faintStyle.Render(" ▸")
			left := pointer + label
			gap := max(1, width-lipgloss.Width(left)-lipgloss.Width(value))
			lines = append(lines, left+strings.Repeat(" ", gap)+value)
			continue
		}
		radio := faintStyle.Render("○")
		if m.settingSelected(row) {
			radio = successStyle.Render("●")
		}
		label := row.label
		note := ""
		if row.section == "editor" {
			e, _ := findEditor(row.value)
			label = m.editorIcon(e) + " " + row.label
			switch {
			case row.value == builtinEditor:
				note = faintStyle.Render("in terminal")
			case !m.editorAvailability[row.value]:
				note = faintStyle.Render("not found")
				label = faintStyle.Render(ansi.Strip(label))
			case e.Terminal:
				note = mutedStyle.Render("terminal")
			default:
				note = mutedStyle.Render("app")
			}
		}
		left := pointer + radio + " " + label
		if i == m.settingsCursor {
			left = pointer + radio + " " + titleStyle.Render(ansi.Strip(label))
		}
		gap := max(1, width-lipgloss.Width(left)-lipgloss.Width(note))
		lines = append(lines, left+strings.Repeat(" ", gap)+note)
	}
	lines = append(lines, "", faintStyle.Render("↑↓ move · ⏎ select · ←→ change · esc close"))
	return modalStyle.Width(width + 4).Render(strings.Join(lines, "\n"))
}

var helpSections = []struct {
	title string
	keys  [][2]string
}{
	{"NAVIGATE", [][2]string{{"↑ ↓  j k", "move (wraps around)"}, {"← →", "switch pane (wraps)"}, {"l h", "expand · collapse"}, {"⏎ space", "open note · board · folder"}, {"g G", "top · bottom"}, {"tab", "switch pane"}}},
	{"NOTES", [][2]string{{"e", "edit (preferred editor)"}, {"i", "edit in built-in editor"}, {"n", "new note"}, {"N", "new kanban board"}, {"y", "copy note as markdown"}, {"m", "open diagrams (image · browser)"}, {"o", "open in browser"}, {"r", "refresh"}}},
	{"VIEW", [][2]string{{"b", "toggle sidebar"}, {"v", "board ⇄ markdown"}, {"pgup pgdn", "scroll note"}, {"w", "switch workspace"}}},
	{"APP", [][2]string{{"ctrl+k", "actions (add to board…)"}, {",", "settings"}, {"?", "this help"}, {"L", "log out"}, {"q", "quit"}}},
	{"BOARD · MOVE", [][2]string{{"h l  ← →", "previous · next column"}, {"j k  ↑ ↓", "previous · next card"}, {"g G", "first · last card"}, {"H L  ⇧← ⇧→", "move card to column"}, {"J K  ⇧↑ ⇧↓", "move card down · up"}}},
	{"BOARD · EDIT", [][2]string{{"x space", "toggle done"}, {"a n", "add card (tab: full editor)"}, {"e ⏎", "edit card line"}, {"o", "open description in editor"}, {"E", "description in code editor"}, {"d", "delete card (d twice)"}, {"c", "add column after this"}, {"v", "show as markdown"}, {"ctrl+k", "actions"}, {"esc q", "leave board"}}},
}

func (m Model) helpColumn(sections []int) string {
	var lines []string
	for n, idx := range sections {
		section := helpSections[idx]
		if n > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, faintStyle.Render(section.title))
		for _, k := range section.keys {
			lines = append(lines, keyStyle.Render(padRight(k[0], 12))+mutedStyle.Render(k[1]))
		}
	}
	return strings.Join(lines, "\n")
}

func (m Model) helpView() string {
	title := accentStyle.Render(m.withIcon(m.icons().Help, "Keyboard shortcuts"))
	left, right := []int{0, 1}, []int{2, 3}
	if m.mode == modeBoard {
		title = accentStyle.Render(m.withIcon(m.icons().Board, "Board shortcuts"))
		left, right = []int{4}, []int{5}
	}
	columns := lipgloss.JoinHorizontal(lipgloss.Top,
		m.helpColumn(left),
		"    ",
		m.helpColumn(right),
	)
	return modalStyle.Render(lipgloss.JoinVertical(lipgloss.Left, title, "", columns, "", faintStyle.Render("esc close")))
}

func padRight(s string, width int) string {
	return s + strings.Repeat(" ", max(0, width-lipgloss.Width(s)))
}

func overlay(base, top string, width, height int) string {
	baseLines := strings.Split(base, "\n")
	for len(baseLines) < height {
		baseLines = append(baseLines, "")
	}
	topLines := strings.Split(top, "\n")
	topWidth := lipgloss.Width(top)
	x := max(0, (width-topWidth)/2)
	y := max(0, (height-len(topLines))/2)
	for i, line := range topLines {
		row := y + i
		if row >= len(baseLines) {
			break
		}
		under := baseLines[row]
		left := ansi.Truncate(under, x, "")
		left += strings.Repeat(" ", max(0, x-lipgloss.Width(left)))
		right := ansi.TruncateLeft(under, x+lipgloss.Width(line), "")
		baseLines[row] = left + line + right
	}
	return strings.Join(baseLines, "\n")
}
