package settings

import (
	"encoding/json"
	"errors"
	"github.com/tridennote/cli/internal/appdir"
	"os"
	"path/filepath"
)

type Settings struct {
	Editor      string `json:"editor"`
	SidebarSide string `json:"sidebarSide"`
	Icons       string `json:"icons"`
}

const (
	SideLeft  = "left"
	SideRight = "right"

	IconsNerd    = "nerd"
	IconsMinimal = "minimal"
)

func Defaults() Settings {
	return Settings{Editor: "builtin", SidebarSide: SideRight, Icons: IconsNerd}
}

func path() (string, error) {
	return appdir.File("settings.json")
}

func Load() Settings {
	s := Defaults()
	p, err := path()
	if err != nil {
		return s
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return s
	}
	_ = json.Unmarshal(data, &s)
	if s.SidebarSide != SideLeft && s.SidebarSide != SideRight {
		s.SidebarSide = SideRight
	}
	if s.Icons != IconsNerd && s.Icons != IconsMinimal {
		s.Icons = IconsNerd
	}
	if s.Editor == "" {
		s.Editor = "builtin"
	}
	return s
}

func Save(s Settings) error {
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
	if err := os.WriteFile(p+".tmp", data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(p+".tmp", p); err != nil {
		return errors.Join(err, os.Remove(p+".tmp"))
	}
	return nil
}
