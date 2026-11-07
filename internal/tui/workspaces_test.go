package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestWorkspacePicker(t *testing.T) {
	m, srv := signedIn(t, &fakeServer{teams: true})
	defer srv.Close()

	m, _ = step(t, m, key("w"))
	if m.modal != modalWorkspaces {
		t.Fatalf("w should open the workspace picker")
	}
	view := ansi.Strip(m.View())
	for _, want := range []string{"Workspaces", "Personal", "current", "Swift Ops", "Side projects", "3"} {
		if !strings.Contains(view, want) {
			t.Fatalf("picker missing %q:\n%s", want, view)
		}
	}

	m, _ = step(t, m, key("j"))
	m, _ = step(t, m, key("j"))
	m, cmd := step(t, m, key("enter"))
	if m.modal != modalNone || m.workspace != 2 || cmd == nil {
		t.Fatalf("enter should switch to Side projects (workspace %d, modal %v)", m.workspace, m.modal)
	}
	if !m.needsAnimation() {
		t.Fatalf("switching workspace should animate while the tree loads")
	}
	loading := ansi.Strip(m.View())
	for _, want := range []string{"Side projects", "▬", "t r i d e n n o t e"} {
		if !strings.Contains(loading, want) {
			t.Fatalf("loading view missing %q:\n%s", want, loading)
		}
	}
	if strings.Contains(loading, "Daily log") {
		t.Fatalf("the old workspace's notes should be gone while loading:\n%s", loading)
	}
	t.Log("\n" + loading)
	m, _ = step(t, m, cmd())
	if m.loadingWorkspace != "" || m.flash != "Switched to Side projects" {
		t.Fatalf("loading should finish with a flash, got %q / %q", m.loadingWorkspace, m.flash)
	}
	if node := m.selected(); node == nil || node.ID != "w3-n" {
		t.Fatalf("tree should reload for the new workspace, got %+v", node)
	}

	m, _ = step(t, m, key("w"))
	m, cmd = step(t, m, key("2"))
	if m.workspace != 1 || cmd == nil {
		t.Fatalf("number keys should jump straight to a workspace, got %d", m.workspace)
	}

	m, _ = step(t, m, key("w"))
	m, cmd = step(t, m, key("esc"))
	if m.modal != modalNone || m.workspace != 1 || cmd != nil {
		t.Fatalf("esc should close without switching")
	}
}

func TestWorkspacePickerNeedsTwoWorkspaces(t *testing.T) {
	m, srv := signedIn(t, &fakeServer{})
	defer srv.Close()
	m, _ = step(t, m, key("w"))
	if m.modal != modalNone || m.flash != "This is your only workspace" {
		t.Fatalf("modal %v, flash %q", m.modal, m.flash)
	}
}
