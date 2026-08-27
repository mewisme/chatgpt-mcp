package context

import (
	"os"
	"path/filepath"
)

type Resolver struct{}

type WorkspaceContext struct {
	WorkingDirectory string
	Instructions     []string
	Skills           []string
}

func (Resolver) Resolve(workdir string) WorkspaceContext {
	ctx := WorkspaceContext{WorkingDirectory: workdir}
	for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
		path := filepath.Join(workdir, name)
		if _, err := os.Stat(path); err == nil {
			ctx.Instructions = append(ctx.Instructions, path)
		}
	}
	return ctx
}
