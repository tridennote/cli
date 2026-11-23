package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/tridennote/cli/internal/api"
)

func linearServer(t *testing.T, saved *[]api.Block) *api.Client {
	t.Helper()
	note, folder := "note", "f1"
	reply := func(w http.ResponseWriter, data any) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/workspaces", func(w http.ResponseWriter, r *http.Request) {
		team := "Swift"
		reply(w, []api.Workspace{{ID: "w1", Name: "Personal", IsDefault: true}, {ID: "w2", Name: "My Workspace", TeamName: &team}})
	})
	mux.HandleFunc("GET /api/workspaces/w2/tree", func(w http.ResponseWriter, r *http.Request) {
		reply(w, []api.TreeNode{
			{ID: folder, Kind: "folder", Name: "Specs"},
			{ID: "n1", Kind: "file", FileType: &note, Name: "File 4", ParentID: &folder},
			{ID: "n2", Kind: "file", FileType: &note, Name: "Todo"},
		})
	})
	mux.HandleFunc("GET /api/notes/n1", func(w http.ResponseWriter, r *http.Request) {
		reply(w, api.Note{Title: "File 4", Blocks: []api.Block{{ID: "b1", Type: "h1", Content: "Build it"}, {ID: "b2", Type: "text", Content: "Use the API."}}})
	})
	mux.HandleFunc("PUT /api/notes/n1", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Blocks []api.Block }
		_ = json.NewDecoder(r.Body).Decode(&body)
		*saved = body.Blocks
		reply(w, api.Note{Title: "File 4", Blocks: body.Blocks})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := api.New(srv.URL)
	client.Token, client.CookieName = "tok", "session"
	return client
}

func pipeInput(t *testing.T, text string) *os.File {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.WriteString(text)
	_ = w.Close()
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func TestLinearPrintsByLink(t *testing.T) {
	var saved []api.Block
	client := linearServer(t, &saved)
	var out, errOut bytes.Buffer
	if err := runLinear(client, []string{"my-workspace", "specs/file-4"}, pipeInput(t, ""), &out, &errOut); err != nil {
		t.Fatal(err, errOut.String())
	}
	if !strings.Contains(out.String(), "# Build it") || !strings.Contains(out.String(), "Use the API.") {
		t.Fatalf("expected markdown, got %q", out.String())
	}
	out.Reset()
	if err := runLinear(client, []string{"swift/my-workspace", "file-4"}, pipeInput(t, ""), &out, &errOut); err != nil || !strings.Contains(out.String(), "Build it") {
		t.Fatalf("team path and leaf slug should resolve: %v", err)
	}
	if err := runLinear(client, []string{"my-workspace", "nope"}, pipeInput(t, ""), &out, &errOut); err == nil || !strings.Contains(err.Error(), "--list") {
		t.Fatalf("unknown note should point at --list, got %v", err)
	}
}

func TestLinearStepByStep(t *testing.T) {
	var saved []api.Block
	client := linearServer(t, &saved)
	var out, errOut bytes.Buffer
	if err := runLinear(client, nil, pipeInput(t, "9\n2\n1\n"), &out, &errOut); err != nil {
		t.Fatal(err, errOut.String())
	}
	prompts := errOut.String()
	for _, want := range []string{"1) Personal", "2) Swift / My Workspace", "enter a number from 1 to 2", "1) Specs/File 4.md", "2) Todo.md", "tridennote -c my-workspace specs/file-4"} {
		if !strings.Contains(prompts, want) {
			t.Fatalf("prompts missing %q:\n%s", want, prompts)
		}
	}
	if !strings.Contains(out.String(), "# Build it") {
		t.Fatalf("piped runs should print the note, got %q", out.String())
	}

	out.Reset()
	if err := runLinear(client, []string{"my-workspace", "--list"}, pipeInput(t, ""), &out, &errOut); err != nil || out.String() != "specs/file-4\ntodo\n" {
		t.Fatalf("--list got %q %v", out.String(), err)
	}
}

func TestLinearWrite(t *testing.T) {
	var saved []api.Block
	client := linearServer(t, &saved)
	var out, errOut bytes.Buffer
	err := runLinear(client, []string{"my-workspace", "file-4", "--write"}, pipeInput(t, "# Build it\n\nUse the API.\n\n- [ ] ship\n"), &out, &errOut)
	if err != nil {
		t.Fatal(err)
	}
	if len(saved) != 3 || saved[0].ID != "b1" || saved[2].Type != "todo" || saved[2].Content != "ship" {
		t.Fatalf("write should reconcile blocks, got %+v", saved)
	}
}
