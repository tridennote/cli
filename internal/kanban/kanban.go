package kanban

import (
	"crypto/rand"
	"fmt"
	"regexp"
	"strings"

	"github.com/tridennote/cli/internal/api"
)

const Extension = ".kanban"

var (
	DefaultColumns = []string{"Backlog", "Todo", "In progress", "Done"}
	Priorities     = []string{"low", "medium", "high", "critical"}
)

var (
	priorityPattern = regexp.MustCompile(`(?i)(^|\s)!(low|medium|high|critical)`)
	duePattern      = regexp.MustCompile(`(^|\s)due:(\d{4}-\d{2}-\d{2})`)
	assigneePattern = regexp.MustCompile(`(^|\s)@([\w.-]+)`)
	tagPattern      = regexp.MustCompile(`(^|\s)#([A-Za-z0-9_][\w/-]*)`)
)

type Meta struct {
	Title    string
	Priority string
	Tags     []string
	Assignee string
	Due      string
}

type Card struct {
	Meta
	ID   string
	Done bool
	Body []api.Block
}

type Column struct {
	ID    string
	Name  string
	Intro []api.Block
	Cards []Card
}

type Board struct {
	Preamble []api.Block
	Columns  []Column
}

func IsBoardName(name string) bool {
	return len(name) > len(Extension) && strings.HasSuffix(strings.ToLower(name), Extension)
}

func DisplayName(name string) string {
	if !IsBoardName(name) {
		return name
	}
	return name[:len(name)-len(Extension)]
}

func NewID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func New(columns ...string) Board {
	if len(columns) == 0 {
		columns = DefaultColumns
	}
	var board Board
	for _, name := range columns {
		board.AddColumn(name)
	}
	return board
}

func boundedMatches(pattern *regexp.Regexp, text string) [][]int {
	var out [][]int
	for _, m := range pattern.FindAllStringSubmatchIndex(text, -1) {
		end := m[1]
		if end == len(text) || strings.ContainsRune(" \t\n\r\f\v", rune(text[end])) {
			out = append(out, m)
		}
	}
	return out
}

func stripMatches(text string, matches [][]int) string {
	var b strings.Builder
	last := 0
	for _, m := range matches {
		b.WriteString(text[last:m[3]])
		last = m[1]
	}
	b.WriteString(text[last:])
	return b.String()
}

func ParseMeta(text string) Meta {
	var meta Meta
	rest := text

	if matches := boundedMatches(priorityPattern, rest); len(matches) > 0 {
		meta.Priority = strings.ToLower(rest[matches[0][4]:matches[0][5]])
		rest = stripMatches(rest, matches)
	}
	if matches := boundedMatches(duePattern, rest); len(matches) > 0 {
		meta.Due = rest[matches[0][4]:matches[0][5]]
		rest = stripMatches(rest, matches)
	}
	if matches := assigneePattern.FindAllStringSubmatchIndex(rest, -1); len(matches) > 0 {
		meta.Assignee = rest[matches[0][4]:matches[0][5]]
		rest = stripMatches(rest, matches)
	}
	if matches := tagPattern.FindAllStringSubmatchIndex(rest, -1); len(matches) > 0 {
		seen := map[string]bool{}
		for _, m := range matches {
			tag := rest[m[4]:m[5]]
			if !seen[tag] {
				seen[tag] = true
				meta.Tags = append(meta.Tags, tag)
			}
		}
		rest = stripMatches(rest, matches)
	}
	meta.Title = strings.Join(strings.Fields(rest), " ")
	return meta
}

func (m Meta) Text() string {
	var parts []string
	if title := strings.TrimSpace(m.Title); title != "" {
		parts = append(parts, title)
	}
	if m.Priority != "" {
		parts = append(parts, "!"+m.Priority)
	}
	for _, tag := range m.Tags {
		parts = append(parts, "#"+tag)
	}
	if m.Assignee != "" {
		parts = append(parts, "@"+m.Assignee)
	}
	if m.Due != "" {
		parts = append(parts, "due:"+m.Due)
	}
	return strings.Join(parts, " ")
}

func isCard(b api.Block) bool {
	return (b.Type == "todo" || b.Type == "bulleted") && b.Depth == 0
}

func Parse(blocks []api.Block) Board {
	var board Board
	for _, b := range blocks {
		switch {
		case b.Type == "h2":
			board.Columns = append(board.Columns, Column{ID: b.ID, Name: strings.TrimSpace(b.Content)})
		case len(board.Columns) == 0:
			if b.Type != "text" || strings.TrimSpace(b.Content) != "" {
				board.Preamble = append(board.Preamble, b)
			}
		case isCard(b):
			col := &board.Columns[len(board.Columns)-1]
			done := b.Type == "todo" && b.Checked != nil && *b.Checked
			col.Cards = append(col.Cards, Card{Meta: ParseMeta(b.Content), ID: b.ID, Done: done})
		default:
			col := &board.Columns[len(board.Columns)-1]
			if n := len(col.Cards); n > 0 {
				col.Cards[n-1].Body = append(col.Cards[n-1].Body, b)
			} else {
				col.Intro = append(col.Intro, b)
			}
		}
	}
	return board
}

func (b Board) Blocks() []api.Block {
	out := append([]api.Block{}, b.Preamble...)
	for _, col := range b.Columns {
		out = append(out, api.Block{ID: col.ID, Type: "h2", Content: col.Name})
		out = append(out, col.Intro...)
		for _, card := range col.Cards {
			done := card.Done
			out = append(out, api.Block{ID: card.ID, Type: "todo", Content: card.Text(), Checked: &done})
			out = append(out, card.Body...)
		}
	}
	return out
}

func (b *Board) AddColumn(name string) int {
	return b.InsertColumn(len(b.Columns), name)
}

func (b *Board) InsertColumn(at int, name string) int {
	at = max(0, min(at, len(b.Columns)))
	column := Column{ID: NewID(), Name: strings.TrimSpace(name)}
	b.Columns = append(b.Columns[:at], append([]Column{column}, b.Columns[at:]...)...)
	return at
}

func (b *Board) valid(col, row int) bool {
	return col >= 0 && col < len(b.Columns) && row >= 0 && row < len(b.Columns[col].Cards)
}

func (b *Board) AddCard(col int, text string) int {
	if col < 0 || col >= len(b.Columns) {
		return -1
	}
	card := Card{Meta: ParseMeta(text), ID: NewID(), Done: col == len(b.Columns)-1 && len(b.Columns) > 1}
	b.Columns[col].Cards = append(b.Columns[col].Cards, card)
	return len(b.Columns[col].Cards) - 1
}

func (b *Board) SetCardText(col, row int, text string) {
	if !b.valid(col, row) {
		return
	}
	b.Columns[col].Cards[row].Meta = ParseMeta(text)
}

func (b *Board) ToggleDone(col, row int) {
	if !b.valid(col, row) {
		return
	}
	card := &b.Columns[col].Cards[row]
	card.Done = !card.Done
}

func (b *Board) DeleteCard(col, row int) {
	if !b.valid(col, row) {
		return
	}
	cards := b.Columns[col].Cards
	b.Columns[col].Cards = append(cards[:row:row], cards[row+1:]...)
}

func (b *Board) MoveCard(col, row, toCol, toRow int) (int, int) {
	if !b.valid(col, row) || toCol < 0 || toCol >= len(b.Columns) {
		return col, row
	}
	card := b.Columns[col].Cards[row]
	if toCol != col {
		last := len(b.Columns) - 1
		switch {
		case toCol == last:
			card.Done = true
		case col == last:
			card.Done = false
		}
	}
	b.DeleteCard(col, row)
	cards := b.Columns[toCol].Cards
	toRow = max(0, min(toRow, len(cards)))
	next := make([]Card, 0, len(cards)+1)
	next = append(next, cards[:toRow]...)
	next = append(next, card)
	next = append(next, cards[toRow:]...)
	b.Columns[toCol].Cards = next
	return toCol, toRow
}
