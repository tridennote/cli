package update

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	repo          = "tridennote/cli"
	scriptURL     = "https://raw.githubusercontent.com/" + repo + "/master/install.sh"
	powershellURL = "https://raw.githubusercontent.com/" + repo + "/master/install.ps1"
	DisableEnv    = "TRIDENNOTE_NO_UPDATE_CHECK"
)

var LatestURL = "https://api.github.com/repos/" + repo + "/releases/latest"

type Method string

const (
	Homebrew   Method = "Homebrew"
	Scoop      Method = "Scoop"
	Chocolatey Method = "Chocolatey"
	GoInstall  Method = "go install"
	Script     Method = "install script"
	PowerShell Method = "PowerShell installer"
)

type Info struct {
	Current string
	Latest  string
	Method  Method
	Command string
}

type Env struct {
	GOOS   string
	Home   string
	GOPATH string
	GOBIN  string
}

func normalize(p string) string {
	p = strings.ReplaceAll(p, `\`, "/")
	for len(p) > 1 && strings.HasSuffix(p, "/") {
		p = strings.TrimSuffix(p, "/")
	}
	return p
}

func samePath(a, b string, goos string) bool {
	a, b = normalize(a), normalize(b)
	if goos == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func parentDir(exe string) string {
	if i := strings.LastIndexAny(exe, `/\`); i > 0 {
		return exe[:i]
	}
	return ""
}

func Detect(exe string, env Env) (Method, string) {
	lower := strings.ToLower(normalize(exe))
	dir := parentDir(exe)
	switch {
	case strings.Contains(lower, "/cellar/") || strings.Contains(lower, "/homebrew/") || strings.Contains(lower, "/linuxbrew/"):
		return Homebrew, dir
	case env.GOOS == "windows" && strings.Contains(lower, "/scoop/"):
		return Scoop, dir
	case env.GOOS == "windows" && strings.Contains(lower, "/chocolatey/"):
		return Chocolatey, dir
	case isGoBin(dir, env):
		return GoInstall, dir
	case env.GOOS == "windows":
		return PowerShell, dir
	}
	return Script, dir
}

func isGoBin(dir string, env Env) bool {
	var candidates []string
	if env.GOBIN != "" {
		candidates = append(candidates, env.GOBIN)
	}
	separator := ":"
	if env.GOOS == "windows" {
		separator = ";"
	}
	for _, p := range strings.Split(env.GOPATH, separator) {
		if p != "" {
			candidates = append(candidates, p+"/bin")
		}
	}
	if env.GOPATH == "" && env.Home != "" {
		candidates = append(candidates, env.Home+"/go/bin")
	}
	for _, c := range candidates {
		if samePath(c, dir, env.GOOS) {
			return true
		}
	}
	return false
}

func Command(method Method, dir string, env Env) string {
	switch method {
	case Homebrew:
		return "brew upgrade tridennote/tap/tridennote"
	case Scoop:
		return "scoop update tridennote"
	case Chocolatey:
		return "choco upgrade tridennote"
	case GoInstall:
		return "go install github.com/" + repo + "/cmd/tridennote@latest"
	case PowerShell:
		cmd := "irm " + powershellURL + " | iex"
		if dir != "" && env.Home != "" && !samePath(dir, env.Home+"/AppData/Local/Programs/tridennote", env.GOOS) {
			cmd = "$env:TRIDENNOTE_INSTALL_DIR='" + dir + "'; " + cmd
		}
		return cmd
	}
	cmd := "curl -fsSL " + scriptURL + " | bash"
	defaults := []string{"/usr/local/bin"}
	if env.Home != "" {
		defaults = append(defaults, env.Home+"/.local/bin")
	}
	for _, d := range defaults {
		if samePath(d, dir, env.GOOS) {
			return cmd
		}
	}
	if dir == "" {
		return cmd
	}
	return "curl -fsSL " + scriptURL + " | TRIDENNOTE_INSTALL_DIR=" + shellQuote(dir) + " bash"
}

func shellQuote(s string) string {
	if strings.IndexFunc(s, func(r rune) bool {
		return !(r == '/' || r == '.' || r == '-' || r == '_' || r == '~' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'))
	}) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func parseVersion(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

func Newer(current, latest string) bool {
	cur, okCur := parseVersion(current)
	lat, okLat := parseVersion(latest)
	if !okCur || !okLat {
		return false
	}
	for i := range cur {
		if lat[i] != cur[i] {
			return lat[i] > cur[i]
		}
	}
	return false
}

func Latest(client *http.Client) (string, error) {
	req, err := http.NewRequest("GET", LatestURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("release check failed: %s", resp.Status)
	}
	var body struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	return body.TagName, nil
}

func CurrentEnv(goos string) Env {
	home, _ := os.UserHomeDir()
	return Env{GOOS: goos, Home: home, GOPATH: os.Getenv("GOPATH"), GOBIN: os.Getenv("GOBIN")}
}

func Executable() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		return resolved
	}
	return exe
}

func Check(current, goos string) (*Info, error) {
	if os.Getenv(DisableEnv) != "" {
		return nil, nil
	}
	if _, ok := parseVersion(current); !ok {
		return nil, nil
	}
	latest, err := Latest(&http.Client{Timeout: 4 * time.Second})
	if err != nil || !Newer(current, latest) {
		return nil, err
	}
	env := CurrentEnv(goos)
	method, dir := Detect(Executable(), env)
	return &Info{
		Current: "v" + strings.TrimPrefix(current, "v"),
		Latest:  "v" + strings.TrimPrefix(latest, "v"),
		Method:  method,
		Command: Command(method, dir, env),
	}, nil
}
