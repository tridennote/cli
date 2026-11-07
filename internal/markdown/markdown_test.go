package markdown

import (
	"testing"

	"github.com/tridennote/cli/internal/api"
)

func ptr[T any](v T) *T { return &v }

func TestFromBlocks(t *testing.T) {
	blocks := []api.Block{
		{Type: "h1", Content: "Title"},
		{Type: "text", Content: "Intro"},
		{Type: "numbered", Content: "one"},
		{Type: "numbered", Content: "two"},
		{Type: "numbered", Content: "nested", Depth: 1},
		{Type: "todo", Content: "done", Checked: ptr(true)},
		{Type: "todo", Content: "open"},
		{Type: "code", Content: "x := 1", Language: ptr("go")},
		{Type: "code", Content: "plain", Language: ptr("plaintext")},
	}
	want := "# Title\n\nIntro\n\n1. one\n2. two\n  1. nested\n\n- [x] done\n- [ ] open\n\n```go\nx := 1\n```\n\n```\nplain\n```\n"
	if got := FromBlocks(blocks); got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}
