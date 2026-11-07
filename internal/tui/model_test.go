package tui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/tridennote/cli/internal/api"
	"github.com/tridennote/cli/internal/session"
	"github.com/tridennote/cli/internal/update"
)

const (
	cookieName = "__Secure-neon-auth.session_token"
	token      = "session-abc"
	specNoteID = "0f8fad5b-d9cb-469f-a165-70867728950e"
	newNoteID  = "6ba7b810-9dad-41d1-80b4-00c04fd430c8"
)

type fakeServer struct {
	mu         sync.Mutex
	authorized bool
	saved      []api.Block
	created    map[string]any
	board      bool
	csv        bool
	teams      bool
	nextID     string
}

func (f *fakeServer) handler(t *testing.T) http.Handler {
	mux := http.NewServeMux()
	reply := func(w http.ResponseWriter, data any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}
	authed := func(r *http.Request) bool {
		c, err := r.Cookie(cookieName)
		return err == nil && c.Value == token
	}
	mux.HandleFunc("POST /api/device-auth/init", func(w http.ResponseWriter, r *http.Request) {
		reply(w, api.DeviceCode{DeviceCode: "dev-1", UserCode: "WXYZ-2345", VerificationURL: "https://app.tridennote.pixcel.org/#/activate?code=WXYZ-2345", Interval: 3})
	})
	mux.HandleFunc("POST /api/device-auth/poll", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.authorized {
			reply(w, api.DevicePoll{Status: "authorized", SessionToken: token, CookieName: cookieName})
			return
		}
		reply(w, api.DevicePoll{Status: "pending"})
	})
	mux.HandleFunc("GET /api/auth/get-session", func(w http.ResponseWriter, r *http.Request) {
		if !authed(r) {
			_, _ = w.Write([]byte("null"))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"user": api.User{ID: "u1", Email: "harith@example.com"}})
	})
	mux.HandleFunc("GET /api/workspaces", func(w http.ResponseWriter, r *http.Request) {
		if !authed(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		workspaces := []api.Workspace{{ID: "w1", Name: "Personal", IsDefault: true}}
		if f.teams {
			workspaces = append(workspaces, api.Workspace{ID: "w2", Name: "Swift Ops"}, api.Workspace{ID: "w3", Name: "Side projects"})
		}
		reply(w, workspaces)
	})
	mux.HandleFunc("GET /api/workspaces/{id}/tree", func(w http.ResponseWriter, r *http.Request) {
		note := "note"
		reply(w, []api.TreeNode{{ID: r.PathValue("id") + "-n", Kind: "file", FileType: &note, Name: "Inbox"}})
	})
	mux.HandleFunc("GET /api/workspaces/w1/tree", func(w http.ResponseWriter, r *http.Request) {
		note, sketch, pdf := "note", "excalidraw", "file"
		folder := "f1"
		rows := []api.TreeNode{
			{ID: "n1", Kind: "file", FileType: &note, Name: "Daily log"},
			{ID: "f1", Kind: "folder", Name: "Projects"},
			{ID: "n2", Kind: "file", FileType: &note, Name: "Roadmap", ParentID: &folder},
			{ID: "x1", Kind: "file", FileType: &sketch, Name: "Whiteboard.excalidraw", ParentID: &folder},
			{ID: "p1", Kind: "file", FileType: &pdf, Name: "invoice.pdf"},
		}
		if f.csv {
			attachment := "a1"
			rows = append(rows, api.TreeNode{ID: "c1", Kind: "file", FileType: &pdf, Name: "yearly.csv", PrimaryAttachmentID: &attachment})
		}
		if f.board {
			rows = append(rows,
				api.TreeNode{ID: "k1", Kind: "file", FileType: &note, Name: "Sprint.kanban"},
				api.TreeNode{ID: specNoteID, Kind: "file", FileType: &note, Name: "Spec"},
			)
		}
		reply(w, rows)
	})
	mux.HandleFunc("GET /api/attachments/a1/download", func(w http.ResponseWriter, r *http.Request) {
		if !authed(r) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write([]byte(fakeCSV()))
	})
	mux.HandleFunc("GET /api/notes/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") == "k1" {
			reply(w, api.Note{Title: "Sprint.kanban", Blocks: sprintBlocks()})
			return
		}
		reply(w, api.Note{Title: "Roadmap", Blocks: []api.Block{
			{ID: "b1", Type: "h2", Content: "Q4"},
			{ID: "b2", Type: "bulleted", Content: "Ship the terminal client"},
			{ID: "b3", Type: "bulleted", Content: "Neovim-style tree"},
		}})
	})
	mux.HandleFunc("PUT /api/notes/{id}", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Blocks []api.Block `json:"blocks"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.saved = body.Blocks
		f.mu.Unlock()
		out := make([]api.Block, len(body.Blocks))
		for i, b := range body.Blocks {
			out[i] = b
			if out[i].ID == "" {
				out[i].ID = "new-" + b.Content
			}
		}
		title := "Roadmap"
		if r.PathValue("id") == "k1" {
			title = "Sprint.kanban"
		}
		reply(w, api.Note{Title: title, Blocks: out})
	})
	mux.HandleFunc("POST /api/workspaces/w1/nodes", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.created = body
		f.mu.Unlock()
		note := "note"
		id := "n9"
		if f.nextID != "" {
			id = f.nextID
		}
		w.WriteHeader(http.StatusCreated)
		reply(w, api.TreeNode{ID: id, Kind: "file", FileType: &note, Name: body["name"].(string)})
	})
	mux.HandleFunc("GET /api/workspaces/w1/members", func(w http.ResponseWriter, r *http.Request) {
		reply(w, []api.Member{
			{UserID: "u1", Name: "Harith Iqbal", Email: "harith@example.com"},
			{UserID: "u2", Name: "Ana Bell", Email: "ana@example.com"},
		})
	})
	mux.HandleFunc("GET /api/workspaces/w1/tags", func(w http.ResponseWriter, r *http.Request) {
		reply(w, []string{"qa", "release"})
	})
	return mux
}

func step(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	return next.(Model), cmd
}

func key(k string) tea.KeyMsg {
	switch k {
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "right":
		return tea.KeyMsg{Type: tea.KeyRight}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
}

func snapshot(t *testing.T, name string, m Model) string {
	t.Helper()
	view := ansi.Strip(m.View())
	if dir := os.Getenv("TUI_SNAPSHOT_DIR"); dir != "" {
		_ = os.WriteFile(filepath.Join(dir, name+".txt"), []byte(view), 0o644)
	}
	return view
}

func TestSignInAndBrowse(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	fake := &fakeServer{}
	srv := httptest.NewServer(fake.handler(t))
	defer srv.Close()

	m := New(api.New(srv.URL))
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 24})

	m, cmd := step(t, m, m.checkSession()())
	if m.state != stateLogin {
		t.Fatalf("expected login state, got %v", m.state)
	}
	m, _ = step(t, m, cmd())
	if m.device == nil || m.device.UserCode != "WXYZ-2345" {
		t.Fatalf("device code not shown: %+v", m.device)
	}
	if view := snapshot(t, "1-login", m); !strings.Contains(view, "WXYZ-2345") || !strings.Contains(view, "not signed in") {
		t.Fatalf("login view missing prompt:\n%s", view)
	}

	m, _ = step(t, m, m.poll("dev-1")())
	if m.state != stateLogin {
		t.Fatalf("pending poll should stay on login")
	}

	fake.authorized = true
	m, cmd = step(t, m, m.poll("dev-1")())
	saved, err := session.Load()
	if err != nil || saved == nil || saved.Token != token || saved.CookieName != cookieName {
		t.Fatalf("session not persisted: %+v %v", saved, err)
	}
	m, cmd = step(t, m, cmd())
	m, cmd = step(t, m, cmd())
	m, _ = step(t, m, cmd())
	if m.state != stateBrowse {
		t.Fatalf("expected browse state, got %v err=%v", m.state, m.err)
	}

	view := snapshot(t, "2-tree", m)
	for _, want := range []string{"Projects/", "Daily log.md", "harith@example.com"} {
		if !strings.Contains(view, want) {
			t.Fatalf("tree missing %q:\n%s", want, view)
		}
	}
	divider := strings.Index(view, "─ in browser")
	if divider < 0 || strings.Index(view, "Daily log.md") > divider {
		t.Fatalf("notes should sit above the browser divider:\n%s", view)
	}
	for _, external := range []string{"invoice.pdf", "Whiteboard.excalidraw  Proj"} {
		if i := strings.Index(view, external); i < divider {
			t.Fatalf("%q should be listed below the divider:\n%s", external, view)
		}
	}

	m, _ = step(t, m, key("l"))
	if !m.visible[0].Expanded {
		t.Fatalf("l should expand folder")
	}
	m, _ = step(t, m, key("down"))
	if m.selected().ID != "n2" {
		t.Fatalf("down should land on Roadmap, got %s", m.selected().ID)
	}
	m, cmd = step(t, m, key("enter"))
	m, _ = step(t, m, cmd())
	view = snapshot(t, "3-note", m)
	if !strings.Contains(view, "Ship the terminal client") {
		t.Fatalf("note content not rendered:\n%s", view)
	}

	if m.focus != focusTree {
		t.Fatalf("opening a note should keep focus in the tree")
	}
	m, _ = step(t, m, key("down"))
	if m.selected().ID != "n1" {
		t.Fatalf("down after opening should keep moving through the tree, got %s", m.selected().ID)
	}
	m, _ = step(t, m, key("down"))
	if m.selected().ID != "p1" {
		t.Fatalf("down should skip the divider into browser files, got %s", m.selected().ID)
	}
	m, _ = step(t, m, key("down"))
	m, _ = step(t, m, key("down"))
	if m.selected().ID != "f1" {
		t.Fatalf("down at the bottom should wrap to the top, got %s", m.selected().ID)
	}
	m, _ = step(t, m, key("up"))
	if m.selected().ID != "x1" {
		t.Fatalf("up at the top should wrap to the bottom, got %s", m.selected().ID)
	}
	for range 3 {
		m, _ = step(t, m, key("up"))
	}
	if m.selected().ID != "n2" {
		t.Fatalf("up should walk back over the divider to Roadmap, got %s", m.selected().ID)
	}
	m, _ = step(t, m, key("h"))
	if m.selected().ID != "f1" || m.visible[0].Expanded {
		t.Fatalf("left on a child should collapse parent and select it")
	}

	m, _ = step(t, m, key("L"))
	if m.state != stateLogin {
		t.Fatalf("logout should return to login")
	}
	if saved, _ := session.Load(); saved != nil {
		t.Fatalf("logout should clear saved session")
	}
}

func TestExpiredSessionPromptsLogin(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	srv := httptest.NewServer((&fakeServer{}).handler(t))
	defer srv.Close()

	client := api.New(srv.URL)
	client.Token = "revoked"
	client.CookieName = cookieName
	m := New(client)
	m, _ = step(t, m, m.checkSession()())
	if m.state != stateLogin {
		t.Fatalf("revoked session should prompt login, got %v", m.state)
	}
}

func signedIn(t *testing.T, fake *fakeServer) (Model, *httptest.Server) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	srv := httptest.NewServer(fake.handler(t))
	client := api.New(srv.URL)
	client.Token = token
	client.CookieName = cookieName
	m := New(client)
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 24})
	m, cmd := step(t, m, m.checkSession()())
	m, cmd = step(t, m, cmd())
	m, _ = step(t, m, cmd())
	if m.state != stateBrowse {
		t.Fatalf("expected browse state, got %v err=%v", m.state, m.err)
	}
	return m, srv
}

func typeText(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, r := range s {
		if r == '\n' {
			m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyEnter})
			continue
		}
		m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m
}

func TestEditExistingNote(t *testing.T) {
	fake := &fakeServer{}
	m, srv := signedIn(t, fake)
	defer srv.Close()

	m, _ = step(t, m, key("l"))
	m, _ = step(t, m, key("down"))
	m, cmd := step(t, m, key("e"))
	m, cmd = step(t, m, cmd())
	if m.mode != modeEdit || m.editID != "n2" {
		t.Fatalf("expected editor for n2, mode=%v id=%q", m.mode, m.editID)
	}
	if !strings.Contains(m.editor.Value(), "- Ship the terminal client") {
		t.Fatalf("editor should start with note markdown:\n%s", m.editor.Value())
	}

	m.editor.CursorEnd()
	for m.editor.Line() < m.editor.LineCount()-1 {
		m.editor.CursorDown()
	}
	m.editor.CursorEnd()
	m = typeText(t, m, "\n- Edit notes from the terminal")
	if !m.dirty() {
		t.Fatalf("typing should mark the editor dirty")
	}
	snapshot(t, "4-editor", m)

	m, _ = step(t, m, key("esc"))
	if m.mode != modeEdit || !m.confirmDiscard {
		t.Fatalf("first esc with unsaved changes should warn, not close")
	}

	m, cmd = step(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	m, _ = step(t, m, cmd())
	if m.dirty() || !strings.HasSuffix(m.flash, "Saved") {
		t.Fatalf("save should clear dirty state, flash=%q err=%v", m.flash, m.err)
	}
	if len(fake.saved) != 4 {
		t.Fatalf("expected 4 blocks saved, got %+v", fake.saved)
	}
	if fake.saved[0].ID != "b1" || fake.saved[1].ID != "b2" || fake.saved[3].ID != "" {
		t.Fatalf("unchanged blocks should keep ids, new block should not: %+v", fake.saved)
	}
	if fake.saved[3].Content != "Edit notes from the terminal" || fake.saved[3].Type != "bulleted" {
		t.Fatalf("new block parsed wrong: %+v", fake.saved[3])
	}

	m, _ = step(t, m, key("esc"))
	if m.mode != modeBrowse {
		t.Fatalf("esc after saving should close the editor")
	}
	if view := snapshot(t, "5-after-save", m); !strings.Contains(view, "Edit notes from the terminal") {
		t.Fatalf("viewer should show saved content:\n%s", view)
	}
}

func TestCreateNoteInFolder(t *testing.T) {
	fake := &fakeServer{}
	m, srv := signedIn(t, fake)
	defer srv.Close()

	m, _ = step(t, m, key("n"))
	if m.mode != modeNewNote || m.newParent == nil || m.newParent.ID != "f1" {
		t.Fatalf("n on a folder should prompt for a note inside it")
	}
	m = typeText(t, m, "Ideas.md")
	snapshot(t, "6-new-note", m)
	m, cmd := step(t, m, key("enter"))
	m, cmd = step(t, m, cmd())
	if fake.created["parentId"] != "f1" || fake.created["name"] != "Ideas" || fake.created["fileType"] != "note" {
		t.Fatalf("unexpected create payload: %+v", fake.created)
	}
	if m.mode != modeEdit || m.editID != "n9" {
		t.Fatalf("new note should open in the editor, mode=%v id=%q", m.mode, m.editID)
	}
	_ = cmd
}

func TestExternalEditorSavesChanges(t *testing.T) {
	fake := &fakeServer{}
	m, srv := signedIn(t, fake)
	defer srv.Close()

	m, _ = step(t, m, key("l"))
	m, _ = step(t, m, key("down"))
	m, cmd := step(t, m, key("enter"))
	m, _ = step(t, m, cmd())

	path := filepath.Join(t.TempDir(), "Roadmap.md")
	baseline, _ := m.noteText("n2")
	if err := os.WriteFile(path, []byte(baseline+"\n- Written in VS Code\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m, cmd = step(t, m, externalDoneMsg{id: "n2", path: path, baseline: baseline})
	if cmd == nil {
		t.Fatalf("changed file should trigger a save")
	}
	m, _ = step(t, m, cmd())
	if len(fake.saved) != 4 || fake.saved[3].Content != "Written in VS Code" {
		t.Fatalf("external edit not saved: %+v", fake.saved)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("temp file should be removed")
	}
}

func TestEditorCommandAddsWaitFlag(t *testing.T) {
	t.Setenv("TRIDENNOTE_EDITOR", "")
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "code")
	if got := strings.Join(systemEditorCommand(), " "); got != "code --wait" {
		t.Fatalf("got %q", got)
	}
	t.Setenv("EDITOR", "nvim")
	if got := strings.Join(systemEditorCommand(), " "); got != "nvim" {
		t.Fatalf("got %q", got)
	}
	t.Setenv("TRIDENNOTE_EDITOR", "/usr/local/bin/subl -n")
	if got := strings.Join(systemEditorCommand(), " "); got != "/usr/local/bin/subl -n --wait" {
		t.Fatalf("got %q", got)
	}
	const vscode = `C:\Users\me\AppData\Local\Programs\Microsoft VS Code\bin\code.cmd`
	t.Setenv("TRIDENNOTE_EDITOR", `"`+vscode+`"`)
	if got := systemEditorCommand(); len(got) != 2 || got[0] != vscode || got[1] != "--wait" {
		t.Fatalf("quoted code.cmd path should stay whole and get --wait, got %q", got)
	}
	t.Setenv("TRIDENNOTE_EDITOR", `'/opt/My Editors/subl' -n`)
	if got := systemEditorCommand(); len(got) != 3 || got[0] != "/opt/My Editors/subl" || got[2] != "--wait" {
		t.Fatalf("single-quoted path with args should split correctly, got %q", got)
	}
	t.Setenv("TRIDENNOTE_EDITOR", "/usr/local/bin/subl -n")
	if got := strings.Join(systemEditorCommand(), " "); got != "/usr/local/bin/subl -n --wait" {
		t.Fatalf("got %q", got)
	}
}

func TestSettingsModalPersistsChoices(t *testing.T) {
	fake := &fakeServer{}
	m, srv := signedIn(t, fake)
	defer srv.Close()

	m, _ = step(t, m, key(","))
	if m.modal != modalSettings {
		t.Fatalf("comma should open settings")
	}
	snapshot(t, "7-settings", m)

	rows := settingRows()
	for i, row := range rows {
		if row.section == "sidebar" {
			m.settingsCursor = i
		}
	}
	m, _ = step(t, m, key("enter"))
	for i, row := range rows {
		if row.section == "icons" {
			m.settingsCursor = i
		}
	}
	m, _ = step(t, m, key("right"))
	m.editorAvailability["nvim"] = false
	for i, row := range rows {
		if row.value == "nvim" {
			m.settingsCursor = i
		}
	}
	m, _ = step(t, m, key("enter"))
	if m.settings.Editor != "builtin" {
		t.Fatalf("unavailable editor must not be selected, got %q", m.settings.Editor)
	}
	m, _ = step(t, m, key("esc"))
	if m.modal != modalNone {
		t.Fatalf("esc should close settings")
	}

	reloaded := New(m.client)
	if reloaded.settings.SidebarSide != "left" || reloaded.settings.Icons != "minimal" {
		t.Fatalf("settings not persisted: %+v", reloaded.settings)
	}
	view := snapshot(t, "8-left-sidebar", m)
	first := strings.Split(view, "\n")[1]
	if !strings.HasPrefix(strings.TrimLeft(first, "│ "), "Ψ Personal") {
		t.Fatalf("tree should render on the left with minimal icons:\n%s", first)
	}

	m, _ = step(t, m, key("right"))
	if m.focus != focusViewer {
		t.Fatalf("right from a left tree should focus the note")
	}
	m, _ = step(t, m, key("right"))
	if m.focus != focusTree {
		t.Fatalf("right from the rightmost pane should wrap to the tree")
	}
	m, _ = step(t, m, key("left"))
	if m.focus != focusViewer {
		t.Fatalf("left from the leftmost pane should wrap to the note")
	}
	m, _ = step(t, m, key("left"))
	if m.focus != focusTree {
		t.Fatalf("left from the note should return to a tree on the left")
	}
	before := m.visible[0].Expanded
	m, _ = step(t, m, key("right"))
	m, _ = step(t, m, key("left"))
	if m.visible[0].Expanded != before {
		t.Fatalf("arrows must switch panes, not fold folders")
	}
}

func TestSidebarToggleAndCopy(t *testing.T) {
	fake := &fakeServer{}
	m, srv := signedIn(t, fake)
	defer srv.Close()

	var copied string
	original := copyToClipboard
	copyToClipboard = func(text string) error { copied = text; return nil }
	defer func() { copyToClipboard = original }()

	m, _ = step(t, m, key("l"))
	m, _ = step(t, m, key("down"))
	m, cmd := step(t, m, key("y"))
	m, _ = step(t, m, cmd())
	if !strings.Contains(copied, "- Ship the terminal client") || !strings.HasPrefix(copied, "## Q4") {
		t.Fatalf("copy should put the note markdown on the clipboard, got %q", copied)
	}
	if !strings.Contains(m.flash, "Copied Roadmap.md") {
		t.Fatalf("copy should confirm, flash=%q", m.flash)
	}

	m, _ = step(t, m, key("b"))
	if !m.sidebarHidden || m.focus != focusViewer || m.treeWidth() != 0 {
		t.Fatalf("b should hide the sidebar and focus the note")
	}
	view := snapshot(t, "9-sidebar-hidden", m)
	if strings.Contains(view, "Projects/") {
		t.Fatalf("tree should be hidden:\n%s", view)
	}
	m, _ = step(t, m, key("b"))
	if m.sidebarHidden || m.focus != focusTree {
		t.Fatalf("b again should restore the sidebar")
	}

	m, _ = step(t, m, key("?"))
	if view := snapshot(t, "10-help", m); !strings.Contains(view, "Keyboard shortcuts") {
		t.Fatalf("help modal not shown:\n%s", view)
	}
}

func TestLoaderAnimatesOnlyWhileLoading(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	fake := &fakeServer{}
	srv := httptest.NewServer(fake.handler(t))
	defer srv.Close()
	client := api.New(srv.URL)
	client.Token = token
	client.CookieName = cookieName

	m := New(client)
	m, cmd := step(t, m, tea.WindowSizeMsg{Width: 100, Height: 24})
	if !m.animating || cmd == nil {
		t.Fatalf("boot screen should start the animation")
	}
	first := snapshot(t, "11-loader-a", m)
	if !strings.Contains(first, "t r i d e n n o t e") {
		t.Fatalf("loader should show the wordmark:\n%s", first)
	}
	for range 9 {
		m, _ = step(t, m, animTickMsg{})
	}
	if snapshot(t, "11-loader-b", m) == first {
		t.Fatalf("frames should differ as the pulse travels")
	}
	for range framesPerPhrase {
		m, _ = step(t, m, animTickMsg{})
	}
	snapshot(t, "11-loader-c", m)
	if m.phrase(loaderBoot) == (Model{frame: 9}).phrase(loaderBoot) {
		t.Fatalf("phrase should rotate")
	}

	m, cmd = step(t, m, m.checkSession()())
	m, cmd = step(t, m, cmd())
	m, _ = step(t, m, cmd())
	if m.state != stateBrowse || m.needsAnimation() {
		t.Fatalf("idle browse should not animate")
	}
	m, cmd = step(t, m, animTickMsg{})
	if cmd != nil || m.animating {
		t.Fatalf("animation should stop once nothing is loading")
	}
}

func TestMermaidRendersInline(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := New(api.New("http://unused"))
	note := api.Note{Title: "Flow", Blocks: []api.Block{
		{Type: "text", Content: "Before the diagram"},
		{Type: "mermaid", Content: "graph TD\n  A[Write] --> B[Think]"},
		{Type: "text", Content: "Between diagrams"},
		{Type: "mermaid", Content: "pie title Pets\n  \"Dogs\" : 3"},
	}}
	out := ansi.Strip(m.renderNote(note, 70))
	if _, err := os.Stat("/dev/null"); err == nil && os.Getenv("TUI_SNAPSHOT_DIR") != "" {
		_ = os.WriteFile(filepath.Join(os.Getenv("TUI_SNAPSHOT_DIR"), "12-mermaid.txt"), []byte(out), 0o644)
	}
	for _, want := range []string{"Before the diagram", "mermaid · flowchart", "Write", "▼", "Between diagrams", "mermaid · pie", "Can't draw pie diagrams as text", "\"Dogs\" : 3"} {
		if !strings.Contains(out, want) {
			t.Fatalf("rendered note missing %q:\n%s", want, out)
		}
	}
	if strings.Index(out, "Before the diagram") > strings.Index(out, "Write") || strings.Index(out, "Write") > strings.Index(out, "Between diagrams") {
		t.Fatalf("diagram should render in place:\n%s", out)
	}
}

func TestDiagramKeyWithoutDiagrams(t *testing.T) {
	fake := &fakeServer{}
	m, srv := signedIn(t, fake)
	defer srv.Close()
	m, _ = step(t, m, key("l"))
	m, _ = step(t, m, key("down"))
	m, cmd := step(t, m, key("m"))
	m, _ = step(t, m, cmd())
	if m.flash != "No mermaid diagrams in this note" {
		t.Fatalf("flash = %q", m.flash)
	}
}

func sprintBlocks() []api.Block {
	open, done := false, true
	return []api.Block{
		{ID: "c1", Type: "h2", Content: "Backlog"},
		{ID: "k1a", Type: "todo", Content: "Set up CI !high #devops @harith due:2026-10-10", Checked: &open},
		{ID: "k1b", Type: "bulleted", Content: "Use GitHub Actions", Depth: 1},
		{ID: "k2a", Type: "bulleted", Content: "Write docs"},
		{ID: "c2", Type: "h2", Content: "In progress"},
		{ID: "c3", Type: "h2", Content: "Done"},
		{ID: "k3a", Type: "todo", Content: "Fix login !critical", Checked: &done},
	}
}

func run(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		t.Fatalf("expected a command")
	}
	m, _ = step(t, m, cmd())
	return m
}

func savedCard(t *testing.T, fake *fakeServer, id string) (int, api.Block) {
	t.Helper()
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for i, b := range fake.saved {
		if b.ID == id {
			return i, b
		}
	}
	t.Fatalf("block %s not saved: %+v", id, fake.saved)
	return 0, api.Block{}
}

func columnOf(t *testing.T, fake *fakeServer, id string) string {
	t.Helper()
	idx, _ := savedCard(t, fake, id)
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for i := idx; i >= 0; i-- {
		if fake.saved[i].Type == "h2" {
			return fake.saved[i].Content
		}
	}
	return ""
}

func TestKanbanBoard(t *testing.T) {
	fake := &fakeServer{board: true}
	m, srv := signedIn(t, fake)
	defer srv.Close()

	view := snapshot(t, "13-board-tree", m)
	if !strings.Contains(view, "Sprint") || strings.Contains(view, "Sprint.kanban") || strings.Contains(view, "Sprint.md") {
		t.Fatalf("board should show without its suffix:\n%s", view)
	}

	m, _ = step(t, m, key("G"))
	m, _ = step(t, m, key("k"))
	m, cmd := step(t, m, key("k"))
	if m.selected().ID != "k1" {
		t.Fatalf("G should select the board, got %s", m.selected().ID)
	}
	m, cmd = step(t, m, cmd())
	m = run(t, m, cmd)
	view = snapshot(t, "14-board-preview", m)
	for _, want := range []string{"Backlog", "In progress", "Done", "Set up CI", "!high", "#devops", "@harith", "due 2026-10-10", "✓ Fix login", "≡ 1"} {
		if !strings.Contains(view, want) {
			t.Fatalf("board preview missing %q:\n%s", want, view)
		}
	}

	m, _ = step(t, m, key("v"))
	view = snapshot(t, "15-board-markdown", m)
	if strings.Contains(view, "open board") || !strings.Contains(view, "Set up CI !high") {
		t.Fatalf("v should show the raw markdown:\n%s", view)
	}
	m, _ = step(t, m, key("v"))

	m, _ = step(t, m, key("enter"))
	if m.mode != modeBoard || m.boardID != "k1" || m.focus != focusViewer {
		t.Fatalf("enter on a board should start board mode, mode=%v", m.mode)
	}
	snapshot(t, "16-board-mode", m)

	m, cmd = step(t, m, key("L"))
	m = run(t, m, cmd)
	if got := columnOf(t, fake, "k1a"); got != "In progress" {
		t.Fatalf("L should move the card right, got %q", got)
	}
	if _, b := savedCard(t, fake, "k1b"); b.Depth != 1 {
		t.Fatalf("card body should travel with the card: %+v", b)
	}
	if m.boardCol != 1 || m.boardRow != 0 {
		t.Fatalf("focus should follow the card: col=%d row=%d", m.boardCol, m.boardRow)
	}

	m, cmd = step(t, m, tea.KeyMsg{Type: tea.KeyShiftRight})
	m = run(t, m, cmd)
	if _, b := savedCard(t, fake, "k1a"); columnOf(t, fake, "k1a") != "Done" || b.Checked == nil || !*b.Checked {
		t.Fatalf("moving into the last column should check the card: %+v", b)
	}
	m, cmd = step(t, m, key("H"))
	m = run(t, m, cmd)
	if _, b := savedCard(t, fake, "k1a"); *b.Checked {
		t.Fatalf("moving out of the last column should uncheck: %+v", b)
	}
	m, cmd = step(t, m, key("x"))
	m = run(t, m, cmd)
	if _, b := savedCard(t, fake, "k1a"); !*b.Checked || columnOf(t, fake, "k1a") != "In progress" {
		t.Fatalf("x should check without moving: %+v", b)
	}

	m, _ = step(t, m, key("h"))
	m, _ = step(t, m, key("a"))
	m = typeText(t, m, "Triage bugs !low #qa @ana due:2026-11-01")
	m, cmd = step(t, m, key("enter"))
	m = run(t, m, cmd)
	fake.mu.Lock()
	var added api.Block
	for _, b := range fake.saved {
		if strings.Contains(b.Content, "Triage bugs") {
			added = b
		}
	}
	fake.mu.Unlock()
	if added.ID == "" || added.Type != "todo" || added.Content != "[#4] Triage bugs !low #qa @ana due:2026-11-01" {
		t.Fatalf("new card should be saved with an id: %+v", added)
	}
	if _, b := savedCard(t, fake, "k2a"); b.Type != "todo" {
		t.Fatalf("bulleted cards are written back as todos: %+v", b)
	}

	m, _ = step(t, m, key("e"))
	m = typeText(t, m, " today")
	m, cmd = step(t, m, key("enter"))
	m = run(t, m, cmd)
	if _, b := savedCard(t, fake, added.ID); b.Content != "[#4] Triage bugs today !low #qa @ana due:2026-11-01" {
		t.Fatalf("edit should rewrite the card text: %+v", b)
	}

	m, cmd = step(t, m, key("d"))
	if cmd != nil || !m.boardConfirm {
		t.Fatalf("first d should only ask for confirmation")
	}
	m, cmd = step(t, m, key("d"))
	m = run(t, m, cmd)
	fake.mu.Lock()
	for _, b := range fake.saved {
		if b.ID == added.ID {
			t.Fatalf("card should be deleted")
		}
	}
	fake.mu.Unlock()

	m, _ = step(t, m, key("c"))
	m = typeText(t, m, "Review")
	m, cmd = step(t, m, key("enter"))
	m = run(t, m, cmd)
	var columns []string
	fake.mu.Lock()
	for _, b := range fake.saved {
		if b.Type == "h2" {
			columns = append(columns, b.Content)
		}
	}
	fake.mu.Unlock()
	if strings.Join(columns, ",") != "Backlog,Review,In progress,Done" {
		t.Fatalf("new column should land after the focused one: %v", columns)
	}

	m, _ = step(t, m, key("?"))
	if view := snapshot(t, "17-board-help", m); !strings.Contains(view, "Board shortcuts") {
		t.Fatalf("board help not shown:\n%s", view)
	}
	m, _ = step(t, m, key("esc"))
	m, _ = step(t, m, key("esc"))
	if m.mode != modeBrowse || m.focus != focusTree {
		t.Fatalf("esc should leave board mode back to the tree")
	}
	if view := snapshot(t, "18-board-after", m); !strings.Contains(view, "Review") {
		t.Fatalf("preview should show the saved board:\n%s", view)
	}
}

func TestCreateBoard(t *testing.T) {
	fake := &fakeServer{}
	m, srv := signedIn(t, fake)
	defer srv.Close()

	m, _ = step(t, m, key("N"))
	if m.mode != modeNewNote || !m.newBoard {
		t.Fatalf("N should prompt for a board")
	}
	if view := snapshot(t, "19-new-board", m); !strings.Contains(view, "New board in") {
		t.Fatalf("prompt should say board:\n%s", view)
	}
	m = typeText(t, m, "Launch")
	m, cmd := step(t, m, key("enter"))
	m, cmd = step(t, m, cmd())
	if fake.created["name"] != "Launch.kanban" || fake.created["fileType"] != "note" {
		t.Fatalf("unexpected create payload: %+v", fake.created)
	}
	if m.mode != modeBoard || m.boardID != "n9" {
		t.Fatalf("new board should open in board mode, mode=%v", m.mode)
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected a batch")
	}
	for _, c := range batch {
		if c == nil {
			continue
		}
		if msg, ok := c().(boardSavedMsg); ok {
			m, _ = step(t, m, msg)
		}
	}
	var columns []string
	for _, b := range fake.saved {
		if b.Type != "h2" || b.ID == "" || strings.HasPrefix(b.ID, "new-") {
			t.Fatalf("default columns should be h2 blocks with client ids: %+v", b)
		}
		columns = append(columns, b.Content)
	}
	if strings.Join(columns, ",") != "Backlog,Todo,In progress,Done" {
		t.Fatalf("default columns wrong: %v", columns)
	}
}

func TestEmptyBoardKeysAreSafe(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	m := New(api.New("http://unused"))
	m, _ = step(t, m, tea.WindowSizeMsg{Width: 100, Height: 24})
	m.state = stateBrowse
	m.notes["e1"] = api.Note{Title: "Empty.kanban"}
	m.enterBoard("e1")
	for _, k := range []string{"G", "g", "j", "k", "l", "h", "H", "L", "J", "K", "x", " ", "e", "d"} {
		var cmd tea.Cmd
		m, cmd = step(t, m, key(k))
		if cmd != nil {
			t.Fatalf("%q on an empty board should do nothing", k)
		}
	}
	m, _ = step(t, m, key("a"))
	if m.boardPrompt != promptNone || m.flash != "Add a column first (c)" {
		t.Fatalf("adding a card without columns should hint, flash=%q", m.flash)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "No columns yet") {
		t.Fatalf("empty board should explain itself:\n%s", view)
	}
}

func openSprintBoard(t *testing.T, fake *fakeServer) (Model, func()) {
	t.Helper()
	m, srv := signedIn(t, fake)
	m.selectByID("k1")
	m, cmd := step(t, m, key("enter"))
	m = run(t, m, cmd)
	if m.mode != modeBoard {
		t.Fatalf("expected board mode, got %v", m.mode)
	}
	m.metaWorkspace = ""
	m = run(t, m, m.loadWorkspaceMeta())
	return m, srv.Close
}

func savedContaining(t *testing.T, fake *fakeServer, text string) api.Block {
	t.Helper()
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for _, b := range fake.saved {
		if strings.Contains(b.Content, text) {
			return b
		}
	}
	t.Fatalf("no saved block contains %q: %+v", text, fake.saved)
	return api.Block{}
}

func TestCardComposerSuggestions(t *testing.T) {
	fake := &fakeServer{board: true}
	m, done := openSprintBoard(t, fake)
	defer done()
	if len(m.people) != 2 || m.people[1].Handle != "ana" {
		t.Fatalf("members should load as handles: %+v", m.people)
	}

	m, _ = step(t, m, key("a"))
	view := snapshot(t, "20-composer", m)
	for _, want := range []string{"New card", "Backlog", "!priority", "#tag", "@person", "due:date", "tab full editor"} {
		if !strings.Contains(view, want) {
			t.Fatalf("composer missing %q:\n%s", want, view)
		}
	}

	m = typeText(t, m, "Triage @an")
	view = snapshot(t, "21-composer-people", m)
	if !strings.Contains(view, "@ana") || !strings.Contains(view, "Ana Bell") || strings.Contains(view, "@harith ") {
		t.Fatalf("@ should list matching people:\n%s", view)
	}
	m, _ = step(t, m, key("enter"))
	if got := m.boardInput.Value(); got != "Triage @ana " {
		t.Fatalf("enter should insert the person, got %q", got)
	}

	m = typeText(t, m, "#re")
	if view := ansi.Strip(m.View()); !strings.Contains(view, "#release") {
		t.Fatalf("# should suggest workspace tags:\n%s", view)
	}
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m = typeText(t, m, "!h")
	m, _ = step(t, m, key("enter"))
	m = typeText(t, m, "due:")
	if s := m.visibleSuggestions(); len(s) == 0 || s[0].detail != "today" {
		t.Fatalf("due: should offer dates, got %+v", s)
	}
	m, _ = step(t, m, key("esc"))
	if len(m.visibleSuggestions()) != 0 || m.boardPrompt != promptAddCard {
		t.Fatal("esc should only hide the suggestions")
	}
	m = typeText(t, m, "2026-12-01")

	m, cmd := step(t, m, key("enter"))
	m = run(t, m, cmd)
	if b := savedContaining(t, fake, "Triage"); b.Content != "[#4] Triage !high #release @ana due:2026-12-01" {
		t.Fatalf("composer should save the tokens: %q", b.Content)
	}
	if m.boardPrompt != promptNone {
		t.Fatal("composer should close after saving")
	}
}

func TestComposerTabOpensFullModeDraft(t *testing.T) {
	fake := &fakeServer{board: true, nextID: newNoteID}
	m, done := openSprintBoard(t, fake)
	defer done()

	m, _ = step(t, m, key("a"))
	m = typeText(t, m, "Write spec !medium")
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyTab})
	if m.mode != modeCardDraft || !m.draftTitleFocus || fake.created != nil {
		t.Fatalf("tab should open a draft focused on the title without creating anything, mode=%v", m.mode)
	}
	view := snapshot(t, "24-card-draft-title", m)
	for _, want := range []string{"New card", "Sprint · Backlog", "TITLE", "Write spec !medium", "DESCRIPTION", "!priority"} {
		if !strings.Contains(view, want) {
			t.Fatalf("draft view missing %q:\n%s", want, view)
		}
	}

	m, _ = step(t, m, key("enter"))
	if m.draftTitleFocus || !m.editor.Focused() {
		t.Fatal("enter on the title should move to the description")
	}
	m = typeText(t, m, "Draft the API\n- first point")
	snapshot(t, "25-card-draft-body", m)
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if !m.draftTitleFocus {
		t.Fatal("shift+tab should go back to the title")
	}
	m, _ = step(t, m, key("enter"))

	m, cmd := step(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if m.mode != modeBoard {
		t.Fatalf("saving should return to the board, mode=%v", m.mode)
	}
	msg := cmd()
	if fake.created["name"] != "Write spec" {
		t.Fatalf("note should be named after the card title: %+v", fake.created)
	}
	if b := savedContaining(t, fake, "Draft the API"); b.Type != "text" {
		t.Fatalf("description should be saved into the note: %+v", b)
	}
	m, cmd = step(t, m, msg)
	if m.mode != modeBoard || !strings.Contains(m.flash, "Created #4 Write spec") {
		t.Fatalf("should stay on the board with a confirmation, mode=%v flash=%q", m.mode, m.flash)
	}
	for _, c := range cmd().(tea.BatchMsg) {
		if c != nil {
			m, _ = step(t, m, c())
		}
	}
	if b := savedContaining(t, fake, "Write spec"); b.Content != "[#4] Write spec !medium note:"+newNoteID {
		t.Fatalf("card should link the new note: %q", b.Content)
	}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "#4") || !strings.Contains(view, "↗ Write spec.md") {
		t.Fatalf("linked card should show its number and note:\n%s", view)
	}
}

func TestCardDraftGuards(t *testing.T) {
	fake := &fakeServer{board: true, nextID: newNoteID}
	m, done := openSprintBoard(t, fake)
	defer done()

	m, _ = step(t, m, key("a"))
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyTab})
	m, cmd := step(t, m, tea.KeyMsg{Type: tea.KeyCtrlS})
	if m.mode != modeCardDraft || !m.draftTitleFocus || fake.created != nil || m.flash != "Give the card a title first" {
		t.Fatalf("an empty title should not create anything, flash=%q", m.flash)
	}
	_ = cmd

	m = typeText(t, m, "Throwaway")
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeCardDraft || !m.confirmDiscard {
		t.Fatal("esc on a dirty draft should ask first")
	}
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeBoard || fake.created != nil {
		t.Fatalf("second esc should discard without creating a note, mode=%v", m.mode)
	}
}

func TestAddNoteToBoardFromActions(t *testing.T) {
	fake := &fakeServer{board: true}
	m, srv := signedIn(t, fake)
	defer srv.Close()
	m.selectByID(specNoteID)

	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyCtrlK})
	if m.modal != modalActions {
		t.Fatal("ctrl+k should open the actions menu")
	}
	m = typeText(t, m, "board")
	view := snapshot(t, "22-actions", m)
	if !strings.Contains(view, "Add to kanban board") || strings.Contains(view, "Settings") {
		t.Fatalf("actions should filter:\n%s", view)
	}
	m, cmd := step(t, m, key("enter"))
	if m.modal != modalAddToBoard || m.addStep != 1 {
		t.Fatalf("a single board should skip straight to columns, modal=%v step=%d", m.modal, m.addStep)
	}
	if batch, ok := cmd().(tea.BatchMsg); ok {
		for _, c := range batch {
			if c != nil {
				m, _ = step(t, m, c())
			}
		}
	} else {
		m = run(t, m, cmd)
	}
	m, _ = step(t, m, key("right"))
	view = snapshot(t, "23-add-to-board", m)
	if !strings.Contains(view, "In progress") || !strings.Contains(view, "Links Spec.md") {
		t.Fatalf("add-to-board should list columns:\n%s", view)
	}
	m, cmd = step(t, m, key("enter"))
	m = run(t, m, cmd)
	b := savedContaining(t, fake, "Spec")
	if b.Content != "[#4] Spec note:"+specNoteID {
		t.Fatalf("note should be linked as a card: %q", b.Content)
	}
	if got := columnOf(t, fake, b.ID); got != "In progress" {
		t.Fatalf("card should land in the chosen column, got %q", got)
	}
	if m.modal != modalNone || !strings.Contains(m.flash, "Added #4 to Sprint · In progress") {
		t.Fatalf("dialog should close with a confirmation, flash=%q", m.flash)
	}
}

func TestUpdateNoticeCopiesCommand(t *testing.T) {
	fake := &fakeServer{}
	m, srv := signedIn(t, fake)
	defer srv.Close()
	if cmd := m.checkUpdate(); cmd != nil {
		t.Fatal("no version means no update check")
	}

	var copied string
	original := copyToClipboard
	copyToClipboard = func(text string) error { copied = text; return nil }
	defer func() { copyToClipboard = original }()

	m, _ = step(t, m, updateMsg{info: &update.Info{Current: "v1.3.0", Latest: "v1.4.0", Method: update.Homebrew, Command: "brew upgrade tridennote/tap/tridennote"}})
	view := snapshot(t, "26-update-available", m)
	if !strings.Contains(view, "⬆ v1.4.0 available u update") {
		t.Fatalf("status bar should announce the update:\n%s", view)
	}
	m, _ = step(t, m, key("u"))
	if copied != "brew upgrade tridennote/tap/tridennote" || !strings.Contains(m.flash, "Homebrew update command") {
		t.Fatalf("u should copy the install-specific command, copied=%q flash=%q", copied, m.flash)
	}

	m.flash = ""
	m, _ = step(t, m, tea.KeyMsg{Type: tea.KeyCtrlK})
	m = typeText(t, m, "update")
	if view := ansi.Strip(m.View()); !strings.Contains(view, "Copy update command (v1.4.0)") {
		t.Fatalf("actions should offer the update:\n%s", view)
	}
}
