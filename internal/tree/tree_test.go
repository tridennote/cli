package tree

import (
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
