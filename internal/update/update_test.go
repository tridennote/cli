package update

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDetectAndCommand(t *testing.T) {
	mac := Env{GOOS: "darwin", Home: "/Users/ana"}
	win := Env{GOOS: "windows", Home: `C:\Users\ana`}
	cases := []struct {
		name   string
		exe    string
		env    Env
		method Method
		cmd    string
	}{
		{"homebrew arm", "/opt/homebrew/Cellar/tridennote/1.3.0/bin/tridennote", mac, Homebrew, "brew upgrade tridennote/tap/tridennote"},
		{"linuxbrew", "/home/linuxbrew/.linuxbrew/Cellar/tridennote/1.3.0/bin/tridennote", Env{GOOS: "linux", Home: "/home/ana"}, Homebrew, "brew upgrade tridennote/tap/tridennote"},
		{"go default", "/Users/ana/go/bin/tridennote", mac, GoInstall, "go install github.com/tridennote/cli/cmd/tridennote@latest"},
		{"go custom GOBIN", "/opt/gobin/tridennote", Env{GOOS: "linux", Home: "/home/ana", GOBIN: "/opt/gobin"}, GoInstall, "go install github.com/tridennote/cli/cmd/tridennote@latest"},
		{"script default", "/usr/local/bin/tridennote", mac, Script, "curl -fsSL https://raw.githubusercontent.com/tridennote/cli/master/install.sh | bash"},
		{"script user dir", "/Users/ana/.local/bin/tridennote", mac, Script, "curl -fsSL https://raw.githubusercontent.com/tridennote/cli/master/install.sh | bash"},
		{"script custom dir", "/Users/ana/my tools/tridennote", mac, Script, "curl -fsSL https://raw.githubusercontent.com/tridennote/cli/master/install.sh | TRIDENNOTE_INSTALL_DIR='/Users/ana/my tools' bash"},
		{"scoop", `C:\Users\ana\scoop\apps\tridennote\current\tridennote.exe`, win, Scoop, "scoop update tridennote"},
		{"chocolatey", `C:\ProgramData\chocolatey\lib\tridennote\tools\tridennote.exe`, win, Chocolatey, "choco upgrade tridennote"},
		{"powershell default", `C:\Users\ana\AppData\Local\Programs\tridennote\tridennote.exe`, win, PowerShell, "irm https://raw.githubusercontent.com/tridennote/cli/master/install.ps1 | iex"},
		{"powershell custom", `D:\tools\tridennote.exe`, win, PowerShell, `$env:TRIDENNOTE_INSTALL_DIR='D:\tools'; irm https://raw.githubusercontent.com/tridennote/cli/master/install.ps1 | iex`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			method, dir := Detect(tc.exe, tc.env)
			if method != tc.method {
				t.Fatalf("Detect = %v, want %v", method, tc.method)
			}
			if got := Command(method, dir, tc.env); got != tc.cmd {
				t.Fatalf("Command = %q\n want %q", got, tc.cmd)
			}
		})
	}
}

func TestNewer(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"1.3.0", "v1.4.0", true},
		{"v1.3.0", "v1.3.1", true},
		{"1.3.0", "v1.3.0", false},
		{"1.10.0", "v1.9.9", false},
		{"2.0.0", "v1.99.0", false},
		{"dev", "v9.9.9", false},
		{"1.3.0-rc.1", "v1.3.0", false},
		{"1.3.0", "nonsense", false},
	}
	for _, tc := range cases {
		if got := Newer(tc.current, tc.latest); got != tc.want {
			t.Fatalf("Newer(%q, %q) = %v, want %v", tc.current, tc.latest, got, tc.want)
		}
	}
}

func TestLatestAndCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v1.4.0"}`))
	}))
	defer srv.Close()
	original := LatestURL
	LatestURL = srv.URL
	defer func() { LatestURL = original }()

	info, err := Check("1.3.0", "darwin")
	if err != nil || info == nil || info.Latest != "v1.4.0" || info.Current != "v1.3.0" || info.Command == "" {
		t.Fatalf("Check = %+v, %v", info, err)
	}
	if info, _ := Check("1.4.0", "darwin"); info != nil {
		t.Fatalf("up to date should report nothing, got %+v", info)
	}
	if info, _ := Check("dev", "darwin"); info != nil {
		t.Fatal("dev builds should never nag")
	}
	t.Setenv(DisableEnv, "1")
	if info, _ := Check("1.3.0", "darwin"); info != nil {
		t.Fatal("the opt-out env var should skip the check")
	}
}
