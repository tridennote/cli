package diagram

import (
	"errors"
	"html/template"
	"os"
	"path/filepath"
)

type Page struct {
	Title    string
	Diagrams []PageDiagram
}

type PageDiagram struct {
	Label  string
	Source string
}

var pageTemplate = template.Must(template.New("page").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}} · diagrams</title>
<style>
  body { margin: 0; padding: 32px 16px; background: #16161e; color: #c0caf5; font: 15px/1.5 system-ui, sans-serif; }
  main { max-width: 1100px; margin: 0 auto; }
  h1 { font-size: 20px; font-weight: 600; margin: 0 0 24px; }
  section { background: #1a1b26; border: 1px solid #292e42; border-radius: 10px; padding: 20px; margin-bottom: 24px; overflow-x: auto; }
  h2 { font-size: 13px; font-weight: 500; color: #9aa5ce; margin: 0 0 16px; text-transform: uppercase; letter-spacing: .04em; }
  pre.mermaid { margin: 0; display: flex; justify-content: center; }
</style>
</head>
<body>
<main>
<h1>{{.Title}}</h1>
{{range .Diagrams}}<section>
<h2>{{.Label}}</h2>
<pre class="mermaid">{{.Source}}</pre>
</section>
{{end}}</main>
<script type="module">
  import mermaid from "https://cdn.jsdelivr.net/npm/mermaid@11/dist/mermaid.esm.min.mjs";
  mermaid.initialize({ startOnLoad: true, theme: "dark", securityLevel: "strict" });
</script>
</body>
</html>
`))

// WriteHTML writes a page that renders the diagrams with mermaid.js and returns its path.
func WriteHTML(page Page) (string, error) {
	if len(page.Diagrams) == 0 {
		return "", errors.New("no diagrams to show")
	}
	dir := filepath.Join(os.TempDir(), "tridennote-mermaid")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "diagrams.html")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return "", err
	}
	if err := pageTemplate.Execute(f, page); err != nil {
		f.Close()
		return "", err
	}
	return path, f.Close()
}
