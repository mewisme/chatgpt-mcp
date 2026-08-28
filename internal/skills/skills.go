package skills

type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Source      string `json:"source,omitempty"`
}

type Loaded struct {
	Skill     Skill  `json:"skill"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
}
