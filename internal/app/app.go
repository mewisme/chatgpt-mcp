package app

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/activity"
	"go.mewis.me/chatgpt-mcp/internal/admin"
	"go.mewis.me/chatgpt-mcp/internal/auth"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/mcp"
	"go.mewis.me/chatgpt-mcp/internal/notification"
	"go.mewis.me/chatgpt-mcp/internal/pluginhost"
	"go.mewis.me/chatgpt-mcp/internal/tools"
	tracepkg "go.mewis.me/chatgpt-mcp/internal/trace"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
	"go.mewis.me/chatgpt-mcp/internal/web"
)

type App struct {
	Config        *config.RuntimeStore
	MCP           *mcp.HTTPRuntime
	Upstream      *upstream.Manager
	Tools         *tools.Runtime
	Activity      *activity.Stream
	Tunnels       *tunnel.Manager
	Tunnel        *tunnel.Client
	Logger        *logger.Logger
	Notifications *notification.Service
	Bridge        *mcp.PrivateBridge
	runtimeCtx    context.Context
	trace         tracepkg.Observer
	running       bool
	bootstrap     sync.Once
	bootstrapErr  error
}

func New(cfg config.Config) (*App, error) {
	return NewWithLoggerContext(context.Background(), cfg, nil)
}

func NewWithLogger(cfg config.Config, appLogger *logger.Logger) (*App, error) {
	return NewWithLoggerContext(context.Background(), cfg, appLogger)
}

// NewWithLoggerContext constructs the runtime application. Shell-policy and Bootstrap
// failures are returned; tools.NewRuntimeWithAccess may still panic on registry/workspace
// bootstrap hard failures (crypto/rand-backed auth helpers similarly panic).
func NewWithLoggerContext(ctx context.Context, cfg config.Config, appLogger *logger.Logger) (*App, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	observer := tracepkg.ObserverFromContext(ctx)
	span := tracepkg.Start(ctx, "APP", "app.construct", "Constructing server runtime application", tracepkg.Bool("mcp_http_enabled", cfg.Server.Enabled), tracepkg.Bool("admin_enabled", cfg.Admin.Enabled), tracepkg.Int("tunnel_enabled_count", cfg.EnabledTunnelCount()), tracepkg.Int("tunnel_count", len(cfg.RuntimeTunnels().Instances)))
	pluginhost.Install()
	stream := activity.NewStream()
	configStore := config.NewRuntimeStore(cfg)
	toolSpan := tracepkg.Start(ctx, "APP", "app.tools.bootstrap", "Bootstrapping tool runtime")
	toolRuntime := tools.NewRuntimeWithAccess(cfg.Permissions.AllowDirs, func() (bool, int) {
		current := configStore.Snapshot()
		return current.Admin.Enabled, current.Admin.Port
	})
	toolSpan.EndMessage("Tool runtime bootstrapped", tracepkg.Int("tool_count", len(toolRuntime.List())))
	if toolRuntime.Upstream != nil {
		toolRuntime.Upstream.SetTraceObserver(observer)
	}
	upstreamSpan := tracepkg.Start(ctx, "APP", "app.upstream.bootstrap", "Bootstrapping upstream MCP manager")
	upstreamCount := 0
	if toolRuntime.Upstream != nil {
		upstreamCount = len(toolRuntime.Upstream.List())
	}
	upstreamSpan.EndMessage("Upstream MCP manager bootstrapped", tracepkg.Int("server_count", upstreamCount))
	if err := toolRuntime.SetShellExecutable(cfg.Shell.Executable); err != nil {
		span.FailMessage("Shell provider configuration failed", err)
		return nil, err
	}
	toolRuntime.SetShellPath(cfg.Shell.Path)
	var mcpRuntime *mcp.HTTPRuntime
	if cfg.Server.Enabled {
		mcpRuntime = mcp.NewHTTPRuntimeWithTools(toolRuntime)
		mcpRuntime.Activity = stream
	}
	if appLogger == nil {
		appLogger = logger.New(logger.Info)
	}
	app := &App{
		Config: configStore, MCP: mcpRuntime, Upstream: toolRuntime.Upstream, Tools: toolRuntime, Activity: stream,
		Logger: appLogger, trace: observer,
	}
	bootstrapStarted := time.Now()
	if err := app.Bootstrap(); err != nil {
		span.FailMessage("Server runtime application bootstrap failed", err, tracepkg.Int64("bootstrap_ms", time.Since(bootstrapStarted).Milliseconds()))
		return nil, err
	}
	span.EndMessage("Server runtime application constructed", tracepkg.Int("tool_count", len(app.Tools.List())), tracepkg.Int("upstream_count", upstreamCount), tracepkg.Int64("bootstrap_ms", time.Since(bootstrapStarted).Milliseconds()))
	return app, nil
}

func (a *App) MCPHandler() http.Handler {
	if a == nil || a.MCP == nil {
		return http.NotFoundHandler()
	}
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
		Upstream: a.Upstream, Tools: a.Tools, Tunnels: a.Tunnels, Config: a.Config, ReloadConfig: a.ReloadConfig,
		Approvals: a.Tools.Approvals, Executions: a.Tools.Executions,
	}
	adminAuth := func() (bool, string) {
		cfg := a.Config.Snapshot()
		return cfg.Auth.AdminEnabled, cfg.Auth.AdminTokenHash
	}
	adminHandler := auth.DynamicHashedMiddleware(adminAuth, admin.New(adminAPI))
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		authEnabled := a.Config != nil && a.Config.Snapshot().Auth.AdminEnabled
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"auth_enabled":` + strconv.FormatBool(authEnabled) + `}`))
	})
	mux.Handle("/admin/", adminHandler)
	mux.Handle("/api/", adminHandler)
	mux.Handle("/api/activity/stream", auth.DynamicHashedMiddleware(adminAuth, activity.Handler(a.Activity)))
	mux.Handle("/api/activity/", auth.DynamicHashedMiddleware(adminAuth, activity.CallHandler(a.Activity)))
	mux.Handle("/", web.Handler(a.Tools.PluginStore))
	return web.SecurityHeaders(mux)
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/mcp", a.MCPHandler())
	mux.Handle("/mcp/", a.MCPHandler())
	mux.Handle("/health", a.MCPHandler())
	mux.Handle("/", a.AdminHandler())
	return mux
}
