package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func stubBrowser(t *testing.T) *[]string {
	t.Helper()
	var opened []string
	prev := openBrowser
	openBrowser = func(url string) error {
		opened = append(opened, url)
		return nil
	}
	t.Cleanup(func() { openBrowser = prev })
	return &opened
}

func TestExternalFilesOpenInBrowser(t *testing.T) {
	opened := stubBrowser(t)
	m, srv := signedIn(t, &fakeServer{})
	defer srv.Close()

	// Rows: Projects/, Daily log.md, divider, invoice.pdf, Whiteboard.excalidraw.
	m, _ = step(t, m, key("down"))
	m, cmd := step(t, m, key("down"))
	if node := m.selected(); node == nil || node.ID != "p1" {
		t.Fatalf("down should skip the divider onto invoice.pdf, got %+v", node)
	}
	m, _ = step(t, m, cmd())
	if m.openID == "p1" || m.loadingNote == "p1" {
		t.Fatalf("external files should not load as notes")
	}
	m, _ = step(t, m, key("k"))
	if m.selected().ID != "n1" {
		t.Fatalf("up should skip the divider back to the note, got %s", m.selected().ID)
	}
	m, _ = step(t, m, key("j"))

	m, _ = step(t, m, key("e"))
	if m.flash != "invoice.pdf only opens in the browser — press ⏎ or o" {
		t.Fatalf("flash = %q", m.flash)
	}
	m, _ = step(t, m, key("enter"))
	if len(*opened) != 1 || (*opened)[0] != m.client.BaseURL+"/note/p1" {
		t.Fatalf("opened = %v", *opened)
	}
	if m.flash != "Opened invoice.pdf in the browser" {
		t.Fatalf("flash = %q", m.flash)
	}

	m, _ = step(t, m, key("k"))
	m, _ = step(t, m, key("o"))
	if len(*opened) != 2 || (*opened)[1] != m.client.BaseURL+"/note/n1" {
		t.Fatalf("o should open notes in the web app too: %v", *opened)
	}
}

func TestStatusBarDimsNoteKeysOnExternalFiles(t *testing.T) {
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
	m, srv := signedIn(t, &fakeServer{})
	defer srv.Close()
	if strings.Contains(m.statusBar(), "browser") {
		t.Fatalf("browser hint should only show on external files")
	}
	enabled := m.statusBar()

	m, _ = step(t, m, key("down"))
	m, _ = step(t, m, key("down"))
	bar := m.statusBar()
	if !strings.Contains(ansi.Strip(bar), "⏎ browser") {
		t.Fatalf("status bar should offer ⏎ browser: %q", ansi.Strip(bar))
	}
	for _, k := range []string{"e edit", "y copy"} {
		if !strings.Contains(bar, faintStyle.Render(k)) {
			t.Fatalf("%q should be dimmed on external files", k)
		}
		if strings.Contains(enabled, faintStyle.Render(k)) {
			t.Fatalf("%q should not be dimmed on notes", k)
		}
	}
}

func TestViewerShowsPlaceholderForExternalFiles(t *testing.T) {
	m, srv := signedIn(t, &fakeServer{})
	defer srv.Close()

	// Open Daily log, then move onto invoice.pdf.
	m, _ = step(t, m, key("down"))
	m, cmd := step(t, m, key("enter"))
	m, _ = step(t, m, cmd())
	m, cmd = step(t, m, key("down"))
	m, _ = step(t, m, cmd())

	view := snapshot(t, "15-external-placeholder", m)
	for _, want := range []string{"t r i d e n n o t e", "invoice.pdf", "PDF · can't be shown in the terminal", "⏎ open in browser"} {
		if !strings.Contains(view, want) {
			t.Fatalf("placeholder missing %q:\n%s", want, view)
		}
	}

	m, cmd = step(t, m, key("down"))
	m, _ = step(t, m, cmd())
	if view := snapshot(t, "16-external-placeholder-dir", m); !strings.Contains(view, "Projects/Whiteboard.excalidraw") || !strings.Contains(view, "Excalidraw drawing") {
		t.Fatalf("placeholder should show the folder and kind:\n%s", view)
	}

	m, _ = step(t, m, key("up"))
	m, cmd = step(t, m, key("up"))
	m, _ = step(t, m, cmd())
	if view := ansi.Strip(m.View()); strings.Contains(view, "can't be shown") {
		t.Fatalf("moving back to the open note should restore it:\n%s", view)
	}
}
