package envfile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const MaxFileBytes = 512 * 1024

type Source struct {
	Name    string
	Content string
}

var skipDirs = map[string]bool{".git": true, "node_modules": true, "vendor": true, ".venv": true, "dist": true, "build": true}

// Collect reads one env file, or every env-looking file under a folder. Folder entries are
// named by their path from the folder's parent, e.g. "api/.env.production".
func Collect(target string) (sources []Source, skipped int, err error) {
	target = ExpandHome(strings.TrimSpace(target))
	if target == "" {
		return nil, 0, errors.New("enter a file or folder path")
	}
	info, err := os.Stat(target)
	if err != nil {
		return nil, 0, err
	}
	if !info.IsDir() {
		if info.Size() > MaxFileBytes {
			return nil, 1, fmt.Errorf("%s is larger than 512 KB", filepath.Base(target))
		}
		raw, err := os.ReadFile(target)
		if err != nil {
			return nil, 0, err
		}
		return []Source{{Name: filepath.Base(target), Content: string(raw)}}, 0, nil
	}
	root := filepath.Clean(target)
	base := filepath.Dir(root)
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if d.IsDir() {
			if p != root && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !LooksLikeEnvFile(d.Name()) {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > MaxFileBytes || !info.Mode().IsRegular() {
			skipped++
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			skipped++
			return nil
		}
		rel, err := filepath.Rel(base, p)
		if err != nil {
			rel = d.Name()
		}
		sources = append(sources, Source{Name: filepath.ToSlash(rel), Content: string(raw)})
		return nil
	})
	if err == nil && len(sources) == 0 {
		err = fmt.Errorf("no .env files found in %s", filepath.Base(root))
	}
	return sources, skipped, err
}

func ExpandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}
