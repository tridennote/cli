package tui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/tridennote/cli/internal/api"
	"github.com/tridennote/cli/internal/envfile"
)

func envServer(t *testing.T) (Model, *httptest.Server, *[]api.EnvUpload) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	var uploads []api.EnvUpload
	files := []api.EnvFileSummary{
		{ID: "e1", Name: ".env", VarCount: 3, UpdatedAt: "2026-11-20T00:00:00Z"},
		{ID: "e2", Name: "api/.env.production", VarCount: 1, UpdatedAt: "2026-11-21T00:00:00Z"},
	}
	vars := map[string][]envfile.Var{
		"e1": {{Key: "API_URL", Value: "https://example.com"}, {Key: "SECRET_KEY", Value: "hunter2"}, {Key: "PORT", Value: "3000"}},
		"e2": {{Key: "DB_URL", Value: "postgres://prod"}},
	}
	reply := func(w http.ResponseWriter, data any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/workspaces/w1/env-files", func(w http.ResponseWriter, r *http.Request) { reply(w, files) })
	mux.HandleFunc("GET /api/workspaces/w1/env-files/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		for _, f := range files {
			if f.ID == id {
				reply(w, api.EnvFile{ID: id, Name: f.Name, Vars: vars[id]})
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("POST /api/workspaces/w1/env-files", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Files []api.EnvUpload `json:"files"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		uploads = append(uploads, body.Files...)
		var saved []string
		for _, u := range body.Files {
			id := "new-" + u.Name
			files = append(files, api.EnvFileSummary{ID: id, Name: u.Name, VarCount: len(envfile.Parse(u.Content))})
			vars[id] = envfile.Parse(u.Content)
			saved = append(saved, id)
		}
		reply(w, api.EnvUploadResult{Files: files, SavedIDs: saved})
	})
	mux.Handle("/", (&fakeServer{}).handler(t))
	srv := httptest.NewServer(mux)
	client := api.New(srv.URL)
	client.Token = token
	client.CookieName = cookieName
	m := New(client)
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 24})
	m, cmd := step(t, m, m.checkSession()())
	m, cmd = step(t, m, cmd())
	m, _ = step(t, m, cmd())
	return m, srv, &uploads
}

func TestEnvVault(t *testing.T) {
	m, srv, uploads := envServer(t)
	defer srv.Close()
	var copied string
	original := copyToClipboard
	copyToClipboard = func(text string) error { copied = text; return nil }
	defer func() { copyToClipboard = original }()

	m, cmd := step(t, m, key("E"))
	if m.mode != modeEnv || cmd == nil {
		t.Fatalf("E should open the env vault")
	}
	m, cmd = step(t, m, cmd())
	m, _ = step(t, m, cmd())
	view := ansi.Strip(m.View())
	for _, want := range []string{"Env vault", ".env", "api/.env.production", "API_URL", envMask} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "hunter2") {
		t.Fatalf("values should start hidden:\n%s", view)
	}

	m, _ = step(t, m, key("enter"))
	m, _ = step(t, m, key("j"))
	m, _ = step(t, m, key("s"))
	if !strings.Contains(ansi.Strip(m.View()), "hunter2") {
		t.Fatalf("s should reveal the current value")
	}
	m, _ = step(t, m, key("y"))
	if copied != "hunter2" {
		t.Fatalf("y should copy the value, got %q", copied)
	}

	m, _ = step(t, m, key("k"))
	m, _ = step(t, m, key(" "))
	m, _ = step(t, m, key(" "))
	m, _ = step(t, m, key("c"))
	if m.env.prompt != envPromptCopy {
		t.Fatalf("c should ask for a format")
	}
	m, _ = step(t, m, key("2"))
	if copied != "API_URL: \"https://example.com\"\nSECRET_KEY: \"hunter2\"\n" {
		t.Fatalf("copy as yaml got %q", copied)
	}

	dir := t.TempDir()
	wd, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer func() { _ = os.Chdir(wd) }()
	m, _ = step(t, m, key("d"))
	m, _ = step(t, m, key("1"))
	raw, err := os.ReadFile(".env")
	if err != nil || string(raw) != "API_URL=https://example.com\nSECRET_KEY=hunter2\n" {
		t.Fatalf("download wrote %q, %v", raw, err)
	}
	if info, _ := os.Stat(".env"); info.Mode().Perm() != 0o600 {
		t.Fatalf("downloads should be private, got %v", info.Mode().Perm())
	}
	m, _ = step(t, m, key("d"))
	m, _ = step(t, m, key("1"))
	if m.env.prompt != envPromptOverwrite {
		t.Fatalf("an existing file should need confirmation")
	}
	m, _ = step(t, m, key("n"))

	if err := os.WriteFile("staging.env", []byte("NEW_KEY=1\nOTHER=2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m, _ = step(t, m, key("i"))
	m = typeText(t, m, "staging.env")
	m, cmd = step(t, m, key("enter"))
	m, cmd = step(t, m, cmd())
	if len(*uploads) != 1 || (*uploads)[0].Name != "staging.env" {
		t.Fatalf("import should upload the file, got %+v", *uploads)
	}
	m, _ = step(t, m, cmd())
	if m.env.file == nil || m.env.file.Name != "staging.env" || !strings.Contains(m.flash, "Saved 1 env file") {
		t.Fatalf("the imported file should be selected, got %+v flash %q", m.env.file, m.flash)
	}

	m, _ = step(t, m, key("q"))
	if m.mode != modeBrowse {
		t.Fatalf("q should leave the env vault")
	}
}
