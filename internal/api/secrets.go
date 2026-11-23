package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/tridennote/cli/internal/envfile"
)

type Secrets struct {
	WorkspaceID string        `json:"workspaceId"`
	Env         string        `json:"env"`
	Vars        []envfile.Var `json:"vars"`
}

type SecretVersion struct {
	Env     string `json:"env"`
	Key     string `json:"key"`
	Version int    `json:"version"`
}

func secretsQuery(workspaceID, env, key string) string {
	q := url.Values{"env": {env}}
	if workspaceID != "" {
		q.Set("workspace", workspaceID)
	}
	if key != "" {
		q.Set("key", key)
	}
	return "/api/v1/secrets?" + q.Encode()
}

// Secrets fetches an env file's variables; workspaceID may be empty when a service token implies it.
func (c *Client) Secrets(workspaceID, env string) (Secrets, error) {
	return getData[Secrets](c, "GET", secretsQuery(workspaceID, env, ""), nil)
}

func (c *Client) SetSecret(workspaceID, env, key, value string) (SecretVersion, error) {
	body := map[string]string{"env": env, "key": key, "value": value}
	if workspaceID != "" {
		body["workspace"] = workspaceID
	}
	return getData[SecretVersion](c, "PUT", "/api/v1/secrets", body)
}

func (c *Client) UnsetSecret(workspaceID, env, key string) error {
	return c.do("DELETE", secretsQuery(workspaceID, env, key), nil, nil)
}

// GitHubOIDCToken asks the GitHub Actions runtime for an ID token with the given audience.
func GitHubOIDCToken(requestURL, requestToken, audience string) (string, error) {
	u, err := url.Parse(requestURL)
	if err != nil {
		return "", fmt.Errorf("bad ACTIONS_ID_TOKEN_REQUEST_URL: %w", err)
	}
	q := u.Query()
	q.Set("audience", audience)
	u.RawQuery = q.Encode()
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "bearer "+requestToken)
	req.Header.Set("Accept", "application/json")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return "", fmt.Errorf("could not get a GitHub OIDC token: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("could not get a GitHub OIDC token: %s (does the job have `permissions: id-token: write`?)", resp.Status)
	}
	var out struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("could not read the GitHub OIDC token: %w", err)
	}
	if out.Value == "" {
		return "", errors.New("GitHub returned an empty OIDC token")
	}
	return out.Value, nil
}
