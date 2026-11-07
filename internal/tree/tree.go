package tree

import (
	"sort"
	"strings"

	"github.com/tridennote/cli/internal/api"
	"github.com/tridennote/cli/internal/kanban"
)

type Node struct {
	ID       string
	Name     string
	IsFolder bool
	IsBoard  bool
	Depth    int
	Parent   *Node
	Children []*Node
	Expanded bool
}

func (n *Node) Label() string {
	if n.IsFolder {
		return n.Name
	}
	if n.IsBoard {
		return kanban.DisplayName(n.Name)
	}
	return n.Name + ".md"
}

func isMarkdown(n api.TreeNode) bool {
	return n.Kind == "file" && n.FileType != nil && *n.FileType == "note"
}

func Build(rows []api.TreeNode) []*Node {
	byID := map[string]*Node{}
	for _, row := range rows {
		if row.Kind != "folder" && !isMarkdown(row) {
			continue
		}
		folder := row.Kind == "folder"
		byID[row.ID] = &Node{ID: row.ID, Name: row.Name, IsFolder: folder, IsBoard: !folder && kanban.IsBoardName(row.Name)}
	}
	var roots []*Node
	for _, row := range rows {
		node, ok := byID[row.ID]
		if !ok {
			continue
		}
		if row.ParentID != nil {
			if parent, ok := byID[*row.ParentID]; ok {
				node.Parent = parent
				parent.Children = append(parent.Children, node)
				continue
			}
		}
		roots = append(roots, node)
	}
	sortNodes(roots, 0)
	return roots
}

func sortNodes(nodes []*Node, depth int) {
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].IsFolder != nodes[j].IsFolder {
			return nodes[i].IsFolder
		}
		return strings.ToLower(nodes[i].Name) < strings.ToLower(nodes[j].Name)
	})
	for _, n := range nodes {
		n.Depth = depth
		sortNodes(n.Children, depth+1)
	}
}

func Visible(roots []*Node) []*Node {
	var out []*Node
	var walk func([]*Node)
	walk = func(nodes []*Node) {
		for _, n := range nodes {
			out = append(out, n)
			if n.IsFolder && n.Expanded {
				walk(n.Children)
			}
		}
	}
	walk(roots)
	return out
}

func ExpandedIDs(roots []*Node) map[string]bool {
	ids := map[string]bool{}
	var walk func([]*Node)
	walk = func(nodes []*Node) {
		for _, n := range nodes {
			if n.Expanded {
				ids[n.ID] = true
			}
			walk(n.Children)
		}
	}
	walk(roots)
	return ids
}

func Restore(roots []*Node, expanded map[string]bool) {
	var walk func([]*Node)
	walk = func(nodes []*Node) {
		for _, n := range nodes {
			n.Expanded = expanded[n.ID]
			walk(n.Children)
		}
	}
	walk(roots)
}
