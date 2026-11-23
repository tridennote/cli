package clilink

import (
	"testing"

	"github.com/tridennote/cli/internal/api"
)

func ptr(s string) *string { return &s }

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"My Note":                 "my-note",
		"  Deploy: v2 (final)!  ": "deploy-v2-final",
		"Container Tracking App — Requirements": "container-tracking-app-requirements",
		"notes.md": "notes",
		"Café 日本":  "café-日本",
	}
	for in, want := range cases {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFind(t *testing.T) {
	note := "note"
	rows := []api.TreeNode{
		{ID: "f1", Kind: "folder", Name: "Work Stuff"},
		{ID: "n1", Kind: "file", FileType: &note, Name: "Plan A", ParentID: ptr("f1")},
		{ID: "n2", Kind: "file", FileType: &note, Name: "Plan A"},
		{ID: "n3", Kind: "file", FileType: &note, Name: "Ideas", ParentID: ptr("f1")},
	}
	notes := Notes(rows)
	if len(notes) != 3 || notes[0].Path != "Work Stuff/Plan A.md" || notes[0].Slug != "work-stuff/plan-a" {
		t.Fatalf("notes %+v", notes)
	}
	if n, _ := FindNote(notes, "work-stuff/plan-a"); n.ID != "n1" {
		t.Fatal("full path")
	}
	if n, _ := FindNote(notes, "plan-a"); n.ID != "n2" {
		t.Fatal("root exact match wins over suffix")
	}
	if n, _ := FindNote(notes, "Ideas.md"); n.ID != "n3" {
		t.Fatal("unique suffix by display name")
	}
	if n, _ := FindNote(notes, "n3"); n.ID != "n3" {
		t.Fatal("by id")
	}
	if _, many := FindNote(notes, "missing"); many != nil {
		t.Fatal("no match")
	}
	ws := []api.Workspace{{ID: "w1", Name: "Personal"}, {ID: "w2", Name: "Ops", TeamName: ptr("Swift")}, {ID: "w3", Name: "Ops"}}
	if w, ok := FindWorkspace(ws, "swift/ops"); !ok || w.ID != "w2" {
		t.Fatal("team path")
	}
	if WorkspaceSlug(ws[1], ws) != "swift/ops" || WorkspaceSlug(ws[0], ws) != "personal" {
		t.Fatal("workspace slug")
	}
}
