package tui

import (
	"github.com/charmbracelet/lipgloss"

	"github.com/tridennote/cli/internal/settings"
)

type iconSet struct {
	FolderClosed string
	FolderOpen   string
	Note         string
	Logo         string
	Settings     string
	Help         string
	Copy         string
	Saved        string
	Edit         string
	Sidebar      string
	Warn         string
	Diagram      string
	Board        string
}

var iconSets = map[string]iconSet{
	settings.IconsNerd: {
		FolderClosed: "",
		FolderOpen:   "",
		Note:         "",
		Logo:         "Ψ",
		Settings:     "",
		Help:         "",
		Copy:         "",
		Saved:        "",
		Edit:         "",
		Sidebar:      "",
		Warn:         "",
		Diagram:      "\uf0e8",
		Board:        "\uf0db",
	},
	settings.IconsMinimal: {
		Logo:     "Ψ",
		Settings: "*",
		Help:     "?",
		Copy:     "»",
		Saved:    "✓",
		Edit:     "✎",
		Sidebar:  "|",
		Warn:     "!",
		Diagram:  "◇",
		Board:    "▦",
	},
}

func (m Model) icons() iconSet {
	if set, ok := iconSets[m.settings.Icons]; ok {
		return set
	}
	return iconSets[settings.IconsNerd]
}

func (m Model) withIcon(icon, label string) string {
	if icon == "" {
		return label
	}
	return icon + " " + label
}

func (m Model) editorIcon(e editorOption) string {
	switch m.settings.Icons {
	case settings.IconsNerd:
		return lipgloss.NewStyle().Foreground(e.Color).Render(e.Nerd)
	}
	return lipgloss.NewStyle().Foreground(e.Color).Render("●")
}
