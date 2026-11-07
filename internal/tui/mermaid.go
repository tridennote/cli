package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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

func mermaidSources(note api.Note) []string {
	var sources []string
	for _, b := range note.Blocks {
		if b.Type == "mermaid" && strings.TrimSpace(b.Content) != "" {
			sources = append(sources, b.Content)
		}
	}
	return sources
}

func (m *Model) requestDiagrams(id string) tea.Cmd {
	note, ok := m.notes[id]
	if !ok {
		m.pending = &pendingOpen{id: id, action: pendingDiagram}
		return m.open(id)
	}
	sources := mermaidSources(note)
	if len(sources) == 0 {
		m.flash = "No mermaid diagrams in this note"
		return nil
	}
	m.renderingDiagrams = true
	return func() tea.Msg {
		images, err := diagram.RenderImages(sources)
		return diagramImagesMsg{images: images, err: err}
	}
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
		m.flash = fmt.Sprintf("%s Opened %d diagram(s) in %s", m.icons().Diagram, len(msg.images), viewer.Name)
		return nil, true
	case diagramViewerDoneMsg:
		if msg.err != nil {
			m.flash = "Image viewer exited: " + msg.err.Error()
		}
		return nil, true
	}
	return nil, false
}
