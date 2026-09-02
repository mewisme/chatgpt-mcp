package instructioncontext

import (
	"errors"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/memory"
)

func LoadAutoMemory(store memory.Store, workspaceID string) (AutoMemorySnapshot, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return AutoMemorySnapshot{}, errors.New("workspace id is required")
	}
	content, err := store.Load(workspaceID)
	if err != nil {
		return AutoMemorySnapshot{}, err
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return AutoMemorySnapshot{}, nil
	}
	return AutoMemorySnapshot{Loaded: true, Content: content, Bytes: len([]byte(content))}, nil
}
