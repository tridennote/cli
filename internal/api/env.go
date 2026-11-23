package api

import "github.com/tridennote/cli/internal/envfile"

type EnvFileSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	VarCount  int    `json:"varCount"`
	UpdatedAt string `json:"updatedAt"`
}

type EnvFile struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	UpdatedAt string        `json:"updatedAt"`
	Vars      []envfile.Var `json:"vars"`
}

type EnvUpload struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

type EnvUploadResult struct {
	Files    []EnvFileSummary `json:"files"`
	SavedIDs []string         `json:"savedIds"`
}

func (c *Client) EnvFiles(workspaceID string) ([]EnvFileSummary, error) {
	return getData[[]EnvFileSummary](c, "GET", "/api/workspaces/"+workspaceID+"/env-files", nil)
}

func (c *Client) EnvFile(workspaceID, id string) (EnvFile, error) {
	return getData[EnvFile](c, "GET", "/api/workspaces/"+workspaceID+"/env-files/"+id, nil)
}

// UploadEnvFiles saves files by name; mode "replace" overwrites a same-named file, "merge" keeps its other keys.
func (c *Client) UploadEnvFiles(workspaceID string, files []EnvUpload, mode string) (EnvUploadResult, error) {
	return getData[EnvUploadResult](c, "POST", "/api/workspaces/"+workspaceID+"/env-files", map[string]any{"files": files, "mode": mode})
}

func (c *Client) DeleteEnvFile(workspaceID, id string) error {
	return c.do("DELETE", "/api/workspaces/"+workspaceID+"/env-files/"+id, nil, nil)
}
