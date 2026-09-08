package application

import (
	"context"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/instructioncontext"
)

func ProjectContextToolProfile(ctx context.Context) instructioncontext.ToolProfile {
	if ctx == nil {
		ctx = context.Background()
	}
	statusCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	status, running, err := RuntimeStatus(statusCtx)
	if err != nil || !running {
		return instructioncontext.ToolProfile{Name: "full"}
	}
	name := status.ToolProfile
	if name == "" {
		name = "full"
	}
	return instructioncontext.ToolProfile{Name: name, Count: status.ToolCount}
}
