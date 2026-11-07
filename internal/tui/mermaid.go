package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/tridennote/cli/internal/api"
	"github.com/tridennote/cli/internal/diagram"
	"github.com/tridennote/cli/internal/markdown"
)

var (
	diagramStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#b4b9ff"))
	diagramCodeStyle = lipgloss.NewStyle().Foreground(muted)
)

type diagramImagesMsg struct {
	images []string
	err    error
}

type diagramViewerDoneMsg struct{ err error }

func renderMarkdown(source string, width int) string {
	if r := rendererFor(width); r != nil {
		if out, err := r.Render(source); err == nil {
			return out
		}
	}
	return source
}

func (m Model) renderNote(note api.Note, width int) string {
	var parts []string
	var group []api.Block
	header := "# " + note.Title + "\n\n"
	flush := func() {
		source := header + markdown.FromBlocks(group)
		header = ""
		parts = append(parts, renderMarkdown(source, width))
		group = nil
	}
	for _, block := range note.Blocks {
		if block.Type != "mermaid" {
			group = append(group, block)
			continue
		}
		if header != "" || len(group) > 0 {
			flush()
		}
		parts = append(parts, m.diagramBlock(block.Content, width))
	}
	if header != "" || len(group) > 0 {
		flush()
	}
	return strings.Join(parts, "")
}

func (m Model) diagramBlock(source string, width int) string {
	const indent = "  "
	drawn, err := diagram.RenderText(source, max(20, width-len(indent)*2))
	label := accentStyle.Render(indent+m.withIcon(m.icons().Diagram, "mermaid · "+drawn.Kind)) +
		faintStyle.Render("   m open as image")

	var body []string
	if err != nil {
		for _, line := range strings.Split(strings.TrimRight(source, "\n"), "\n") {
			body = append(body, indent+diagramCodeStyle.Render(line))
		}
		body = append(body, "", faintStyle.Render(indent+"Can't draw "+drawn.Kind+" diagrams as text, press m to see it rendered"))
	} else {
		for _, line := range drawn.Lines {
			body = append(body, indent+diagramStyle.Render(line))
		}
		if drawn.Truncated {
			body = append(body, "", faintStyle.Render(indent+"Clipped to fit, press b for more room or m for the full image"))
		}
	}
	return "\n" + label + "\n\n" + strings.Join(body, "\n") + "\n\n"
}

type diagramEntry struct {
	label  string
	source string
}

func noteDiagrams(note api.Note) []diagramEntry {
	var out []diagramEntry
	heading := ""
	for _, b := range note.Blocks {
		if len(b.Type) == 2 && b.Type[0] == 'h' {
			heading = strings.TrimSpace(b.Content)
			continue
		}
		if b.Type != "mermaid" || strings.TrimSpace(b.Content) == "" {
			continue
		}
		label := diagram.Kind(b.Content)
		if heading != "" {
			label = heading + " · " + label
		}
		out = append(out, diagramEntry{label: label, source: b.Content})
	}
	return out
}

func (m *Model) requestDiagrams(id string) tea.Cmd {
	note, ok := m.notes[id]
	if !ok {
		m.pending = &pendingOpen{id: id, action: pendingDiagram}
		return m.open(id)
	}
	entries := noteDiagrams(note)
	if len(entries) == 0 {
		m.flash = "No mermaid diagrams in this note"
		return nil
	}
	m.modal = modalDiagrams
	m.diagramNoteID = id
	m.diagrams = entries
	m.diagramCursor = 0
	m.diagramPicked = make([]bool, len(entries))
	return nil
}

// chosenDiagrams returns the marked diagrams, or the one under the cursor when none are marked.
func (m Model) chosenDiagrams() []diagramEntry {
	var out []diagramEntry
	for i, picked := range m.diagramPicked {
		if picked {
			out = append(out, m.diagrams[i])
		}
	}
	if len(out) == 0 && m.diagramCursor < len(m.diagrams) {
		out = append(out, m.diagrams[m.diagramCursor])
	}
	return out
}

func (m Model) pickedCount() int {
	n := 0
	for _, picked := range m.diagramPicked {
		if picked {
			n++
		}
	}
	return n
}

func (m *Model) handleDiagramsKey(msg tea.KeyMsg) tea.Cmd {
	count := len(m.diagrams)
	switch msg.String() {
	case "esc", "q", "ctrl+c":
		m.modal = modalNone
	case "up", "k", "shift+tab":
		m.diagramCursor = (m.diagramCursor - 1 + count) % count
	case "down", "j", "tab":
		m.diagramCursor = (m.diagramCursor + 1) % count
	case "g":
		m.diagramCursor = 0
	case "G":
		m.diagramCursor = count - 1
	case " ", "x":
		m.diagramPicked[m.diagramCursor] = !m.diagramPicked[m.diagramCursor]
		m.diagramCursor = min(m.diagramCursor+1, count-1)
	case "a":
		all := m.pickedCount() < count
		for i := range m.diagramPicked {
			m.diagramPicked[i] = all
		}
	case "enter", "m":
		chosen := m.chosenDiagrams()
		m.modal = modalNone
		return m.openDiagramImages(chosen)
	case "o":
		chosen := m.chosenDiagrams()
		m.modal = modalNone
		return m.openDiagramsInBrowser(chosen)
	}
	return nil
}

func (m *Model) openDiagramImages(entries []diagramEntry) tea.Cmd {
	sources := make([]string, len(entries))
	for i, e := range entries {
		sources[i] = e.source
	}
	m.renderingDiagrams = true
	return func() tea.Msg {
		images, err := diagram.RenderImages(sources)
		return diagramImagesMsg{images: images, err: err}
	}
}

func (m *Model) openDiagramsInBrowser(entries []diagramEntry) tea.Cmd {
	page := diagram.Page{Title: m.nodeName(m.diagramNoteID)}
	for _, e := range entries {
		page.Diagrams = append(page.Diagrams, diagram.PageDiagram{Label: e.label, Source: e.source})
	}
	path, err := diagram.WriteHTML(page)
	if err == nil {
		err = openBrowser(path)
	}
	if err != nil {
		m.flash = "Could not open the browser: " + err.Error()
		return nil
	}
	m.flash = fmt.Sprintf("%s Opened %s in the browser", m.icons().Diagram, plural(len(entries), "diagram"))
	return nil
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

func (m Model) diagramsView() string {
	width := max(40, min(64, m.width-6))
	inner := width - 4
	lines := []string{
		accentStyle.Render(m.withIcon(m.icons().Diagram, "Open diagrams")),
		faintStyle.Render(ansi.Truncate(m.nodeName(m.diagramNoteID)+" · "+plural(len(m.diagrams), "diagram"), inner, "…")),
		"",
	}
	for i, d := range m.diagrams {
		pointer, label := "  ", mutedStyle.Render(d.label)
		if i == m.diagramCursor {
			pointer, label = accentStyle.Render("› "), titleStyle.Render(d.label)
		}
		box := faintStyle.Render("○")
		if m.diagramPicked[i] {
			box = successStyle.Render("●")
		}
		row := pointer + box + " " + faintStyle.Render(fmt.Sprintf("%d ", i+1)) + label
		lines = append(lines, ansi.Truncate(row, inner, "…"))
	}
	target := "this one"
	if n := m.pickedCount(); n > 0 {
		target = plural(n, "marked diagram")
	}
	lines = append(lines,
		"",
		mutedStyle.Render("Opens "+target),
		faintStyle.Render("↑↓ move · space mark · a all"),
		faintStyle.Render("⏎ image · o browser · esc close"),
	)
	return modalStyle.Width(width).Render(strings.Join(lines, "\n"))
}

func (m *Model) handleDiagramMsg(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case diagramImagesMsg:
		m.renderingDiagrams = false
		if msg.err != nil {
			m.flash = msg.err.Error()
			return nil, true
		}
		viewer, err := diagram.ImageViewer(msg.images)
		if err != nil {
			m.flash = err.Error()
			return nil, true
		}
		if viewer.Terminal {
			return tea.ExecProcess(viewer.Command, func(err error) tea.Msg { return diagramViewerDoneMsg{err: err} }), true
		}
		if err := viewer.Command.Start(); err != nil {
			m.flash = "Could not open " + viewer.Name + ": " + err.Error()
			return nil, true
		}
		m.flash = fmt.Sprintf("%s Opened %s in %s", m.icons().Diagram, plural(len(msg.images), "diagram"), viewer.Name)
		return nil, true
	case diagramViewerDoneMsg:
		if msg.err != nil {
			m.flash = "Image viewer exited: " + msg.err.Error()
		}
		return nil, true
	}
	return nil, false
}
