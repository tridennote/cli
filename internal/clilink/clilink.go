// Package clilink turns workspace and note names into the slugs used by `tridennote -c`.
// The web app's src/shared/cli-link.ts must produce identical slugs.
package clilink

import (
	"strings"
	"unicode"

	"github.com/tridennote/cli/internal/api"
)

func Slug(name string) string {
	name = strings.TrimSuffix(strings.TrimSpace(name), ".md")
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			dash = false
			continue
		}
		if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}

type Note struct {
	ID   string
	Name string
	Path string
	Slug string
}

// Notes lists the workspace's markdown notes with readable paths ("folder/note.md") and slugs ("folder/note").
func Notes(rows []api.TreeNode) []Note {
	byID := map[string]api.TreeNode{}
	for _, r := range rows {
		byID[r.ID] = r
	}
	var out []Note
	for _, r := range rows {
		if r.Kind != "file" || r.FileType == nil || *r.FileType != "note" {
			continue
		}
		names, slugs := []string{r.Name + ".md"}, []string{Slug(r.Name)}
		for parent := r.ParentID; parent != nil; {
			p, ok := byID[*parent]
			if !ok {
				break
			}
			names = append([]string{p.Name}, names...)
			slugs = append([]string{Slug(p.Name)}, slugs...)
			parent = p.ParentID
		}
		out = append(out, Note{ID: r.ID, Name: r.Name, Path: strings.Join(names, "/"), Slug: strings.Join(slugs, "/")})
	}
	return out
}

func normalizeQuery(q string) string {
	parts := strings.Split(strings.Trim(strings.TrimSpace(q), "/"), "/")
	for i, p := range parts {
		parts[i] = Slug(p)
	}
	return strings.Join(parts, "/")
}

// FindNote matches a note by id, full slug path, or a unique trailing slug ("note" or "folder/note").
func FindNote(notes []Note, query string) (Note, []Note) {
	for _, n := range notes {
		if n.ID == query {
			return n, nil
		}
	}
	q := normalizeQuery(query)
	var suffix []Note
	for _, n := range notes {
		if n.Slug == q {
			return n, nil
		}
		if strings.HasSuffix(n.Slug, "/"+q) {
			suffix = append(suffix, n)
		}
	}
	if len(suffix) == 1 {
		return suffix[0], nil
	}
	return Note{}, suffix
}

// FindWorkspace matches by id, slug of the name, or "team/name".
func FindWorkspace(workspaces []api.Workspace, query string) (api.Workspace, bool) {
	q := normalizeQuery(query)
	for _, w := range workspaces {
		if w.ID == query || Slug(w.Name) == q {
			return w, true
		}
		if w.TeamName != nil && Slug(*w.TeamName)+"/"+Slug(w.Name) == q {
			return w, true
		}
	}
	return api.Workspace{}, false
}

func WorkspaceSlug(w api.Workspace, all []api.Workspace) string {
	slug := Slug(w.Name)
	for _, other := range all {
		if other.ID != w.ID && Slug(other.Name) == slug && w.TeamName != nil {
			return Slug(*w.TeamName) + "/" + slug
		}
	}
	return slug
}
