package diagram

import (
	"os"
	"strings"
	"testing"
)

func TestWriteHTMLEscapesSources(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	path, err := WriteHTML(Page{Title: "Flow", Diagrams: []PageDiagram{
		{Label: "Login · flowchart", Source: "graph TD\n  A[<b>x</b>] --> B"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	for _, want := range []string{`<pre class="mermaid">graph TD`, "A[&lt;b&gt;x&lt;/b&gt;] --&gt; B", "Login · flowchart", "mermaid.initialize"} {
		if !strings.Contains(out, want) {
			t.Fatalf("page missing %q:\n%s", want, out)
		}
	}
	if _, err := WriteHTML(Page{}); err == nil {
		t.Fatal("empty page should fail")
	}
}
