package app

import (
	"go.mewis.me/chatgpt-mcp/internal/mcp"
	"go.mewis.me/chatgpt-mcp/internal/tools"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
)

func (a *App) Bootstrap() error {
	if a.Upstream != nil {
		if err := a.Upstream.Load(); err != nil {
			return err
		}
	}
	if a.Tools == nil {
		a.Tools = tools.NewRuntime()
	}
	if a.MCP == nil {
		a.MCP = mcp.NewHTTPRuntimeWithTools(a.Tools)
	} else if a.MCP.Server == nil {
		a.MCP.Server = mcp.NewRuntimeWithTools(a.Tools)
	} else {
		a.MCP.Server.Tools = a.Tools
	}
	return nil
}

func newUpstream() *upstream.Manager { return upstream.NewManager(upstream.NewStore(upstream.Path())) }
