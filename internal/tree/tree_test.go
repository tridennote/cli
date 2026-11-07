package tree

import (
	"strings"
	"testing"

	"github.com/tridennote/cli/internal/api"
)

func s(v string) *string { return &v }

func TestBuildFiltersAndSorts(t *testing.T) {
	rows := []api.TreeNode{
		{ID: "n2", Kind: "file", FileType: s("note"), Name: "zeta"},
		{ID: "x1", Kind: "file", FileType: s("excalidraw"), Name: "sketch.excalidraw"},
		{ID: "f1", Kind: "folder", Name: "Projects"},
		{ID: "n1", Kind: "file", FileType: s("note"), Name: "Alpha"},
		{ID: "p1", Kind: "file", FileType: s("file"), Name: "doc.pdf", ParentID: s("f1")},
		{ID: "n3", Kind: "file", FileType: s("note"), Name: "plan", ParentID: s("f1")},
	}
	roots := Build(rows)
	var labels []string
	for _, n := range roots {
		labels = append(labels, n.Label())
	}
	want := []string{"Projects", "Alpha.md", "zeta.md"}
	if len(labels) != len(want) {
		t.Fatalf("got %v want %v", labels, want)
	}
	for i := range want {
		if labels[i] != want[i] {
			t.Fatalf("got %v want %v", labels, want)
		}
	}
	if len(roots[0].Children) != 1 || roots[0].Children[0].Label() != "plan.md" {
		t.Fatalf("folder children not filtered: %+v", roots[0].Children)
	}
	if roots[0].Children[0].Depth != 1 {
		t.Fatalf("depth not set")
	}
}

func TestVisibleRespectsExpansion(t *testing.T) {
	rows := []api.TreeNode{
		{ID: "f1", Kind: "folder", Name: "A"},
		{ID: "n1", Kind: "file", FileType: s("note"), Name: "inside", ParentID: s("f1")},
	}
	roots := Build(rows)
	if got := len(Visible(roots)); got != 1 {
		t.Fatalf("collapsed visible = %d", got)
	}
	roots[0].Expanded = true
	if got := len(Visible(roots)); got != 2 {
		t.Fatalf("expanded visible = %d", got)
	}
	Restore(roots, map[string]bool{})
	if roots[0].Expanded {
		t.Fatalf("restore did not collapse")
	}
}

func TestBoardsDropSuffix(t *testing.T) {
	roots := Build([]api.TreeNode{
		{ID: "k1", Kind: "file", FileType: s("note"), Name: "Roadmap.kanban"},
		{ID: "f1", Kind: "folder", Name: "Team.kanban"},
	})
	if len(roots) != 2 {
		t.Fatalf("got %+v", roots)
	}
	if roots[0].IsBoard || roots[0].Label() != "Team.kanban" {
		t.Fatalf("folders are never boards: %+v", roots[0])
	}
	if !roots[1].IsBoard || roots[1].Label() != "Roadmap" || roots[1].Name != "Roadmap.kanban" {
		t.Fatalf("board label should drop the suffix: %+v", roots[1])
	}
}

func TestExternalsFlattenWithFolderPath(t *testing.T) {
	rows := []api.TreeNode{
		{ID: "f1", Kind: "folder", Name: "Projects"},
		{ID: "f2", Kind: "folder", Name: "Specs", ParentID: s("f1")},
		{ID: "n1", Kind: "file", FileType: s("note"), Name: "plan", ParentID: s("f1")},
		{ID: "p1", Kind: "file", FileType: s("file"), Name: "doc.pdf", ParentID: s("f2")},
		{ID: "x1", Kind: "file", FileType: s("excalidraw"), Name: "sketch"},
		{ID: "x2", Kind: "file", FileType: s("excalidraw"), Name: "arch.excalidraw"},
	}
	externals := Externals(rows)
	var got []string
	for _, n := range externals {
		if !n.External {
			t.Fatalf("not marked external: %+v", n)
		}
		got = append(got, n.Dir+"|"+n.Label())
	}
	want := []string{"|arch.excalidraw", "|sketch.excalidraw", "Projects/Specs|doc.pdf"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v want %v", got, want)
	}

	roots := Build(rows)
	all := Rows(roots, externals)
	if len(all) != 1+1+3 || !all[1].Divider || all[2].ID != "x2" {
		t.Fatalf("rows should be tree, divider, externals: %+v", all)
	}
	if len(Rows(nil, externals)) != 3 {
		t.Fatalf("no divider without notes above it")
	}
	if len(Rows(roots, nil)) != 1 {
		t.Fatalf("no divider without externals")
	}
}

func TestCSVFilesJoinTheNoteTree(t *testing.T) {
	rows := []api.TreeNode{
		{ID: "f1", Kind: "folder", Name: "Ops"},
		{ID: "c1", Kind: "file", FileType: s("file"), Name: "trips.CSV", ParentID: s("f1"), PrimaryAttachmentID: s("a1")},
		{ID: "c2", Kind: "file", FileType: s("file"), Name: "orphan.csv"},
		{ID: "p1", Kind: "file", FileType: s("file"), Name: "doc.pdf", PrimaryAttachmentID: s("a2")},
	}
	roots := Build(rows)
	if len(roots) != 1 || len(roots[0].Children) != 1 {
		t.Fatalf("only the folder and its csv belong in the tree: %+v", roots)
	}
	csv := roots[0].Children[0]
	if !csv.IsCSV || csv.AttachmentID != "a1" || csv.Label() != "trips.CSV" {
		t.Fatalf("csv node = %+v", csv)
	}
	externals := Externals(rows)
	if len(externals) != 2 || externals[0].ID != "p1" || externals[1].ID != "c2" {
		var ids []string
		for _, e := range externals {
			ids = append(ids, e.ID)
		}
		t.Fatalf("pdf and the csv without bytes stay external, got %v", ids)
	}
}
