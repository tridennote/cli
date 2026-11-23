package envfile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	text := strings.Join([]string{
		"# comment",
		"",
		"API_URL=https://example.com",
		"export TOKEN=abc123 # trailing",
		"EMPTY=",
		`SINGLE='keep \n # raw'`,
		`DOUBLE="line1\nline2 \"q\""`,
		`MULTI="first`,
		`second"`,
		"HASH=a#b",
		"API_URL=override",
		"not a line",
	}, "\r\n")
	want := []Var{
		{"TOKEN", "abc123"},
		{"EMPTY", ""},
		{"SINGLE", `keep \n # raw`},
		{"DOUBLE", "line1\nline2 \"q\""},
		{"MULTI", "first\nsecond"},
		{"HASH", "a#b"},
		{"API_URL", "override"},
	}
	if got := Parse(text); !reflect.DeepEqual(got, want) {
		t.Fatalf("Parse:\n got %#v\nwant %#v", got, want)
	}
}

func TestRender(t *testing.T) {
	vars := []Var{{"PLAIN", "abc"}, {"SPACED", `a "b" c`}, {"LINES", "x\ny"}}
	if got := Parse(Render(vars, FormatEnv)); !reflect.DeepEqual(got, vars) {
		t.Fatalf(".env round trip: %#v", got)
	}
	if got, want := Render(vars, FormatYAML), "PLAIN: \"abc\"\nSPACED: \"a \\\"b\\\" c\"\nLINES: \"x\\ny\"\n"; got != want {
		t.Fatalf("yaml: %q", got)
	}
	if got := Render(vars[:1], FormatTXT); got != "PLAIN=abc\n" {
		t.Fatalf("txt: %q", got)
	}
	var decoded map[string]string
	if err := json.Unmarshal([]byte(Render(vars, FormatJSON)), &decoded); err != nil || decoded["LINES"] != "x\ny" || len(decoded) != 3 {
		t.Fatalf("json: %v %v", decoded, err)
	}
	if Render(nil, FormatEnv) != "" || Render(nil, FormatJSON) != "{}\n" {
		t.Fatal("empty renders")
	}
}

func TestNames(t *testing.T) {
	cases := map[string]string{
		DownloadName("api/.env.production", FormatEnv): ".env.production",
		DownloadName(".env.local", FormatYAML):         "env.local.yml",
		DownloadName("secrets", FormatEnv):             "secrets.env",
		DownloadName("prod.env", FormatJSON):           "prod.json",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("got %q want %q", got, want)
		}
	}
	for name, want := range map[string]bool{"app/.env": true, ".env.local": true, "prod.env": true, "environment.go": false} {
		if LooksLikeEnvFile(name) != want {
			t.Errorf("LooksLikeEnvFile(%q) != %v", name, want)
		}
	}
	if f, ok := ParseFormat("yml"); !ok || f != FormatYAML {
		t.Error("ParseFormat yml")
	}
}

func TestCollect(t *testing.T) {
	dir := t.TempDir()
	root := dir + "/proj"
	for p, body := range map[string]string{
		"/proj/.env":                "A=1",
		"/proj/api/.env.production": "B=2",
		"/proj/api/main.go":         "package main",
		"/proj/node_modules/x/.env": "C=3",
	} {
		full := dir + p
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sources, skipped, err := Collect(root)
	if err != nil || skipped != 0 {
		t.Fatal(err, skipped)
	}
	var names []string
	for _, s := range sources {
		names = append(names, s.Name)
	}
	if want := []string{"proj/.env", "proj/api/.env.production"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("names %v", names)
	}
	single, _, err := Collect(root + "/.env")
	if err != nil || len(single) != 1 || single[0].Name != ".env" || single[0].Content != "A=1" {
		t.Fatalf("single %v %v", single, err)
	}
}
