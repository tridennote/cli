package tree

import (
	"path"
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
	// IsCSV files live in the tree beside notes and render as tables.
	IsCSV bool
	// AttachmentID points at a CSV's uploaded bytes.
	AttachmentID string
	Depth        int
	Parent       *Node
	Children     []*Node
	Expanded     bool

	// External files (drawings, uploads) can't be shown in the terminal and open in the browser.
	External bool
	Dir      string

	// Divider separates notes from external files in the sidebar and is never selectable.
	Divider bool
}

func (n *Node) Label() string {
	if n.IsFolder {
		return n.Name
	}
	if n.IsBoard {
		return kanban.DisplayName(n.Name)
	}
	if n.External || n.IsCSV {
		return n.Name
	}
	return n.Name + ".md"
}

func isMarkdown(n api.TreeNode) bool {
	return n.Kind == "file" && n.FileType != nil && *n.FileType == "note"
}

// isCSV reports an uploaded .csv whose bytes the terminal can fetch and show.
func isCSV(n api.TreeNode) bool {
	return n.Kind == "file" && n.FileType != nil && *n.FileType == "file" && n.PrimaryAttachmentID != nil &&
		strings.EqualFold(path.Ext(n.Name), ".csv")
}

func Build(rows []api.TreeNode) []*Node {
	byID := map[string]*Node{}
	for _, row := range rows {
		node := &Node{ID: row.ID, Name: row.Name}
		switch {
		case row.Kind == "folder":
			node.IsFolder = true
		case isMarkdown(row):
			node.IsBoard = kanban.IsBoardName(row.Name)
		case isCSV(row):
			node.IsCSV = true
			node.AttachmentID = *row.PrimaryAttachmentID
		default:
			continue
		}
		byID[row.ID] = node
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

// Externals lists the files the terminal can't show, flattened with their folder path.
func Externals(rows []api.TreeNode) []*Node {
	folders := map[string]api.TreeNode{}
	for _, row := range rows {
		if row.Kind == "folder" {
			folders[row.ID] = row
		}
	}
	dir := func(parentID *string) string {
		var parts []string
		for seen := 0; parentID != nil && seen < len(folders); seen++ {
			folder, ok := folders[*parentID]
			if !ok {
				break
			}
			parts = append([]string{folder.Name}, parts...)
			parentID = folder.ParentID
		}
		return strings.Join(parts, "/")
	}
	var out []*Node
	for _, row := range rows {
		if row.Kind != "file" || row.FileType == nil || *row.FileType == "note" || isCSV(row) {
			continue
		}
		name := row.Name
		if *row.FileType == "excalidraw" && !strings.HasSuffix(strings.ToLower(name), ".excalidraw") {
			name += ".excalidraw"
		}
		out = append(out, &Node{ID: row.ID, Name: name, External: true, Dir: dir(row.ParentID)})
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := strings.ToLower(out[i].Dir), strings.ToLower(out[j].Dir)
		if a != b {
			return a < b
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
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

// Rows is the sidebar: the visible note tree, then a divider and the external files.
func Rows(roots, externals []*Node) []*Node {
	out := Visible(roots)
	if len(externals) == 0 {
		return out
	}
	if len(out) > 0 {
		out = append(out, &Node{Divider: true})
	}
	return append(out, externals...)
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
