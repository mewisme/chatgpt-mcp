package app

import (
	"net/http"

	"go.mewis.me/chatgpt-mcp/internal/activity"
	"go.mewis.me/chatgpt-mcp/internal/admin"
	"go.mewis.me/chatgpt-mcp/internal/auth"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/mcp"
	"go.mewis.me/chatgpt-mcp/internal/tools"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
	"go.mewis.me/chatgpt-mcp/internal/web"
)

type App struct {
	Config   config.Config
	MCP      *mcp.HTTPRuntime
	Upstream *upstream.Manager
	Tools    *tools.Registry
	Activity *activity.Stream
	Tunnel   *tunnel.Client
}

func New(cfg config.Config) *App {
	store := upstream.NewStore(upstream.Path())
	manager := upstream.NewManager(store)
	stream := activity.NewStream()
	mcpRuntime := mcp.NewHTTPRuntime()
	mcpRuntime.Activity = stream
	mcpRuntime.Lifecycle = mcp.NewLifecycle(mcpRuntime.Sessions, stream)
	tunnelConfig := cfg.Tunnel
	if tunnelConfig.Origin == "" {
		tunnelConfig.Origin = config.TunnelOrigin(cfg)
	}
	return &App{Config: cfg, MCP: mcpRuntime, Upstream: manager, Tools: tools.NewRegistry(), Activity: stream, Tunnel: tunnel.NewConfigured(tunnelConfig)}
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mcpHandler := auth.HashedMiddleware(a.Config.Auth.MCPEnabled, a.Config.Auth.MCPTokenHash, a.MCP.Handler())
	mux.Handle("/mcp", mcpHandler)
	mux.Handle("/mcp/", mcpHandler)
	if a.Config.Admin.Enabled {
		adminHandler := auth.HashedMiddleware(a.Config.Auth.AdminEnabled, a.Config.Auth.AdminTokenHash, admin.New(admin.API{Upstream: a.Upstream, Tools: a.Tools, Tunnel: a.Tunnel, Config: &a.Config}))
		mux.Handle("/admin/", adminHandler)
		mux.Handle("/api/", adminHandler)
		mux.Handle("/api/activity/stream", auth.HashedMiddleware(a.Config.Auth.AdminEnabled, a.Config.Auth.AdminTokenHash, activity.Handler(a.Activity)))
	}
	mux.Handle("/", web.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	return mux
}
