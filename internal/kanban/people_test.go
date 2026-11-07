package kanban

import (
	"testing"

	"github.com/tridennote/cli/internal/api"
)

func TestPeopleHandles(t *testing.T) {
	people := People([]api.Member{
		{Name: "Harith", Email: "Harith.Iqbal@example.com"},
		{Name: "Sam A", Email: "sam@acme.com"},
		{Name: "Sam G", Email: "sam@gmail.com"},
		{Name: "Plus", Email: "a+b@x.io"},
	})
	want := []string{"harith.iqbal", "sam-acme", "sam-gmail", "a-b"}
	for i, p := range people {
		if p.Handle != want[i] {
			t.Fatalf("handle %d = %q, want %q", i, p.Handle, want[i])
		}
	}
	if got := ParseMeta("Ship @" + people[0].Handle).Assignee; got != "harith.iqbal" {
		t.Fatalf("handle should survive the @mention pattern, got %q", got)
	}
	if p, ok := FindPerson(people, "SAM-ACME"); !ok || p.Name != "Sam A" {
		t.Fatalf("FindPerson should be case-insensitive, got %+v %v", p, ok)
	}
	if _, ok := FindPerson(people, "designer"); ok {
		t.Fatal("free text should not match anyone")
	}
}
