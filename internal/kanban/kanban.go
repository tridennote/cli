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
	numberPattern   = regexp.MustCompile(`(^|\s)\[#(\d+)\]`)
	notePattern     = regexp.MustCompile(`(?i)(^|\s)note:([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})`)
)

type Meta struct {
	Number   int
	NoteID   string
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

	if matches := boundedMatches(numberPattern, rest); len(matches) > 0 {
		_, _ = fmt.Sscanf(rest[matches[0][4]:matches[0][5]], "%d", &meta.Number)
		rest = stripMatches(rest, matches)
	}
	if matches := boundedMatches(notePattern, rest); len(matches) > 0 {
		meta.NoteID = strings.ToLower(rest[matches[0][4]:matches[0][5]])
		rest = stripMatches(rest, matches)
	}

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
	if m.Number > 0 {
		parts = append(parts, fmt.Sprintf("[#%d]", m.Number))
	}
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
	if m.NoteID != "" {
		parts = append(parts, "note:"+m.NoteID)
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

func (b Board) NextNumber() int {
	highest := 0
	for _, col := range b.Columns {
		for _, card := range col.Cards {
			highest = max(highest, card.Number)
		}
	}
	return highest + 1
}

func (b *Board) EnsureNumbers() {
	next := b.NextNumber()
	for c := range b.Columns {
		for r := range b.Columns[c].Cards {
			if b.Columns[c].Cards[r].Number == 0 {
				b.Columns[c].Cards[r].Number = next
				next++
			}
		}
	}
}

func (b Board) Blocks() []api.Block {
	b.Columns = append([]Column(nil), b.Columns...)
	for i := range b.Columns {
		b.Columns[i].Cards = append([]Card(nil), b.Columns[i].Cards...)
	}
	b.EnsureNumbers()
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
	b.EnsureNumbers()
	meta := ParseMeta(text)
	meta.Number = b.NextNumber()
	return b.appendCard(col, meta)
}

func (b *Board) LinkNote(col int, meta Meta, noteID string) int {
	if col < 0 || col >= len(b.Columns) {
		return -1
	}
	b.EnsureNumbers()
	meta.Title = strings.TrimSpace(meta.Title)
	meta.Number = b.NextNumber()
	meta.NoteID = noteID
	return b.appendCard(col, meta)
}

func (b Board) Locate(cardID string) (int, int, bool) {
	for c, col := range b.Columns {
		for r, card := range col.Cards {
			if card.ID == cardID {
				return c, r, true
			}
		}
	}
	return 0, 0, false
}

func isList(b api.Block) bool {
	return b.Type == "todo" || b.Type == "bulleted" || b.Type == "numbered"
}

func BodyToNote(body []api.Block) []api.Block {
	out := make([]api.Block, 0, len(body))
	for _, b := range body {
		b.ID = ""
		if isList(b) {
			b.Depth = max(0, b.Depth-1)
		}
		out = append(out, b)
	}
	return out
}

func NoteToBody(blocks []api.Block) []api.Block {
	out := make([]api.Block, 0, len(blocks))
	for _, b := range blocks {
		if b.Type == "text" && strings.TrimSpace(b.Content) == "" {
			continue
		}
		b.ID = NewID()
		switch {
		case b.Type == "h1" || b.Type == "h2":
			b.Type = "h3"
		case isList(b):
			b.Depth++
		}
		out = append(out, b)
	}
	return out
}

func (b *Board) appendCard(col int, meta Meta) int {
	card := Card{Meta: meta, ID: NewID(), Done: col == len(b.Columns)-1 && len(b.Columns) > 1}
	b.Columns[col].Cards = append(b.Columns[col].Cards, card)
	return len(b.Columns[col].Cards) - 1
}

func (b *Board) SetCardText(col, row int, text string) {
	if !b.valid(col, row) {
		return
	}
	card := &b.Columns[col].Cards[row]
	meta := ParseMeta(text)
	meta.Number = card.Number
	meta.NoteID = card.NoteID
	card.Meta = meta
}

func (b *Board) SetCardBody(col, row int, body []api.Block) {
	if !b.valid(col, row) {
		return
	}
	b.Columns[col].Cards[row].Body = body
}

func (b *Board) SetCardNote(col, row int, noteID string) {
	if !b.valid(col, row) {
		return
	}
	card := &b.Columns[col].Cards[row]
	card.NoteID = noteID
	card.Body = nil
}

func (b Board) LinksTo(noteID string) []CardRef {
	var refs []CardRef
	for c, col := range b.Columns {
		for r, card := range col.Cards {
			if card.NoteID != "" && card.NoteID == noteID {
				refs = append(refs, CardRef{Column: c, Row: r, ColumnName: col.Name, Card: card})
			}
		}
	}
	return refs
}

type CardRef struct {
	Column     int
	Row        int
	ColumnName string
	Card       Card
}

func (m Meta) EditableText() string {
	m.Number = 0
	m.NoteID = ""
	return m.Text()
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
