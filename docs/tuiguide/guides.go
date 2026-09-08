package tuiguide

import (
	"embed"
	"fmt"
	"strings"
)

type Topic struct {
	ID          string
	Title       string
	Description string
	Keywords    []string
	File        string
}

var topics = []Topic{
	{ID: "getting-started", Title: "Getting Started", Description: "Navigation, Commands, help, mouse support, and the TUI interaction model.", Keywords: []string{"home", "navigation", "commands", "ctrl+k", "keyboard", "mouse"}, File: "getting-started.md"},
	{ID: "editors", Title: "Editors & Forms", Description: "Full-page editors, sections, validation, dirty drafts, switches, path pickers, and save behavior.", Keywords: []string{"form", "editor", "ctrl+s", "validation", "path", "picker", "switch"}, File: "editors.md"},
	{ID: "workspaces", Title: "Workspaces", Description: "Workspace registration, access directories, containers, Project Context build, and preview.", Keywords: []string{"workspace", "container", "access", "project context", "memory", "skills"}, File: "workspaces.md"},
	{ID: "mcp", Title: "MCP Servers", Description: "Upstream MCP servers, Form/JSON creation, transports, tools, health, and OAuth.", Keywords: []string{"mcp", "server", "stdio", "http", "json", "oauth", "tools"}, File: "mcp.md"},
	{ID: "tunnel", Title: "Tunnel", Description: "Runtime tunnel configuration, admin key, and managed OpenAI Secure MCP Tunnels.", Keywords: []string{"tunnel", "managed", "openai", "admin key", "runtime"}, File: "tunnel.md"},
	{ID: "requests", Title: "Requests & Approvals", Description: "Approval inbox, request details, countdowns, approve/deny flows, and live approval dialogs.", Keywords: []string{"request", "approval", "allow", "deny", "guard", "countdown"}, File: "requests.md"},
	{ID: "logs", Title: "Logs", Description: "Runtime logs, command execution output, filters, follow/pause, and structured event details.", Keywords: []string{"logs", "runtime", "execution", "filter", "journal", "follow"}, File: "logs.md"},
	{ID: "config", Title: "Configuration", Description: "Configuration domains, typed fields, shell policy, storage maintenance, import/export, and conversion.", Keywords: []string{"config", "shell", "allow commands", "policy", "import", "export", "storage"}, File: "config.md"},
	{ID: "instruction", Title: "Instruction", Description: "Global Context Markdown preview, Global Rules, instruction sources, and source policy.", Keywords: []string{"instruction", "context", "rules", "sources", "markdown", "glamour"}, File: "instruction.md"},
	{ID: "runtime", Title: "Runtime & System", Description: "Managed service state, authentication, installation, updates, aliases, and runtime lifecycle actions.", Keywords: []string{"runtime", "service", "auth", "install", "update", "alias", "status"}, File: "runtime.md"},
}

//go:embed *.md
var files embed.FS

func Topics() []Topic { return append([]Topic(nil), topics...) }

func Lookup(id string) (Topic, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	for _, topic := range topics {
		if topic.ID == id {
			return topic, true
		}
	}
	return Topic{}, false
}

func Markdown(id string) (string, error) {
	topic, ok := Lookup(id)
	if !ok {
		return "", fmt.Errorf("unknown TUI guide topic: %s", strings.TrimSpace(id))
	}
	data, err := files.ReadFile(topic.File)
	if err != nil {
		return "", fmt.Errorf("read TUI guide %s: %w", topic.ID, err)
	}
	return string(data), nil
}
