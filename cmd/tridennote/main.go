package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tridennote/cli/internal/api"
	"github.com/tridennote/cli/internal/session"
	"github.com/tridennote/cli/internal/tui"
)

var (
	baseURL = api.DefaultBaseURL
	version = "dev"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v", "version":
			fmt.Println("tridennote " + version)
			return
		case "--help", "-h", "help":
			fmt.Println("tridennote " + version + "\n\nNotes, next level, in your terminal.\n\nUsage:\n  tridennote            open your vault\n  tridennote --version  print the version\n\nPress ? inside the app for keyboard shortcuts.")
			return
		}
	}
	client := api.New(baseURL)

	saved, err := session.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not read saved session: %v\n", err)
	}
	if saved != nil {
		client.Token = saved.Token
		client.CookieName = saved.CookieName
	}
	client.OnRotate = func(token string) {
		_ = session.Save(session.Session{Token: token, CookieName: client.CookieName})
	}

	program := tea.NewProgram(tui.New(client).WithVersion(version), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "tridennote: %v\n", err)
		os.Exit(1)
	}
}
