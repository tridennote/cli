package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/tridennote/cli/internal/kanban"
)

func TestTagHueMatchesWeb(t *testing.T) {
	want := map[string]float64{"devops": 285, "qa": 25, "release": 140, "Assets": 350, "area/web": 175}
	for tag, hue := range want {
		if got := tagHue(tag); got != hue {
			t.Fatalf("tagHue(%q) = %v, want %v (must match the web app)", tag, got, hue)
		}
	}
}

func TestTokenStyles(t *testing.T) {
	m := Model{people: []kanban.Person{{Name: "Ana", Handle: "ana"}}}
	cases := []struct {
		token string
		want  bool
	}{
		{"#devops", true},
		{"#", false},
		{"!high", true},
		{"!hi", true},
		{"@ana", true},
		{"@", false},
		{"due:2026-01-02", true},
		{"due:", true},
		{"plain", false},
	}
	for _, tc := range cases {
		if _, ok := m.tokenStyle(tc.token); ok != tc.want {
			t.Fatalf("tokenStyle(%q) styled=%v, want %v", tc.token, ok, tc.want)
		}
	}
	known, _ := m.tokenStyle("@ana")
	unknown, _ := m.tokenStyle("@bob")
	if !known.GetBold() || unknown.GetBold() {
		t.Fatal("known people should be bold, free text should not")
	}
}

func TestHighlightedInputKeepsTextAndScrolls(t *testing.T) {
	m := Model{}
	input := newBoardInput()
	input.SetValue("Ship it !high #ops @ana due:2026-01-02")
	input.Focus()
	if got := ansi.Strip(m.highlightedInput(input, 80)); got != "Ship it !high #ops @ana due:2026-01-02 " {
		t.Fatalf("highlighting must not change the text, got %q", got)
	}
	narrow := ansi.Strip(m.highlightedInput(input, 12))
	if len([]rune(narrow)) != 12 || !strings.HasSuffix(narrow, "01-02 ") {
		t.Fatalf("narrow input should scroll to the cursor, got %q", narrow)
	}
	input.SetCursor(0)
	if got := ansi.Strip(m.highlightedInput(input, 12)); got != "Ship it !hig" {
		t.Fatalf("cursor at start should show the start, got %q", got)
	}
}
