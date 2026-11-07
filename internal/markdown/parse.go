package markdown

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/tridennote/cli/internal/api"
)

var (
	fencePattern    = regexp.MustCompile("^```(\\S*)")
	rulePattern     = regexp.MustCompile(`^(-{3,}|\*{3,}|_{3,})$`)
	headingPattern  = regexp.MustCompile(`^(#{1,6})\s+(.*)$`)
	todoPattern     = regexp.MustCompile(`^[-*]\s+\[([ xX])\]\s+(.*)$`)
	bulletPattern   = regexp.MustCompile(`^[-*]\s+(.*)$`)
	numberedPattern = regexp.MustCompile(`^\d+\.\s+(.*)$`)
	quotePattern    = regexp.MustCompile(`^(\s*>\s?)+`)
)

func indentWidth(line string) int {
	leading := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	return len(strings.ReplaceAll(leading, "\t", "  "))
}

func pushIndent(stack *[]int, width int) int {
	for len(*stack) > 0 && width <= (*stack)[len(*stack)-1] {
		*stack = (*stack)[:len(*stack)-1]
	}
	*stack = append(*stack, width)
	return len(*stack) - 1
}

func ToBlocks(text string) []api.Block {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var blocks []api.Block
	var listStack []int

	for i := 0; i < len(lines); {
		raw := lines[i]
		trimmed := strings.TrimSpace(raw)

		if trimmed == "" {
			i++
			continue
		}

		if fence := fencePattern.FindStringSubmatch(trimmed); fence != nil {
			listStack = nil
			language := fence[1]
			if language == "" {
				language = "plaintext"
			}
			var code []string
			i++
			for i < len(lines) && strings.TrimSpace(lines[i]) != "```" {
				code = append(code, lines[i])
				i++
			}
			i++
			if language == "mermaid" {
				blocks = append(blocks, api.Block{Type: "mermaid", Content: strings.Join(code, "\n")})
			} else {
				lang := language
				blocks = append(blocks, api.Block{Type: "code", Content: strings.Join(code, "\n"), Language: &lang})
			}
			continue
		}

		if rulePattern.MatchString(trimmed) {
			listStack = nil
			i++
			continue
		}

		if heading := headingPattern.FindStringSubmatch(trimmed); heading != nil {
			listStack = nil
			level := min(len(heading[1]), 4)
			blocks = append(blocks, api.Block{Type: fmt.Sprintf("h%d", level), Content: heading[2]})
			i++
			continue
		}

		if todo := todoPattern.FindStringSubmatch(trimmed); todo != nil {
			checked := strings.EqualFold(todo[1], "x")
			blocks = append(blocks, api.Block{Type: "todo", Content: todo[2], Checked: &checked, Depth: pushIndent(&listStack, indentWidth(raw))})
			i++
			continue
		}

		if bullet := bulletPattern.FindStringSubmatch(trimmed); bullet != nil {
			blocks = append(blocks, api.Block{Type: "bulleted", Content: bullet[1], Depth: pushIndent(&listStack, indentWidth(raw))})
			i++
			continue
		}

		if numbered := numberedPattern.FindStringSubmatch(trimmed); numbered != nil {
			blocks = append(blocks, api.Block{Type: "numbered", Content: numbered[1], Depth: pushIndent(&listStack, indentWidth(raw))})
			i++
			continue
		}

		if strings.HasPrefix(trimmed, ">") {
			listStack = nil
			blocks = append(blocks, api.Block{Type: "quote", Content: quotePattern.ReplaceAllString(trimmed, "")})
			i++
			continue
		}

		if strings.HasPrefix(trimmed, "|") {
			listStack = nil
			var table []string
			for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), "|") {
				table = append(table, strings.TrimSpace(lines[i]))
				i++
			}
			blocks = append(blocks, api.Block{Type: "table", Content: strings.Join(table, "\n")})
			continue
		}

		listStack = nil
		blocks = append(blocks, api.Block{Type: "text", Content: trimmed})
		i++
	}
	return blocks
}

var unparseableTypes = map[string]bool{"image": true}

func Reconcile(previous, next []api.Block) []api.Block {
	unused := map[string][]int{}
	for i, b := range previous {
		if b.ID != "" {
			unused[b.Content] = append(unused[b.Content], i)
		}
	}
	taken := map[int]bool{}
	out := make([]api.Block, len(next))
	for i, b := range next {
		out[i] = b
		match := -1
		for _, idx := range unused[b.Content] {
			if taken[idx] {
				continue
			}
			if previous[idx].Type == b.Type {
				match = idx
				break
			}
			if match == -1 && (unparseableTypes[previous[idx].Type] && b.Type == "text") {
				match = idx
			}
		}
		if match == -1 {
			continue
		}
		taken[match] = true
		out[i].ID = previous[match].ID
		if unparseableTypes[previous[match].Type] {
			out[i].Type = previous[match].Type
		}
	}
	return out
}
