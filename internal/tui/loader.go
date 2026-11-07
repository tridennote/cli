package tui

import (
	"math"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/lucasb-eyer/go-colorful"
)

const (
	frameInterval   = 70 * time.Millisecond
	framesPerPhrase = 26
	typingSpeed     = 2
)

type animTickMsg struct{}

type loaderKind int

const (
	loaderBoot loaderKind = iota
	loaderVault
	loaderCode
	loaderAwaitAuth
	loaderNote
	loaderDiagram
	loaderWorkspace
)

var loaderPhrases = map[loaderKind][]string{
	loaderBoot: {
		"Sharpening the pencils",
		"Asking Neon who you are",
		"Checking your name on the list",
		"Warming up the ink",
		"Flipping to the first page",
	},
	loaderVault: {
		"Stacking your notes",
		"Herding stray notes",
		"Sorting pages by vibes",
		"Dusting off the shelves",
		"Taking your notes up a level",
	},
	loaderCode: {
		"Minting a secret handshake",
		"Rolling dice for your code",
		"Forging a one-time key",
	},
	loaderAwaitAuth: {
		"Waiting for the browser to nod",
		"Listening for the green light",
		"Holding the door open",
		"Staring at the browser, politely",
	},
	loaderNote: {
		"Turning to the page",
		"Unfolding the note",
		"Fetching the words",
	},
	loaderWorkspace: {
		"Walking over to the other desk",
		"Unlocking the next cabinet",
		"Swapping notebooks",
		"Rearranging the shelves",
	},
	loaderDiagram: {
		"Asking the mermaid to pose",
		"Drawing boxes and arrows",
		"Untangling the arrows",
	},
}

var (
	gradientStops = []colorful.Color{
		mustHex("#7c85f5"),
		mustHex("#c084fc"),
		mustHex("#6ee7b7"),
	}
	trailColor = mustHex("#3a3a4a")
	headColor  = mustHex("#ffffff")
)

func mustHex(h string) colorful.Color {
	c, err := colorful.Hex(h)
	if err != nil {
		panic(err)
	}
	return c
}

func gradientAt(t float64) colorful.Color {
	t = math.Mod(math.Mod(t, 1)+1, 1) * float64(len(gradientStops))
	i := int(t) % len(gradientStops)
	next := (i + 1) % len(gradientStops)
	return gradientStops[i].BlendHcl(gradientStops[next], t-float64(int(t))).Clamped()
}

func colored(c colorful.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(c.Hex()))
}

func animTick() tea.Cmd {
	return tea.Tick(frameInterval, func(time.Time) tea.Msg { return animTickMsg{} })
}

func (m Model) noteLoading() bool {
	if m.state != stateBrowse || m.mode == modeEdit || m.openID == "" {
		return false
	}
	_, ok := m.notes[m.openID]
	return !ok
}

func (m Model) needsAnimation() bool {
	switch m.state {
	case stateBoot, stateLoading:
		return m.err == nil
	case stateLogin:
		return m.err == nil && (m.device == nil || m.browserOpened)
	case stateBrowse:
		return m.loadingWorkspace != "" || m.noteLoading() || m.csvLoading() || m.renderingDiagrams
	}
	return false
}

func (m Model) wordmark() string {
	letters := []rune("tridennote")
	var b strings.Builder
	for i, r := range letters {
		phase := float64(i)/float64(len(letters)) - float64(m.frame)*0.025
		lift := (math.Sin(float64(i)*0.9-float64(m.frame)*0.35) + 1) / 2
		c := gradientAt(phase).BlendHcl(headColor, lift*0.35).Clamped()
		b.WriteString(colored(c).Bold(true).Render(string(r)))
		if i < len(letters)-1 {
			b.WriteString(" ")
		}
	}
	return b.String()
}

func (m Model) pulse(width int) string {
	const spacing = 6
	travel := width + 8
	head := m.frame % travel
	var b strings.Builder
	for pos := 0; pos < width; pos++ {
		isNode := pos%spacing == 0 || pos == width-1
		glyph := "─"
		if isNode {
			glyph = "○"
		}
		dist := head - pos
		switch {
		case dist == 0:
			if isNode {
				glyph = "◉"
			} else {
				glyph = "━"
			}
			b.WriteString(colored(headColor).Bold(true).Render(glyph))
		case dist > 0 && dist <= 6:
			fade := float64(dist) / 7
			c := gradientAt(float64(pos)/float64(width)-float64(m.frame)*0.01).BlendHcl(trailColor, fade).Clamped()
			if isNode {
				glyph = "●"
			} else {
				glyph = "━"
			}
			b.WriteString(colored(c).Render(glyph))
		default:
			b.WriteString(colored(trailColor).Render(glyph))
		}
	}
	return b.String()
}

func (m Model) phrase(kind loaderKind) string {
	phrases := loaderPhrases[kind]
	idx := (m.frame / framesPerPhrase) % len(phrases)
	text := []rune(phrases[idx] + "…")
	shown := min(len(text), (m.frame%framesPerPhrase)*typingSpeed)
	caret := " "
	if (m.frame/4)%2 == 0 || shown < len(text) {
		caret = "▍"
	}
	visible := string(text[:shown])
	pad := strings.Repeat(" ", len(text)-shown)
	return mutedStyle.Render(visible) + accentStyle.Render(caret) + pad
}

func (m Model) loader(kind loaderKind) string {
	return lipgloss.JoinVertical(lipgloss.Center,
		m.wordmark(),
		"",
		m.pulse(31),
		"",
		m.phrase(kind),
	)
}

// skeletonRows stands in for the sidebar tree while a workspace loads, with a shimmer sweeping down it.
func (m Model) skeletonRows(width, rows int) []string {
	shapes := []struct{ indent, length int }{
		{0, 10}, {1, 14}, {1, 9}, {0, 12}, {0, 16}, {1, 11}, {2, 8}, {1, 13}, {0, 9}, {0, 14},
	}
	base := mustHex("#2a2a36")
	var out []string
	for i := range rows {
		shape := shapes[i%len(shapes)]
		indent := 1 + shape.indent*2
		length := min(shape.length, width-indent-3)
		if length <= 0 {
			break
		}
		wave := (math.Sin(float64(i)*0.6-float64(m.frame)*0.25) + 1) / 2
		c := base.BlendHcl(gradientAt(float64(i)*0.04-float64(m.frame)*0.01), wave*0.45).Clamped()
		style := colored(c)
		out = append(out, strings.Repeat(" ", indent)+style.Render("▪ ")+style.Render(strings.Repeat("▬", length)))
	}
	return out
}

func (m Model) miniLoader(kind loaderKind) string {
	return m.pulse(13) + "  " + m.phrase(kind)
}
