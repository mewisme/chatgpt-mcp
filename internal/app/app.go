package app

import (
	"net/http"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/mcp"
)

type App struct {
	Config  config.Config
	MCP     *mcp.Server
	Runtime *mcp.Runtime
}

func New(cfg config.Config) *App {
	return &App{Config: cfg, MCP: mcp.NewServer(), Runtime: mcp.NewRuntime()}
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/mcp", a.MCP.HandlerFunc())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"ok":true}`)) })
	return mux
}
