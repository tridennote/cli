package session

import (
	"encoding/json"
	"errors"
	"github.com/tridennote/cli/internal/appdir"
	"os"
	"path/filepath"
)

type Session struct {
	Token      string `json:"token"`
	CookieName string `json:"cookieName"`
	Email      string `json:"email,omitempty"`
}

func path() (string, error) {
	return appdir.File("session.json")
}

func Load() (*Session, error) {
	p, err := path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil || s.Token == "" {
		return nil, nil
	}
	return &s, nil
}

func Save(s Session) error {
	p, err := path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

func Clear() error {
	p, err := path()
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
