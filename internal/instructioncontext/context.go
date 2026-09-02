package instructioncontext

import (
	"time"

	"go.mewis.me/chatgpt-mcp/internal/rules"
	"go.mewis.me/chatgpt-mcp/internal/skills"
)

type SectionKind string

const (
	SectionUser    SectionKind = "user"
	SectionProject SectionKind = "project"
	SectionRule    SectionKind = "rule"
	SectionImport  SectionKind = "import"
)

type Section struct {
	Path          string      `json:"path"`
	Kind          SectionKind `json:"kind"`
	Source        string      `json:"source,omitempty"`
	Content       string      `json:"content"`
	Truncated     bool        `json:"truncated"`
	OriginalBytes int         `json:"original_bytes,omitempty"`
	LoadedBytes   int         `json:"loaded_bytes"`
}

type ProjectMemoryBundle struct {
	Root            string    `json:"root"`
	WorkspaceRoots  []string  `json:"workspace_roots"`
	Sections        []Section `json:"sections"`
	Imports         []Section `json:"imports,omitempty"`
	TotalBytes      int       `json:"total_bytes"`
	BudgetBytes     int       `json:"budget_bytes"`
	BudgetTruncated bool      `json:"budget_truncated"`
	LoadedAt        time.Time `json:"loaded_at"`
}

type AdminSnapshot struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url,omitempty"`
}

type EnvironmentSnapshot struct {
	Platform       string        `json:"platform"`
	OS             string        `json:"os"`
	Arch           string        `json:"arch"`
	Go             string        `json:"go"`
	PID            int           `json:"pid"`
	WorkspaceID    string        `json:"workspace_id"`
	WorkspaceRoot  string        `json:"workspace_root"`
	CWD            string        `json:"cwd"`
	EffectiveRoots []string      `json:"effective_roots"`
	Admin          AdminSnapshot `json:"admin"`
}

type GitSnapshot struct {
	IsRepo          bool     `json:"is_repo"`
	Root            string   `json:"root,omitempty"`
	Branch          string   `json:"branch,omitempty"`
	StatusShort     string   `json:"status_short,omitempty"`
	StatusTruncated bool     `json:"status_truncated,omitempty"`
	RecentCommits   []string `json:"recent_commits,omitempty"`
	Error           string   `json:"error,omitempty"`
}

type AutoMemorySnapshot struct {
	Loaded  bool   `json:"loaded"`
	Content string `json:"content,omitempty"`
	Bytes   int    `json:"bytes"`
}

type ToolProfile struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type InstructionContext struct {
	Root             string              `json:"root"`
	WorkspaceID      string              `json:"workspace_id"`
	WorkspaceRoots   []string            `json:"workspace_roots"`
	Environment      EnvironmentSnapshot `json:"environment"`
	Git              GitSnapshot         `json:"git"`
	ProjectMemory    ProjectMemoryBundle `json:"project_memory"`
	AutoMemory       AutoMemorySnapshot  `json:"auto_memory"`
	Rules            []rules.Rule        `json:"rules"`
	Skills           []skills.Skill      `json:"skills"`
	ToolProfile      ToolProfile         `json:"tool_profile"`
	InstructionsText string              `json:"instructions_text"`
	InstructionBytes int                 `json:"instruction_bytes"`
	LoadedAt         time.Time           `json:"loaded_at"`
}
