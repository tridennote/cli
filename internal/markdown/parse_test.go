package markdown

import (
	"testing"

	"github.com/tridennote/cli/internal/api"
)

func TestToBlocksRoundTrip(t *testing.T) {
	blocks := []api.Block{
		{Type: "h1", Content: "Title"},
		{Type: "text", Content: "Intro"},
		{Type: "bulleted", Content: "a"},
		{Type: "bulleted", Content: "nested", Depth: 1},
		{Type: "todo", Content: "done", Checked: ptr(true)},
		{Type: "quote", Content: "wise words"},
		{Type: "code", Content: "x := 1\ny := 2", Language: ptr("go")},
		{Type: "mermaid", Content: "graph TD"},
		{Type: "table", Content: "| a | b |\n| - | - |"},
	}
	got := ToBlocks(FromBlocks(blocks))
	if len(got) != len(blocks) {
		t.Fatalf("got %d blocks, want %d: %+v", len(got), len(blocks), got)
	}
	for i := range blocks {
		if got[i].Type != blocks[i].Type || got[i].Content != blocks[i].Content || got[i].Depth != blocks[i].Depth {
			t.Fatalf("block %d: got %+v want %+v", i, got[i], blocks[i])
		}
	}
	if got[4].Checked == nil || !*got[4].Checked {
		t.Fatalf("todo lost its checked state")
	}
	if got[6].Language == nil || *got[6].Language != "go" {
		t.Fatalf("code lost its language")
	}
}

func TestToBlocksDefaultsCodeLanguage(t *testing.T) {
	got := ToBlocks("```\nplain\n```")
	if len(got) != 1 || got[0].Language == nil || *got[0].Language != "plaintext" {
		t.Fatalf("got %+v", got)
	}
}

func TestReconcileKeepsIdsAndImages(t *testing.T) {
	previous := []api.Block{
		{ID: "b1", Type: "h1", Content: "Title"},
		{ID: "b2", Type: "image", Content: "![cat](attachment)"},
		{ID: "b3", Type: "text", Content: "old line"},
	}
	next := ToBlocks("# Title\n\n![cat](attachment)\n\nbrand new line\n")
	got := Reconcile(previous, next)
	if got[0].ID != "b1" {
		t.Fatalf("unchanged heading should keep id: %+v", got[0])
	}
	if got[1].ID != "b2" || got[1].Type != "image" {
		t.Fatalf("image block should keep id and type: %+v", got[1])
	}
	if got[2].ID != "" {
		t.Fatalf("new line should get a fresh id: %+v", got[2])
	}
}
