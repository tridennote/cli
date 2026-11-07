package api

type Workspace struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"isDefault"`
}

type TreeNode struct {
	ID       string  `json:"id"`
	ParentID *string `json:"parentId"`
	Kind     string  `json:"kind"`
	FileType *string `json:"fileType"`
	Name     string  `json:"name"`

	PrimaryAttachmentID *string `json:"primaryAttachmentId"`
}

type Block struct {
	ID       string  `json:"id,omitempty"`
	Type     string  `json:"type"`
	Content  string  `json:"content"`
	Checked  *bool   `json:"checked"`
	Language *string `json:"language"`
	Depth    int     `json:"depth"`
}

type Note struct {
	Title  string  `json:"title"`
	Blocks []Block `json:"blocks"`
}

func (c *Client) Workspaces() ([]Workspace, error) {
	workspaces, err := getData[[]Workspace](c, "GET", "/api/workspaces", nil)
	if err != nil || len(workspaces) > 0 {
		return workspaces, err
	}
	created, err := getData[Workspace](c, "POST", "/api/users/me/bootstrap", nil)
	if err != nil {
		return nil, err
	}
	return []Workspace{created}, nil
}

func (c *Client) Tree(workspaceID string) ([]TreeNode, error) {
	return getData[[]TreeNode](c, "GET", "/api/workspaces/"+workspaceID+"/tree", nil)
}

func (c *Client) Note(nodeID string) (Note, error) {
	return getData[Note](c, "GET", "/api/notes/"+nodeID, nil)
}

func (c *Client) SaveNote(nodeID string, blocks []Block) (Note, error) {
	if blocks == nil {
		blocks = []Block{}
	}
	return getData[Note](c, "PUT", "/api/notes/"+nodeID, map[string]any{"blocks": blocks})
}

func (c *Client) CreateNote(workspaceID string, parentID *string, name string) (TreeNode, error) {
	return getData[TreeNode](c, "POST", "/api/workspaces/"+workspaceID+"/nodes", map[string]any{
		"kind":     "file",
		"fileType": "note",
		"name":     name,
		"parentId": parentID,
	})
}

type Member struct {
	UserID string  `json:"userId"`
	Name   string  `json:"name"`
	Email  string  `json:"email"`
	Image  *string `json:"image"`
}

func (c *Client) Members(workspaceID string) ([]Member, error) {
	return getData[[]Member](c, "GET", "/api/workspaces/"+workspaceID+"/members", nil)
}

func (c *Client) Tags(workspaceID string) ([]string, error) {
	return getData[[]string](c, "GET", "/api/workspaces/"+workspaceID+"/tags", nil)
}
