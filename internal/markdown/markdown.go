package markdown

import (
	"fmt"
	"strings"

	"github.com/tridennote/cli/internal/api"
)

var chainingTypes = map[string]bool{"bulleted": true, "numbered": true, "todo": true}

func blockLines(b api.Block, numberedIndex int) []string {
	indent := strings.Repeat("  ", b.Depth)
	switch b.Type {
	case "h1":
		return []string{"# " + b.Content}
	case "h2":
		return []string{"## " + b.Content}
	case "h3":
		return []string{"### " + b.Content}
	case "h4":
		return []string{"#### " + b.Content}
	case "bulleted":
		return []string{indent + "- " + b.Content}
	case "numbered":
		return []string{fmt.Sprintf("%s%d. %s", indent, numberedIndex, b.Content)}
	case "todo":
		mark := " "
		if b.Checked != nil && *b.Checked {
			mark = "x"
		}
		return []string{fmt.Sprintf("%s- [%s] %s", indent, mark, b.Content)}
	case "quote":
		return []string{"> " + b.Content}
	case "code":
		lang := ""
		if b.Language != nil && *b.Language != "plaintext" {
			lang = *b.Language
		}
		return []string{"```" + lang, b.Content, "```"}
	case "mermaid":
		return []string{"```mermaid", b.Content, "```"}
	case "table":
		return strings.Split(b.Content, "\n")
	default:
		return []string{b.Content}
	}
}

func FromBlocks(blocks []api.Block) string {
	var lines []string
	counters := map[int]int{}
	prevType := ""
	for i, b := range blocks {
		numberedIndex := 0
		if b.Type == "numbered" {
			counters[b.Depth]++
			for depth := range counters {
				if depth > b.Depth {
					delete(counters, depth)
				}
			}
			numberedIndex = counters[b.Depth]
		} else {
			clear(counters)
		}
		sameChain := prevType == b.Type && chainingTypes[b.Type]
		if i > 0 && !sameChain {
			lines = append(lines, "")
		}
		lines = append(lines, blockLines(b, numberedIndex)...)
		prevType = b.Type
	}
	return strings.Join(lines, "\n") + "\n"
}
