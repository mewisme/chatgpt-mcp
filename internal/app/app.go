package app

import (
	"net/http"

	"go.mewis.me/chatgpt-mcp/internal/auth"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/mcp"
)

type App struct {
	Config config.Config
	MCP    *mcp.HTTPRuntime
}

func New(cfg config.Config) *App { return &App{Config: cfg, MCP: mcp.NewHTTPRuntime()} }

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/mcp", auth.Middleware("", a.MCP.Handler()))
	mux.Handle("/mcp/", auth.Middleware("", a.MCP.Handler()))
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	return mux
}
