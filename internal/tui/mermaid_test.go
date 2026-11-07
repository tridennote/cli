package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/tridennote/cli/internal/api"
)

func pickerModel(t *testing.T) Model {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	m := New(api.New("http://unused"))
	m.width, m.height = 100, 40
	m.notes["n1"] = api.Note{Title: "Flow", Blocks: []api.Block{
		{Type: "h2", Content: "Login"},
		{Type: "mermaid", Content: "sequenceDiagram\n  A->>B: hi"},
		{Type: "mermaid", Content: "graph TD\n  A --> B"},
		{Type: "h2", Content: "Pets"},
		{Type: "mermaid", Content: "pie title Pets\n  \"Dogs\" : 3"},
		{Type: "mermaid", Content: "  "},
	}}
	if cmd := m.requestDiagrams("n1"); cmd != nil {
		t.Fatalf("picker should open without a command")
	}
	return m
}

func TestDiagramPickerLists(t *testing.T) {
	m := pickerModel(t)
	if m.modal != modalDiagrams || len(m.diagrams) != 3 {
		t.Fatalf("modal = %v, diagrams = %d", m.modal, len(m.diagrams))
	}
	view := ansi.Strip(m.diagramsView())
	for _, want := range []string{"Login · sequence", "Login · flowchart", "Pets · pie", "Opens this one"} {
		if !strings.Contains(view, want) {
			t.Fatalf("picker missing %q:\n%s", want, view)
		}
	}
}

func TestDiagramPickerChoosesCursorWhenNothingMarked(t *testing.T) {
	m := pickerModel(t)
	m.handleDiagramsKey(key("down"))
	m.handleDiagramsKey(key("down"))
	chosen := m.chosenDiagrams()
	if len(chosen) != 1 || chosen[0].label != "Pets · pie" {
		t.Fatalf("chosen = %+v", chosen)
	}
}

func TestDiagramPickerMarksSeveral(t *testing.T) {
	m := pickerModel(t)
	space := tea.KeyMsg{Type: tea.KeySpace}
	m.handleDiagramsKey(space) // marks 1, cursor to 2
	m.handleDiagramsKey(key("down"))
	m.handleDiagramsKey(space) // marks 3
	chosen := m.chosenDiagrams()
	if len(chosen) != 2 || chosen[0].label != "Login · sequence" || chosen[1].label != "Pets · pie" {
		t.Fatalf("chosen = %+v", chosen)
	}
	if !strings.Contains(ansi.Strip(m.diagramsView()), "Opens 2 marked diagrams") {
		t.Fatalf("footer should count marked diagrams")
	}
	m.handleDiagramsKey(key("a"))
	if len(m.chosenDiagrams()) != 3 {
		t.Fatalf("a should mark all")
	}
	m.handleDiagramsKey(key("a"))
	if m.pickedCount() != 0 {
		t.Fatalf("a again should clear marks")
	}
}

func TestDiagramPickerEnterRendersImages(t *testing.T) {
	m := pickerModel(t)
	cmd := m.handleDiagramsKey(key("enter"))
	if cmd == nil || !m.renderingDiagrams || m.modal != modalNone {
		t.Fatalf("enter should close the picker and start rendering")
	}
}
