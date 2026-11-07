package tui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"

	"github.com/tridennote/cli/internal/kanban"
)

var (
	tagHues      = []float64{25, 55, 85, 140, 175, 210, 250, 285, 320, 350}
	tagTokenRe   = regexp.MustCompile(`^#[A-Za-z0-9_][\w/-]*$`)
	dateTokenRe  = regexp.MustCompile(`^due:\d{4}-\d{2}-\d{2}$`)
	dueStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#5eead4"))
	pendingStyle = lipgloss.NewStyle().Foreground(muted).Italic(true)
	cursorCell   = lipgloss.NewStyle().Reverse(true)
	updateStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#fbbf24")).Bold(true)
)

func tagHue(tag string) float64 {
	var hash uint32
	for _, r := range strings.ToLower(tag) {
		hash = hash*31 + uint32(r)
	}
	return tagHues[hash%uint32(len(tagHues))]
}

func tagChip(tag string) lipgloss.Style {
	hue := tagHue(tag)
	return lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorful.OkLch(0.86, 0.1, hue).Clamped().Hex())).
		Background(lipgloss.Color(colorful.OkLch(0.32, 0.07, hue).Clamped().Hex()))
}

func renderTag(tag string) string {
	return tagChip(tag).Render("#" + tag)
}

func (m Model) tokenStyle(token string) (lipgloss.Style, bool) {
	switch {
	case strings.HasPrefix(token, "#") && tagTokenRe.MatchString(token):
		return tagChip(token[1:]), true
	case strings.HasPrefix(token, "!"):
		if style, ok := priorityStyles[strings.ToLower(token[1:])]; ok {
			return style, true
		}
		return pendingStyle, true
	case strings.HasPrefix(token, "@") && len(token) > 1:
		if _, known := kanban.FindPerson(m.people, token[1:]); known {
			return assigneeStyle.Bold(true), true
		}
		return assigneeStyle, true
	case strings.HasPrefix(strings.ToLower(token), "due:"):
		if dateTokenRe.MatchString(strings.ToLower(token)) {
			return dueStyle, true
		}
		return pendingStyle, true
	}
	return lipgloss.Style{}, false
}

func (m Model) highlightedInput(input textinput.Model, width int) string {
	value := []rune(input.Value())
	focused := input.Focused()
	if len(value) == 0 {
		placeholder := faintStyle.Render(input.Placeholder)
		if focused {
			return cursorCell.Render(" ") + placeholder
		}
		return placeholder
	}

	styles := make([]*lipgloss.Style, len(value))
	for start := 0; start < len(value); {
		if value[start] == ' ' {
			start++
			continue
		}
		end := start
		for end < len(value) && value[end] != ' ' {
			end++
		}
		if style, ok := m.tokenStyle(string(value[start:end])); ok {
			for i := start; i < end; i++ {
				styles[i] = &style
			}
		}
		start = end
	}

	pos := min(input.Position(), len(value))
	total := len(value) + 1
	from := 0
	if width > 1 && total > width {
		from = max(0, min(pos-width+1, total-width))
	}
	to := len(value)
	if width > 1 {
		to = min(len(value), from+width)
	}

	plain := lipgloss.NewStyle().Foreground(text)
	var b strings.Builder
	for i := from; i < to; i++ {
		cell := string(value[i])
		style := plain
		if styles[i] != nil {
			style = *styles[i]
		}
		if focused && i == pos {
			style = style.Reverse(true)
		}
		b.WriteString(style.Render(cell))
	}
	if focused && pos >= len(value) {
		b.WriteString(cursorCell.Render(" "))
	}
	return b.String()
}
