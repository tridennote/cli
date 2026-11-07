package kanban

import (
	"regexp"
	"strings"

	"github.com/tridennote/cli/internal/api"
)

type Person struct {
	Name   string
	Email  string
	Handle string
}

var (
	handleUnsafe = regexp.MustCompile(`[^\w.-]+`)
	handleEdges  = regexp.MustCompile(`^[.-]+|[.-]+$`)
)

func toHandle(text string) string {
	return strings.ToLower(handleEdges.ReplaceAllString(handleUnsafe.ReplaceAllString(text, "-"), ""))
}

func People(members []api.Member) []Person {
	local := make([]string, len(members))
	for i, m := range members {
		user, _, _ := strings.Cut(m.Email, "@")
		local[i] = toHandle(user)
		if local[i] == "" {
			local[i] = toHandle(m.Name)
		}
	}
	people := make([]Person, len(members))
	for i, m := range members {
		clashes := 0
		for _, h := range local {
			if h == local[i] {
				clashes++
			}
		}
		handle := local[i]
		if _, domain, ok := strings.Cut(m.Email, "@"); ok && clashes > 1 {
			first, _, _ := strings.Cut(domain, ".")
			if d := toHandle(first); d != "" {
				handle += "-" + d
			}
		}
		people[i] = Person{Name: m.Name, Email: m.Email, Handle: handle}
	}
	return people
}

func FindPerson(people []Person, handle string) (Person, bool) {
	key := strings.ToLower(handle)
	for _, p := range people {
		if p.Handle == key {
			return p, true
		}
	}
	return Person{}, false
}
