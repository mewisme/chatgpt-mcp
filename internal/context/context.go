package context

import "go.mewis.me/chatgpt-mcp/internal/skills"

type WorkspaceContext struct {
	WorkingDirectory string
	Skills []skills.Skill
}

func Resolve(workingDirectory string) WorkspaceContext {
	return WorkspaceContext{WorkingDirectory: workingDirectory}
}
