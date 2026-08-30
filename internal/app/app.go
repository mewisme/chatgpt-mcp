package app

import (
	"net/http"

	"go.mewis.me/chatgpt-mcp/internal/activity"
	"go.mewis.me/chatgpt-mcp/internal/admin"
	"go.mewis.me/chatgpt-mcp/internal/auth"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/mcp"
	mcpoauth "go.mewis.me/chatgpt-mcp/internal/oauth"
	"go.mewis.me/chatgpt-mcp/internal/telemetry"
	"go.mewis.me/chatgpt-mcp/internal/tools"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
	"go.mewis.me/chatgpt-mcp/internal/web"
)

type App struct {
	Config     *config.RuntimeStore
	MCP        *mcp.HTTPRuntime
	Upstream   *upstream.Manager
	Tools      *tools.Runtime
	Activity   *activity.Stream
	Tunnel     *tunnel.Client
	Logger     *logger.Logger
	OAuth      *mcpoauth.Store
	OAuthFlows *mcpoauth.FlowManager
}

func New(cfg config.Config) *App { return NewWithLogger(cfg, nil) }

func NewWithLogger(cfg config.Config, appLogger *logger.Logger) *App {
	stream := activity.NewStream()
	toolRuntime := tools.NewRuntimeWithFeatures(cfg.Features)
	mcpRuntime := mcp.NewHTTPRuntimeWithTools(toolRuntime)
	mcpRuntime.Activity = stream
	oauthStore := mcpoauth.NewStore(mcpoauth.Path())
	if appLogger == nil {
		appLogger = logger.New(logger.Info)
	}
	telemetry.AttachTools(toolRuntime, stream, appLogger)
	configStore := config.NewRuntimeStore(cfg)
	app := &App{
		Config: configStore, MCP: mcpRuntime, Upstream: toolRuntime.Upstream, Tools: toolRuntime, Activity: stream,
		Tunnel: tunnel.NewConfiguredWithLogger(cfg.Tunnel, toolRuntime, appLogger), Logger: appLogger,
		OAuth: oauthStore, OAuthFlows: mcpoauth.NewFlowManager(oauthStore),
	}
	app.attachTunnelLifecycle()
	return app
}

func (a *App) MCPHandler() http.Handler {
	mux := http.NewServeMux()
	mcpHandler := auth.DynamicHashedMiddleware(func() (bool, string) {
		cfg := a.Config.Snapshot()
		return cfg.Auth.MCPEnabled, cfg.Auth.MCPTokenHash
	}, a.MCP.Handler())
	mux.Handle("/mcp", mcpHandler)
	mux.Handle("/mcp/", mcpHandler)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	return mux
}

func (a *App) AdminHandler() http.Handler {
	mux := http.NewServeMux()
	cfg := a.Config.Snapshot()
	if !cfg.Admin.Enabled {
		return http.NotFoundHandler()
	}
	adminAPI := admin.API{
		Upstream: a.Upstream, Tools: a.Tools, Tunnel: a.Tunnel, Config: a.Config, OAuth: a.OAuth, OAuthFlows: a.OAuthFlows,
	}
	adminAuth := func() (bool, string) {
		cfg := a.Config.Snapshot()
		return cfg.Auth.AdminEnabled, cfg.Auth.AdminTokenHash
	}
	adminHandler := auth.DynamicHashedMiddleware(adminAuth, admin.New(adminAPI))
	mux.Handle("/oauth/callback/", adminAPI.OAuthCallbackHandler())
	mux.Handle("/admin/", adminHandler)
	mux.Handle("/api/", adminHandler)
	mux.Handle("/api/activity/stream", auth.DynamicHashedMiddleware(adminAuth, activity.Handler(a.Activity)))
	mux.Handle("/", web.Handler())
	return mux
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/mcp", a.MCPHandler())
	mux.Handle("/mcp/", a.MCPHandler())
	mux.Handle("/health", a.MCPHandler())
	mux.Handle("/", a.AdminHandler())
	return mux
}
