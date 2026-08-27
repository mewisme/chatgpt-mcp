package app

import (
	"net/http"

	"go.mewis.me/chatgpt-mcp/internal/admin"
	"go.mewis.me/chatgpt-mcp/internal/auth"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/mcp"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
)

type App struct {
	Config   config.Config
	MCP      *mcp.HTTPRuntime
	Upstream *upstream.Manager
}

func New(cfg config.Config) *App {
	store := upstream.NewStore(upstream.Path())
	manager := upstream.NewManager(store)
	return &App{Config: cfg, MCP: mcp.NewHTTPRuntime(), Upstream: manager}
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/mcp", auth.Middleware("", a.MCP.Handler()))
	mux.Handle("/mcp/", auth.Middleware("", a.MCP.Handler()))
	mux.Handle("/admin/", admin.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	return mux
}
