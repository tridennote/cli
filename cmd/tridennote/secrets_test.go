package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/tridennote/cli/internal/api"
	"github.com/tridennote/cli/internal/envfile"
)

func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestParseRunArgs(t *testing.T) {
	cases := []struct {
		args []string
		env  map[string]string
		want runOptions
	}{
		{[]string{"--", "npm", "start", "--port", "3"}, nil, runOptions{Env: ".env", Command: []string{"npm", "start", "--port", "3"}}},
		{[]string{"-w", "w1", "-e", "prod.env", "--no-override", "--", "go", "run", "."}, nil, runOptions{Workspace: "w1", Env: "prod.env", NoOverride: true, Command: []string{"go", "run", "."}}},
		{[]string{"--env=x.env", "make", "-j4"}, nil, runOptions{Env: "x.env", Command: []string{"make", "-j4"}}},
		{[]string{"--", "env"}, map[string]string{"TRIDENNOTE_ENV": "ci.env"}, runOptions{Env: "ci.env", Command: []string{"env"}}},
		{[]string{"-e", "a.env", "--", "env"}, map[string]string{"TRIDENNOTE_ENV": "ci.env"}, runOptions{Env: "a.env", Command: []string{"env"}}},
	}
	for _, c := range cases {
		got, err := parseRunArgs(c.args, envMap(c.env), io.Discard)
		if err != nil {
			t.Errorf("%v: %v", c.args, err)
			continue
		}
		if got.Workspace != c.want.Workspace || got.Env != c.want.Env || got.NoOverride != c.want.NoOverride || !slices.Equal(got.Command, c.want.Command) {
			t.Errorf("%v: got %+v, want %+v", c.args, got, c.want)
		}
	}
	for _, bad := range [][]string{{}, {"--"}, {"-e", "x"}, {"stray", "--", "cmd"}, {"--bogus", "--", "cmd"}} {
		if _, err := parseRunArgs(bad, envMap(nil), io.Discard); err == nil {
			t.Errorf("%v: expected error", bad)
		}
	}
}

func TestMergeEnv(t *testing.T) {
	base := []string{"PATH=/bin", "A=old", "B=keep"}
	vars := []envfile.Var{{Key: "A", Value: "new"}, {Key: "C", Value: "x=y"}}
	got := mergeEnv(base, vars, true)
	if want := []string{"PATH=/bin", "A=new", "B=keep", "C=x=y"}; !slices.Equal(got, want) {
		t.Errorf("override: %v", got)
	}
	got = mergeEnv(base, vars, false)
	if want := []string{"PATH=/bin", "A=old", "B=keep", "C=x=y"}; !slices.Equal(got, want) {
		t.Errorf("no override: %v", got)
	}
	if base[1] != "A=old" {
		t.Error("base was modified")
	}
}

func TestSelectAuth(t *testing.T) {
	gh := map[string]string{"ACTIONS_ID_TOKEN_REQUEST_URL": "http://x", "ACTIONS_ID_TOKEN_REQUEST_TOKEN": "t"}
	if selectAuth(envMap(nil)) != authSession {
		t.Error("empty env should use the session")
	}
	if selectAuth(envMap(gh)) != authGitHubOIDC {
		t.Error("actions env should use OIDC")
	}
	if selectAuth(envMap(map[string]string{"ACTIONS_ID_TOKEN_REQUEST_URL": "http://x"})) != authSession {
		t.Error("OIDC needs both variables")
	}
	gh["TRIDENNOTE_TOKEN"] = "tdn_st_x"
	if selectAuth(envMap(gh)) != authServiceToken {
		t.Error("service token should win")
	}
}

func TestAuthenticate(t *testing.T) {
	oidc := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("audience") != "tridennote" || r.Header.Get("Authorization") != "bearer rt" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"value":"gh-jwt"}`))
	}))
	defer oidc.Close()
	gh := map[string]string{"ACTIONS_ID_TOKEN_REQUEST_URL": oidc.URL + "/?api-version=2.0", "ACTIONS_ID_TOKEN_REQUEST_TOKEN": "rt"}

	c := api.New("http://unused")
	mode, ws, _, err := authenticate(c, "", envMap(map[string]string{"TRIDENNOTE_TOKEN": "tdn_st_x"}))
	if err != nil || mode != authServiceToken || ws != "" || c.BearerToken != "tdn_st_x" {
		t.Errorf("service token: mode=%v ws=%q bearer=%q err=%v", mode, ws, c.BearerToken, err)
	}

	c = api.New("http://unused")
	if _, _, _, err := authenticate(c, "", envMap(gh)); err == nil || !strings.Contains(err.Error(), "workspace ID") {
		t.Errorf("OIDC without workspace: %v", err)
	}
	gh["TRIDENNOTE_WORKSPACE"] = "ws-1"
	mode, ws, _, err = authenticate(c, "", envMap(gh))
	if err != nil || mode != authGitHubOIDC || ws != "ws-1" || c.BearerToken != "gh-jwt" {
		t.Errorf("OIDC: mode=%v ws=%q bearer=%q err=%v", mode, ws, c.BearerToken, err)
	}
	if _, ws, _, _ = authenticate(api.New("http://unused"), "ws-flag", envMap(gh)); ws != "ws-flag" {
		t.Errorf("-w should beat TRIDENNOTE_WORKSPACE, got %q", ws)
	}

	if _, _, _, err := authenticate(api.New("http://unused"), "", envMap(nil)); err == nil || !strings.Contains(err.Error(), "not signed in") {
		t.Errorf("no session: %v", err)
	}
}

func TestParseAssignments(t *testing.T) {
	vars, err := parseAssignments([]string{"A=1", "B=x=y", "C=", "D=-"}, strings.NewReader("from stdin\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []envfile.Var{{Key: "A", Value: "1"}, {Key: "B", Value: "x=y"}, {Key: "C", Value: ""}, {Key: "D", Value: "from stdin"}}
	if !slices.Equal(vars, want) {
		t.Errorf("got %v", vars)
	}
	for _, bad := range [][]string{{"NOEQUALS"}, {"=v"}, {"A=-", "B=-"}} {
		if _, err := parseAssignments(bad, strings.NewReader("")); err == nil {
			t.Errorf("%v: expected error", bad)
		}
	}
}

func secretsServer(t *testing.T, calls *[]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tdn_st_ok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		q := r.URL.Query()
		switch r.Method {
		case "GET":
			if q.Get("env") != "prod.env" {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"error":{"code":"not_found","message":"env not found"}}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":{"workspaceId":"w1","env":"prod.env","vars":[{"key":"FOO","value":"bar"},{"key":"SECRET","value":"s3cr3t"}]}}`))
		case "PUT":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			*calls = append(*calls, "PUT "+body["env"]+" "+body["key"]+"="+body["value"])
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"data":{"env":"prod.env","key":"` + body["key"] + `","version":2}}`))
		case "DELETE":
			*calls = append(*calls, "DELETE "+q.Get("env")+" "+q.Get("key"))
			w.WriteHeader(http.StatusNoContent)
		}
	}))
}

func clearAuthEnv(t *testing.T) {
	for _, k := range []string{"TRIDENNOTE_TOKEN", "TRIDENNOTE_WORKSPACE", "TRIDENNOTE_ENV", "ACTIONS_ID_TOKEN_REQUEST_URL", "ACTIONS_ID_TOKEN_REQUEST_TOKEN"} {
		t.Setenv(k, "")
	}
}

func TestEnvCommandsWithServiceToken(t *testing.T) {
	clearAuthEnv(t)
	t.Setenv("TRIDENNOTE_TOKEN", "tdn_st_ok")
	var calls []string
	srv := secretsServer(t, &calls)
	defer srv.Close()
	client := api.New(srv.URL)

	var stdout, stderr bytes.Buffer
	if err := runEnv(client, []string{"get", "prod.env", "-k", "FOO"}, nil, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != "FOO=bar\n" {
		t.Errorf("get output %q", stdout.String())
	}
	if err := runEnv(client, []string{"get", "missing.env"}, nil, io.Discard, io.Discard); err == nil || err.Error() != "env not found" {
		t.Errorf("missing env: %v", err)
	}

	stderr.Reset()
	if err := runEnv(client, []string{"set", "prod.env", "A=1", "B=-"}, strings.NewReader("piped\n"), io.Discard, &stderr); err != nil {
		t.Fatal(err)
	}
	if err := runEnv(client, []string{"unset", "prod.env", "A"}, nil, io.Discard, &stderr); err != nil {
		t.Fatal(err)
	}
	want := []string{"PUT prod.env A=1", "PUT prod.env B=piped", "DELETE prod.env A"}
	if !slices.Equal(calls, want) {
		t.Errorf("calls = %v", calls)
	}
	if strings.Contains(stderr.String(), "piped") {
		t.Errorf("stderr leaked a value: %q", stderr.String())
	}

	t.Setenv("TRIDENNOTE_TOKEN", "tdn_st_bad")
	if err := runEnv(client, []string{"get", "prod.env"}, nil, io.Discard, io.Discard); err == nil || !strings.Contains(err.Error(), "TRIDENNOTE_TOKEN was rejected") {
		t.Errorf("bad token: %v", err)
	}
}

func TestRunPassesSecretsAndExitCode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	clearAuthEnv(t)
	t.Setenv("TRIDENNOTE_TOKEN", "tdn_st_ok")
	t.Setenv("FOO", "preset")
	srv := secretsServer(t, new([]string))
	defer srv.Close()
	client := api.New(srv.URL)

	err := runRun(client, []string{"-e", "prod.env", "--", "sh", "-c", `[ "$FOO" = bar ] && [ "$SECRET" = s3cr3t ] && exit 7`}, io.Discard)
	var code exitCode
	if !errors.As(err, &code) || code != 7 {
		t.Errorf("override run: %v", err)
	}
	err = runRun(client, []string{"-e", "prod.env", "--no-override", "--", "sh", "-c", `[ "$FOO" = preset ] && exit 0; exit 1`}, io.Discard)
	if err != nil {
		t.Errorf("no-override run: %v", err)
	}
}
