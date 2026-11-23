package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGitHubOIDCToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "bearer req-token" {
			t.Errorf("Authorization = %q", got)
		}
		if r.URL.Query().Get("api-version") != "2.0" || r.URL.Query().Get("audience") != "tridennote" {
			t.Errorf("query = %q", r.URL.RawQuery)
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"value": "jwt-123"})
	}))
	defer srv.Close()
	token, err := GitHubOIDCToken(srv.URL+"/token?api-version=2.0", "req-token", "tridennote")
	if err != nil || token != "jwt-123" {
		t.Fatalf("token = %q, err = %v", token, err)
	}
}

func TestGitHubOIDCTokenErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/empty" {
			_, _ = w.Write([]byte(`{}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	if _, err := GitHubOIDCToken(srv.URL+"/denied", "t", "tridennote"); err == nil {
		t.Error("expected error for 403")
	}
	if _, err := GitHubOIDCToken(srv.URL+"/empty", "t", "tridennote"); err == nil {
		t.Error("expected error for empty value")
	}
}

func TestBearerSecrets(t *testing.T) {
	var method, query, auth string
	var cookie bool
	var body map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, query, auth = r.Method, r.URL.RawQuery, r.Header.Get("Authorization")
		_, err := r.Cookie("sess")
		cookie = err == nil
		switch r.Method {
		case "GET":
			_, _ = w.Write([]byte(`{"data":{"workspaceId":"w1","env":".env","vars":[{"key":"A","value":"1"}]}}`))
		case "PUT":
			_ = json.NewDecoder(r.Body).Decode(&body)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"data":{"env":".env","key":"A","version":3}}`))
		case "DELETE":
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer srv.Close()
	c := New(srv.URL)
	c.Token, c.CookieName, c.BearerToken = "cookie-val", "sess", "tdn_st_abc"

	s, err := c.Secrets("", ".env")
	if err != nil || len(s.Vars) != 1 || s.Vars[0].Value != "1" {
		t.Fatalf("Secrets = %+v, %v", s, err)
	}
	if auth != "Bearer tdn_st_abc" || cookie || query != "env=.env" {
		t.Errorf("auth=%q cookie=%v query=%q", auth, cookie, query)
	}
	v, err := c.SetSecret("w1", "prod.env", "A", "2")
	if err != nil || v.Version != 3 || method != "PUT" {
		t.Fatalf("SetSecret = %+v, %v", v, err)
	}
	if body["workspace"] != "w1" || body["env"] != "prod.env" || body["key"] != "A" || body["value"] != "2" {
		t.Errorf("body = %v", body)
	}
	if err := c.UnsetSecret("w1", ".env", "A"); err != nil || method != "DELETE" || query != "env=.env&key=A&workspace=w1" {
		t.Errorf("UnsetSecret err=%v method=%s query=%q", err, method, query)
	}
}
