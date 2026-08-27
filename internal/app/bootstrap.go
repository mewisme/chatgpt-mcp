package app

import (
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
		a.Tools = tools.NewRegistry()
	}
	return nil
}

func newUpstream() *upstream.Manager { return upstream.NewManager(upstream.NewStore(upstream.Path())) }
