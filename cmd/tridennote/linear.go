package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tridennote/cli/internal/api"
	"github.com/tridennote/cli/internal/clilink"
	"github.com/tridennote/cli/internal/markdown"
)

const linearUsage = `Usage:
  tridennote -c                              pick a workspace, then a note, then how to open it
  tridennote -c <workspace>                  list that workspace's notes and pick one
  tridennote -c <workspace> <note>           open that note (prints markdown when not in a terminal,
                                             so agents can run it directly)
  tridennote -c <workspace> --list           print every note's path, one per line

Flags (after the arguments or before them):
  -p, --print                                print markdown to stdout (default when piped)
  -o, --open vi|vim|nvim|nano                open in a terminal editor; saving writes back
      --write                                replace the note with markdown from stdin
      --list                                 list notes instead of opening one

<workspace> and <note> are slugs, e.g. "my-workspace" and "folder/my-note" (or a note id).
The web app's "Copy terminal command" gives you the exact line.`

type linear struct {
	client *api.Client
	in     *bufio.Reader
	out    io.Writer
	errOut io.Writer
	tty    bool
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func runLinear(client *api.Client, args []string, stdin *os.File, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("tridennote -c", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprintln(stderr, linearUsage) }
	var print, write, list bool
	var open string
	fs.BoolVar(&print, "print", false, "")
	fs.BoolVar(&print, "p", false, "")
	fs.StringVar(&open, "open", "", "")
	fs.StringVar(&open, "o", "", "")
	fs.BoolVar(&write, "write", false, "")
	fs.BoolVar(&list, "list", false, "")
	positional, err := parseInterspersed(fs, args)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(positional) > 2 {
		return fmt.Errorf("too many arguments\n\n%s", linearUsage)
	}
	if client.Token == "" {
		return errors.New("not signed in — run `tridennote` once to sign in")
	}
	l := linear{client: client, in: bufio.NewReader(stdin), out: stdout, errOut: stderr, tty: isTerminal(stdin) && isTerminal(os.Stdout)}

	workspaces, err := client.Workspaces()
	if err != nil {
		if errors.Is(err, api.ErrUnauthorized) {
			return errors.New("your session expired — run `tridennote` to sign in again")
		}
		return err
	}
	ws, err := l.pickWorkspace(workspaces, positional)
	if err != nil {
		return err
	}
	rows, err := client.Tree(ws.ID)
	if err != nil {
		return err
	}
	notes := clilink.Notes(rows)
	if list {
		for _, n := range notes {
			fmt.Fprintln(stdout, n.Slug)
		}
		return nil
	}
	note, err := l.pickNote(ws, workspaces, notes, positional)
	if err != nil {
		return err
	}

	switch {
	case write:
		return l.writeFromStdin(note)
	case open != "":
		return l.openInEditor(note, open)
	case print || !l.tty:
		return l.print(note)
	}
	return l.chooseAction(note)
}

func (l linear) prompt(question string, max int) (int, string, error) {
	for {
		fmt.Fprint(l.errOut, question)
		line, err := l.in.ReadString('\n')
		answer := strings.TrimSpace(line)
		if answer == "" && err != nil {
			return 0, "", errors.New("no answer given")
		}
		if n, convErr := strconv.Atoi(answer); convErr == nil {
			if n >= 1 && n <= max {
				return n - 1, answer, nil
			}
		} else if answer != "" {
			return -1, answer, nil
		}
		if err != nil {
			return 0, "", fmt.Errorf("%q isn't one of the choices", answer)
		}
		fmt.Fprintf(l.errOut, "  enter a number from 1 to %d\n", max)
	}
}

func (l linear) pickWorkspace(all []api.Workspace, positional []string) (api.Workspace, error) {
	if len(positional) > 0 {
		if w, ok := clilink.FindWorkspace(all, positional[0]); ok {
			return w, nil
		}
		return api.Workspace{}, fmt.Errorf("no workspace %q — run `tridennote -c` to see them", positional[0])
	}
	fmt.Fprintln(l.errOut, "Workspaces:")
	for i, w := range all {
		fmt.Fprintf(l.errOut, "  %d) %s  (%s)\n", i+1, w.Label(), clilink.WorkspaceSlug(w, all))
	}
	for {
		i, text, err := l.prompt("Choose a workspace: ", len(all))
		if err != nil {
			return api.Workspace{}, err
		}
		if i >= 0 {
			return all[i], nil
		}
		if w, ok := clilink.FindWorkspace(all, text); ok {
			return w, nil
		}
		fmt.Fprintf(l.errOut, "  no workspace %q\n", text)
	}
}

func (l linear) pickNote(ws api.Workspace, all []api.Workspace, notes []clilink.Note, positional []string) (clilink.Note, error) {
	if len(notes) == 0 {
		return clilink.Note{}, fmt.Errorf("%s has no markdown notes", ws.Label())
	}
	if len(positional) == 2 {
		n, candidates := clilink.FindNote(notes, positional[1])
		if n.ID != "" {
			return n, nil
		}
		if len(candidates) > 1 {
			var names []string
			for _, c := range candidates {
				names = append(names, c.Slug)
			}
			return clilink.Note{}, fmt.Errorf("%q matches several notes: %s", positional[1], strings.Join(names, ", "))
		}
		return clilink.Note{}, fmt.Errorf("no note %q in %s — try `tridennote -c %s --list`", positional[1], ws.Label(), clilink.WorkspaceSlug(ws, all))
	}
	fmt.Fprintf(l.errOut, "Notes in %s:\n", ws.Label())
	for i, n := range notes {
		fmt.Fprintf(l.errOut, "  %d) %s\n", i+1, n.Path)
	}
	for {
		i, text, err := l.prompt("Choose a note: ", len(notes))
		if err != nil {
			return clilink.Note{}, err
		}
		if i >= 0 {
			fmt.Fprintf(l.errOut, "  tip: next time run  tridennote -c %s %s\n", clilink.WorkspaceSlug(ws, all), notes[i].Slug)
			return notes[i], nil
		}
		if n, _ := clilink.FindNote(notes, text); n.ID != "" {
			return n, nil
		}
		fmt.Fprintf(l.errOut, "  no note %q\n", text)
	}
}

var terminalEditors = []string{"vi", "vim", "nvim", "nano"}

func (l linear) chooseAction(note clilink.Note) error {
	options := []string{"print"}
	for _, e := range terminalEditors {
		if _, err := exec.LookPath(e); err == nil {
			options = append(options, e)
		}
	}
	fmt.Fprintf(l.errOut, "Open %s with:\n", note.Path)
	for i, o := range options {
		fmt.Fprintf(l.errOut, "  %d) %s\n", i+1, o)
	}
	i, text, err := l.prompt("Choose: ", len(options))
	if err != nil {
		return err
	}
	choice := text
	if i >= 0 {
		choice = options[i]
	}
	if choice == "print" || choice == "p" {
		return l.print(note)
	}
	return l.openInEditor(note, choice)
}

func (l linear) print(note clilink.Note) error {
	full, err := l.client.Note(note.ID)
	if err != nil {
		return err
	}
	_, err = io.WriteString(l.out, markdown.FromBlocks(full.Blocks))
	return err
}

func (l linear) writeFromStdin(note clilink.Note) error {
	raw, err := io.ReadAll(l.in)
	if err != nil {
		return err
	}
	return l.save(note, string(raw), "")
}

func (l linear) save(note clilink.Note, content, previous string) error {
	if content == previous {
		fmt.Fprintln(l.errOut, "No changes.")
		return nil
	}
	full, err := l.client.Note(note.ID)
	if err != nil {
		return err
	}
	if _, err := l.client.SaveNote(note.ID, markdown.Reconcile(full.Blocks, markdown.ToBlocks(content))); err != nil {
		return err
	}
	fmt.Fprintf(l.errOut, "Saved %s\n", note.Path)
	return nil
}

func (l linear) openInEditor(note clilink.Note, editor string) error {
	known := false
	for _, e := range terminalEditors {
		known = known || e == editor
	}
	if !known {
		return fmt.Errorf("unsupported editor %q (use vi, vim, nvim or nano)", editor)
	}
	bin, err := exec.LookPath(editor)
	if err != nil {
		return fmt.Errorf("%s isn't installed", editor)
	}
	full, err := l.client.Note(note.ID)
	if err != nil {
		return err
	}
	original := markdown.FromBlocks(full.Blocks)
	dir, err := os.MkdirTemp("", "tridennote-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	file := filepath.Join(dir, filepath.Base(clilink.Slug(note.Name)+".md"))
	if err := os.WriteFile(file, []byte(original), 0o600); err != nil {
		return err
	}
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return errors.New("opening an editor needs a terminal — use --print or --write instead")
	}
	defer tty.Close()
	cmd := exec.Command(bin, file)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = tty, tty, tty
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s exited: %w", editor, err)
	}
	edited, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	return l.save(note, string(edited), original)
}
