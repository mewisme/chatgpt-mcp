package app

import (
	"go.mewis.me/chatgpt-mcp/internal/mcp"
	"go.mewis.me/chatgpt-mcp/internal/tools"
)

func (a *App) Bootstrap() error {
	if a.Tools == nil {
		a.Tools = tools.NewRuntime()
	}
	a.Upstream = a.Tools.Upstream
	if a.MCP == nil {
		a.MCP = mcp.NewHTTPRuntimeWithTools(a.Tools)
	} else if a.MCP.Server == nil {
		a.MCP.Server = mcp.NewRuntimeWithTools(a.Tools)
	} else {
		a.MCP.Server.Tools = a.Tools
	}
	return nil
}
