package kanban

import (
	"reflect"
	"regexp"
	"testing"

	"github.com/tridennote/cli/internal/api"
)

func ptr[T any](v T) *T { return &v }

func TestParseMeta(t *testing.T) {
	cases := []struct {
		name string
		text string
		want Meta
	}{
		{"plain", "Write API docs", Meta{Title: "Write API docs"}},
		{"everything", "Set up CI pipeline !high #devops #ci @harith due:2026-10-10", Meta{Title: "Set up CI pipeline", Priority: "high", Tags: []string{"devops", "ci"}, Assignee: "harith", Due: "2026-10-10"}},
		{"tokens anywhere", "#infra Fix @sam the !critical login bug", Meta{Title: "Fix the login bug", Priority: "critical", Tags: []string{"infra"}, Assignee: "sam"}},
		{"priority case-insensitive", "Ship !HIGH", Meta{Title: "Ship", Priority: "high"}},
		{"first priority wins", "Ship !low !high", Meta{Title: "Ship", Priority: "low"}},
		{"first assignee wins", "Pair @ana @ben", Meta{Title: "Pair", Assignee: "ana"}},
		{"unknown priority stays", "Wow !urgent", Meta{Title: "Wow !urgent"}},
		{"priority needs word end", "Wow !highest", Meta{Title: "Wow !highest"}},
		{"needs leading whitespace", "mail me@example.com about issue#4", Meta{Title: "mail me@example.com about issue#4"}},
		{"nested tags and dedupe", "Plan #area/web #area/web #q4-goals", Meta{Title: "Plan", Tags: []string{"area/web", "q4-goals"}}},
		{"bad due stays", "Renew due:soon", Meta{Title: "Renew due:soon"}},
		{"assignee with dots", "Review @first.last-name", Meta{Title: "Review", Assignee: "first.last-name"}},
		{"collapses whitespace", "  a   #x   b  ", Meta{Title: "a b", Tags: []string{"x"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseMeta(tc.text); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ParseMeta(%q) = %+v, want %+v", tc.text, got, tc.want)
			}
		})
	}
}

func TestMetaText(t *testing.T) {
	cases := []struct {
		meta Meta
		want string
	}{
		{Meta{Title: "Write docs"}, "Write docs"},
		{Meta{Title: "Fix", Priority: "critical", Tags: []string{"a", "b"}, Assignee: "sam", Due: "2026-01-02"}, "Fix !critical #a #b @sam due:2026-01-02"},
		{Meta{Tags: []string{"only"}}, "#only"},
	}
	for _, tc := range cases {
		if got := tc.meta.Text(); got != tc.want {
			t.Fatalf("Text() = %q, want %q", got, tc.want)
		}
		if again := ParseMeta(tc.want); again.Text() != tc.want {
			t.Fatalf("re-parse of %q changed it to %q", tc.want, again.Text())
		}
	}
}

func sample() []api.Block {
	return []api.Block{
		{ID: "p1", Type: "text", Content: "Board notes"},
		{ID: "p2", Type: "text", Content: "  "},
		{ID: "p3", Type: "h1", Content: "Roadmap"},
		{ID: "c1", Type: "h2", Content: " Backlog "},
		{ID: "i1", Type: "quote", Content: "Ideas go here"},
		{ID: "k1", Type: "todo", Content: "Set up CI pipeline !high #devops #ci @harith due:2026-10-10", Checked: ptr(false)},
		{ID: "b1", Type: "bulleted", Content: "Card body line", Depth: 1},
		{ID: "b2", Type: "todo", Content: "Sub-task in the body", Checked: ptr(true), Depth: 1},
		{ID: "b3", Type: "h3", Content: "Details"},
		{ID: "k2", Type: "bulleted", Content: "Write API docs"},
		{ID: "c2", Type: "h2", Content: "In progress"},
		{ID: "c3", Type: "h2", Content: "Done"},
		{ID: "k3", Type: "todo", Content: "Fix login bug !critical", Checked: ptr(true)},
		{ID: "b4", Type: "code", Content: "x := 1", Language: ptr("go")},
	}
}

func TestParse(t *testing.T) {
	board := Parse(sample())
	if len(board.Preamble) != 2 || board.Preamble[0].ID != "p1" || board.Preamble[1].ID != "p3" {
		t.Fatalf("preamble should keep non-empty blocks and h1: %+v", board.Preamble)
	}
	if len(board.Columns) != 3 {
		t.Fatalf("want 3 columns, got %+v", board.Columns)
	}
	backlog := board.Columns[0]
	if backlog.Name != "Backlog" || backlog.ID != "c1" {
		t.Fatalf("column name should be trimmed: %+v", backlog)
	}
	if len(backlog.Intro) != 1 || backlog.Intro[0].ID != "i1" {
		t.Fatalf("intro wrong: %+v", backlog.Intro)
	}
	if len(backlog.Cards) != 2 {
		t.Fatalf("want 2 cards, got %+v", backlog.Cards)
	}
	ci := backlog.Cards[0]
	if ci.Title != "Set up CI pipeline" || ci.Priority != "high" || ci.Assignee != "harith" || ci.Due != "2026-10-10" || !reflect.DeepEqual(ci.Tags, []string{"devops", "ci"}) {
		t.Fatalf("card meta wrong: %+v", ci)
	}
	if len(ci.Body) != 3 || ci.Body[2].ID != "b3" {
		t.Fatalf("body should hold nested blocks and non-column headings: %+v", ci.Body)
	}
	if backlog.Cards[1].Done || backlog.Cards[1].ID != "k2" {
		t.Fatalf("bulleted card should be an open card: %+v", backlog.Cards[1])
	}
	if len(board.Columns[1].Cards) != 0 {
		t.Fatalf("in progress should be empty")
	}
	done := board.Columns[2].Cards[0]
	if !done.Done || done.Priority != "critical" || len(done.Body) != 1 {
		t.Fatalf("done card wrong: %+v", done)
	}
}

func TestRoundTrip(t *testing.T) {
	in := sample()
	out := Parse(in).Blocks()

	var want []api.Block
	for _, b := range in {
		if b.ID == "p2" {
			continue
		}
		switch b.ID {
		case "c1":
			b.Content = "Backlog"
		case "k1":
			b.Content = "[#1] " + b.Content
		case "k2":
			b.Type = "todo"
			b.Checked = ptr(false)
			b.Content = "[#2] " + b.Content
		case "k3":
			b.Content = "[#3] " + b.Content
		}
		want = append(want, b)
	}
	if len(out) != len(want) {
		t.Fatalf("got %d blocks want %d:\n%+v", len(out), len(want), out)
	}
	for i := range want {
		g, w := out[i], want[i]
		if g.ID != w.ID || g.Type != w.Type || g.Content != w.Content || g.Depth != w.Depth {
			t.Fatalf("block %d: got %+v want %+v", i, g, w)
		}
		if (g.Checked == nil) != (w.Checked == nil) || (g.Checked != nil && *g.Checked != *w.Checked) {
			t.Fatalf("block %d checked: got %v want %v", i, g.Checked, w.Checked)
		}
	}
	if again := Parse(out).Blocks(); !reflect.DeepEqual(again, out) {
		t.Fatalf("second round trip changed blocks:\n%+v\n%+v", again, out)
	}
}

func TestNewBoardDefaults(t *testing.T) {
	blocks := New().Blocks()
	uuid := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	var names []string
	seen := map[string]bool{}
	for _, b := range blocks {
		if b.Type != "h2" || !uuid.MatchString(b.ID) || seen[b.ID] {
			t.Fatalf("default column block wrong: %+v", b)
		}
		seen[b.ID] = true
		names = append(names, b.Content)
	}
	if !reflect.DeepEqual(names, DefaultColumns) {
		t.Fatalf("got columns %v", names)
	}
}

func TestMoveCardTogglesDoneAtLastColumn(t *testing.T) {
	board := New()
	row := board.AddCard(0, "Ship it !high")
	col, row := board.MoveCard(0, row, 2, 0)
	if board.Columns[2].Cards[row].Done {
		t.Fatalf("moving between open columns should not check")
	}
	col, row = board.MoveCard(col, row, 3, 5)
	card := board.Columns[col].Cards[row]
	if col != 3 || row != 0 || !card.Done {
		t.Fatalf("moving into the last column should check: col=%d row=%d %+v", col, row, card)
	}
	board.ToggleDone(col, row)
	board.ToggleDone(col, row)
	col, row = board.MoveCard(col, row, 1, 0)
	if board.Columns[col].Cards[row].Done {
		t.Fatalf("moving out of the last column should uncheck")
	}
	board.ToggleDone(col, row)
	if !board.Columns[col].Cards[row].Done || col != 1 {
		t.Fatalf("toggling should not move the card")
	}
}

func TestReorderAndDelete(t *testing.T) {
	board := New("Todo", "Done")
	board.AddCard(0, "a")
	board.AddCard(0, "b")
	board.AddCard(0, "c")
	_, row := board.MoveCard(0, 2, 0, 0)
	if row != 0 {
		t.Fatalf("row = %d", row)
	}
	titles := func() []string {
		var out []string
		for _, c := range board.Columns[0].Cards {
			out = append(out, c.Title)
		}
		return out
	}
	if got := titles(); !reflect.DeepEqual(got, []string{"c", "a", "b"}) {
		t.Fatalf("reorder got %v", got)
	}
	board.DeleteCard(0, 1)
	if got := titles(); !reflect.DeepEqual(got, []string{"c", "b"}) {
		t.Fatalf("delete got %v", got)
	}
	board.SetCardText(0, 0, "see #x @y")
	if c := board.Columns[0].Cards[0]; c.Title != "see" || c.Assignee != "y" || c.Tags[0] != "x" {
		t.Fatalf("set text got %+v", c)
	}
	if row := board.AddCard(1, "landed"); !board.Columns[1].Cards[row].Done {
		t.Fatalf("a card added to the last column starts done")
	}
}

func TestBoardNames(t *testing.T) {
	cases := map[string]bool{"Roadmap.kanban": true, "x.KANBAN": true, ".kanban": false, "kanban": false, "notes.md": false}
	for name, want := range cases {
		if got := IsBoardName(name); got != want {
			t.Fatalf("IsBoardName(%q) = %v", name, got)
		}
	}
	if got := DisplayName("Roadmap.Kanban"); got != "Roadmap" {
		t.Fatalf("DisplayName = %q", got)
	}
	if got := DisplayName("plain"); got != "plain" {
		t.Fatalf("DisplayName = %q", got)
	}
}

func TestCardNumbersAndNoteLinks(t *testing.T) {
	const noteID = "0f8fad5b-d9cb-469f-a165-70867728950e"
	meta := ParseMeta("[#12] Release plan !high note:" + "0F8FAD5B-D9CB-469F-A165-70867728950E")
	want := Meta{Number: 12, NoteID: noteID, Title: "Release plan", Priority: "high"}
	if !reflect.DeepEqual(meta, want) {
		t.Fatalf("ParseMeta = %+v, want %+v", meta, want)
	}
	if got := meta.Text(); got != "[#12] Release plan !high note:"+noteID {
		t.Fatalf("Text() = %q", got)
	}
	if got := meta.EditableText(); got != "Release plan !high" {
		t.Fatalf("EditableText() = %q", got)
	}
	if tag := ParseMeta("Fix bug #12"); tag.Number != 0 || !reflect.DeepEqual(tag.Tags, []string{"12"}) {
		t.Fatalf("plain #12 should stay a tag, got %+v", tag)
	}
}

func TestNumberingAndLinking(t *testing.T) {
	const noteID = "0f8fad5b-d9cb-469f-a165-70867728950e"
	board := New("Todo", "Done")
	board.Columns[0].Cards = []Card{{Meta: Meta{Number: 4, Title: "Numbered"}, ID: "a"}, {Meta: Meta{Title: "Legacy"}, ID: "b"}}

	row := board.AddCard(0, "New !low")
	if got := board.Columns[0].Cards[row].Number; got != 6 {
		t.Fatalf("AddCard number = %d, want 6 (after numbering the legacy card)", got)
	}
	if got := board.Columns[0].Cards[1].Number; got != 5 {
		t.Fatalf("legacy card should be numbered before the new one, got %d", got)
	}
	board.SetCardText(0, row, "Renamed #x")
	if card := board.Columns[0].Cards[row]; card.Number != 6 || card.Title != "Renamed" {
		t.Fatalf("SetCardText lost the number: %+v", card)
	}

	linked := board.LinkNote(1, Meta{Title: " Release plan ", Priority: "high"}, noteID)
	card := board.Columns[1].Cards[linked]
	if card.Number != 7 || card.NoteID != noteID || card.Title != "Release plan" || card.Priority != "high" || !card.Done {
		t.Fatalf("LinkNote card = %+v", card)
	}
	if refs := board.LinksTo(noteID); len(refs) != 1 || refs[0].ColumnName != "Done" {
		t.Fatalf("LinksTo = %+v", refs)
	}

	reparsed := Parse(board.Blocks())
	var numbers []int
	for _, col := range reparsed.Columns {
		for _, c := range col.Cards {
			numbers = append(numbers, c.Number)
		}
	}
	if !reflect.DeepEqual(numbers, []int{4, 5, 6, 7}) {
		t.Fatalf("numbers after save = %v", numbers)
	}
	untouched := New("Todo")
	untouched.Columns[0].Cards = []Card{{Meta: Meta{Title: "Legacy"}, ID: "z"}}
	_ = untouched.Blocks()
	if untouched.Columns[0].Cards[0].Number != 0 {
		t.Fatal("Blocks() must not mutate the board")
	}
}

func TestBodyConversionAndLocate(t *testing.T) {
	note := []api.Block{
		{Type: "h1", Content: "Plan"},
		{Type: "text", Content: ""},
		{Type: "bulleted", Content: "Build", Depth: 0},
		{Type: "todo", Content: "Deploy", Depth: 1, Checked: ptr(false)},
		{Type: "code", Content: "vp build"},
	}
	body := NoteToBody(note)
	got := [][3]any{}
	for _, b := range body {
		got = append(got, [3]any{b.Type, b.Content, b.Depth})
		if b.ID == "" {
			t.Fatal("body blocks need ids")
		}
	}
	want := [][3]any{{"h3", "Plan", 0}, {"bulleted", "Build", 1}, {"todo", "Deploy", 2}, {"code", "vp build", 0}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("NoteToBody = %v, want %v", got, want)
	}
	back := BodyToNote(body)
	if back[1].Depth != 0 || back[2].Depth != 1 || back[0].ID != "" {
		t.Fatalf("BodyToNote should undo list nesting and drop ids: %+v", back)
	}

	board := Parse(sample())
	col, row, ok := board.Locate("k3")
	if !ok || board.Columns[col].Cards[row].ID != "k3" {
		t.Fatalf("Locate(k3) = %d,%d,%v", col, row, ok)
	}
	if _, _, ok := board.Locate("missing"); ok {
		t.Fatal("Locate should miss unknown ids")
	}
}
