package tui

import (
	"context"
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"

	"go.mewis.me/chatgpt-mcp/docs/tuiguide"
	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/capability"
	"go.mewis.me/chatgpt-mcp/internal/tui/action"
	tuipage "go.mewis.me/chatgpt-mcp/internal/tui/page"
)

type navigateMsg struct {
	route   Route
	sibling bool
}

func defaultActionRegistry() *action.Registry {
	actions := []action.Action{
		navigationAction("app.go.workspaces", "Workspaces", Route{Kind: RouteWorkspaces}, []string{"workspace", "workspaces", "ws", "container", "containers"}, capability.WorkspaceList, capability.WorkspaceShow, capability.WorkspaceAccessList, capability.WorkspaceContainerList, capability.WorkspaceContainerShow),
		navigationAction("app.go.mcp", "MCP Servers", Route{Kind: RouteMCP}, []string{"mcp", "server", "upstream"}, capability.MCPServerList, capability.MCPServerShow),
		navigationAction("app.go.plugins", "Plugins", Route{Kind: RoutePlugins}, []string{"plugin", "plugins", "installed", "marketplace"}, capability.PluginList, capability.PluginInfo),
		navigationAction("app.go.plugins.marketplace", "Plugin Marketplace", Route{Kind: RoutePlugins, Section: "marketplace"}, []string{"plugin", "marketplace", "search", "install"}, capability.PluginSearch),
		navigationAction("app.go.plugins.updates", "Plugin Updates", Route{Kind: RoutePlugins, Section: "updates"}, []string{"plugin", "update", "outdated"}, capability.PluginOutdated),
		navigationAction("app.go.plugins.registries", "Plugin Registries", Route{Kind: RoutePlugins, Section: "registries"}, []string{"plugin", "registry", "trust"}, capability.PluginRegistryList),
		navigationAction("app.go.tunnel", "Tunnel", Route{Kind: RouteTunnel}, []string{"tunnel", "secure", "admin", "profiles"}, capability.TunnelList, capability.TunnelStatus, capability.TunnelAdminList, capability.TunnelAdminAdd, capability.TunnelAdminVerify, capability.TunnelAdminRemove),
		navigationAction("app.go.admins", "Admin Profiles", Route{Kind: RouteTunnelAdmins}, []string{"tunnel", "admin", "profile", "profiles"}, capability.TunnelAdminList, capability.TunnelAdminAdd, capability.TunnelAdminVerify, capability.TunnelAdminRemove),
		navigationAction("app.go.tunnels", "Managed Tunnels", Route{Kind: RouteTunnels}, []string{"tunnel", "tunnels", "managed", "openai"}, capability.TunnelManagedList, capability.TunnelManagedGet),
		navigationAction("app.go.requests", "Requests", Route{Kind: RouteRequests}, []string{"request", "approval"}, capability.RequestView),
		navigationAction("app.go.logs", "Logs", Route{Kind: RouteLogs}, []string{"logs", "events", "journal"}),
		navigationAction("app.go.logs-exec", "Command Execution", Route{Kind: RouteLogsExec}, []string{"logs", "command", "execution", "exec", "output"}),
		navigationAction("app.go.logs-tools", "Tool Calls", Route{Kind: RouteLogsTools}, []string{"logs", "tools", "calls", "tool calls"}),
		navigationAction("app.go.config", "Config", Route{Kind: RouteConfig}, []string{"config", "settings", "cfg"}, capability.ConfigPath, capability.ConfigGet),
		navigationAction("app.go.instruction", "Instruction", Route{Kind: RouteInstruction}, []string{"instruction", "instructions", "global", "context", "rules", "sources"}),
		navigationAction("app.go.runtime", "Runtime", Route{Kind: RouteRuntime}, []string{"runtime", "status", "service"}, capability.AuthStatus, capability.AliasStatus),
		navigationAction("app.go.about", "About", Route{Kind: RouteAbout}, []string{"about", "version", "build", "uptime"}, capability.VersionAbout),
		navigationAction("app.go.guide", "Guide", Route{Kind: RouteGuide}, []string{"guide", "help", "docs", "documentation"}),
	}
	actions = append(actions, guideActions()...)
	actions = append(actions, instructionNavigationActions()...)
	actions = append(actions, workspaceActions()...)
	actions = append(actions, mcpActions()...)
	actions = append(actions, pluginActions()...)
	actions = append(actions, tunnelActions()...)
	actions = append(actions, requestActions()...)
	actions = append(actions, logsActions()...)
	actions = append(actions, systemActions()...)
	actions = append(actions, configActions()...)
	registry, err := action.NewRegistry(actions...)
	if err != nil {
		panic(err)
	}
	return registry
}

func guideActions() []action.Action {
	topics := tuiguide.Topics()
	actions := make([]action.Action, 0, len(topics))
	for _, topic := range topics {
		topic := topic
		actions = append(actions, action.Action{
			ID: "guide." + strings.ReplaceAll(topic.ID, "/", "."), Title: topic.Title, Category: "Guide", Description: topic.Description,
			Keywords: topic.Keywords, Scope: action.ScopeGlobal,
			Run: func(context.Context, action.Context) tea.Cmd {
				return func() tea.Msg { return navigateMsg{route: Route{Kind: RouteGuide, ResourceID: topic.ID}} }
			},
		})
	}
	return actions
}

func systemActions() []action.Action {
	return []action.Action{
		systemAction("system.refresh", "Refresh system status", "Refresh runtime, service, auth, installation, update, and build state", []string{"system", "runtime", "refresh", "status"}, []string{"status"}, tuipage.SystemRefresh, false),
		systemAction("runtime.up.user", "Start user service", "Install or update and start the per-user managed runtime", []string{"runtime", "service", "up", "user", "start"}, []string{"up"}, tuipage.RuntimeUpUser, false),
		systemAction("runtime.up.system", "Start system service", "Install or update and start the machine-level managed runtime", []string{"runtime", "service", "up", "system", "start"}, []string{"up", "--system"}, tuipage.RuntimeUpSystem, true),
		systemAction("runtime.down.user", "Stop user service", "Stop and remove the per-user managed runtime while preserving config and logs", []string{"runtime", "service", "down", "user", "stop"}, []string{"down"}, tuipage.RuntimeDownUser, false),
		systemAction("runtime.down.system", "Stop system service", "Stop and remove the machine-level managed runtime while preserving config and logs", []string{"runtime", "service", "down", "system", "stop"}, []string{"down", "--system"}, tuipage.RuntimeDownSystem, true),
		systemAction("runtime.restart.user", "Restart user service", "Restart the per-user managed runtime", []string{"runtime", "service", "restart", "user"}, []string{"restart"}, tuipage.RuntimeRestartUser, false),
		systemAction("runtime.restart.system", "Restart system service", "Restart the machine-level managed runtime", []string{"runtime", "service", "restart", "system"}, []string{"restart", "--system"}, tuipage.RuntimeRestartSystem, true),
		systemAction("runtime.foreground", "Run foreground runtime", "Show the foreground serve command to run after leaving the TUI", []string{"runtime", "foreground", "serve", "terminal"}, []string{"serve"}, tuipage.RuntimeForeground, false),
		systemAction("mcp.stdio.foreground", "Run MCP stdio server", "Show the MCP stdio command to run after leaving the TUI", []string{"mcp", "stdio", "cursor", "terminal", "transport"}, []string{"mcp", "stdio"}, tuipage.MCPStdioForeground, false),
		systemAction("mcp.http.foreground", "Run standalone MCP HTTP server", "Show the Streamable HTTP/SSE command to run after leaving the TUI", []string{"mcp", "http", "sse", "transport", "terminal"}, []string{"mcp", "http"}, tuipage.MCPHTTPForeground, false),
		systemAction("transport.mcp-http.enable", "Enable MCP HTTP server", "Enable the local MCP HTTP transport and reload the running runtime", []string{"mcp", "http", "server", "transport", "enable", "listener"}, []string{"config", "set"}, tuipage.MCPHTTPEnable, false),
		systemAction("transport.mcp-http.disable", "Disable MCP HTTP server", "Disable the local MCP HTTP transport and close its listener; the Secure MCP Tunnel must remain enabled", []string{"mcp", "http", "server", "transport", "disable", "listener", "port"}, []string{"config", "set"}, tuipage.MCPHTTPDisable, false),
		systemAction("config.initialize.external", "Initialize configuration", "Show the initialization command that creates configuration and one-time authentication tokens", []string{"config", "init", "initialize", "token"}, []string{"init"}, tuipage.ConfigInitialize, false),
		systemAction("config.uninitialize.external", "Uninitialize configuration", "Show the destructive command that removes local configuration and state", []string{"config", "uninit", "uninitialize", "remove", "state"}, []string{"uninit"}, tuipage.ConfigUninitialize, false),
		systemAction("auth.mcp.enable", "Enable Direct MCP HTTP authentication", "Protects direct /mcp HTTP only. Secure MCP Tunnel is unaffected.", []string{"auth", "mcp", "enable", "direct"}, []string{"auth", "mcp", "enable"}, tuipage.AuthMCPEnable, false),
		systemAction("auth.mcp.disable", "Disable Direct MCP HTTP authentication", "Disables bearer auth for direct /mcp HTTP only. Secure MCP Tunnel is unaffected.", []string{"auth", "mcp", "disable", "direct"}, []string{"auth", "mcp", "disable"}, tuipage.AuthMCPDisable, false),
		systemAction("auth.mcp.show", "Reveal Direct MCP HTTP token", "Show the stored Direct MCP HTTP token. Reuse it when adding this MCP server to ChatGPT.", []string{"auth", "mcp", "token", "show", "reveal", "direct"}, []string{"auth", "mcp", "show"}, tuipage.AuthMCPShow, false),
		systemAction("auth.mcp.copy", "Copy Direct MCP HTTP token", "Copy the stored Direct MCP HTTP token without rotating it.", []string{"auth", "mcp", "token", "copy", "clipboard", "direct"}, []string{"auth", "mcp", "copy"}, tuipage.AuthMCPCopy, false),
		systemAction("auth.mcp.rotate", "Rotate Direct MCP HTTP token", "Generate a new Direct MCP HTTP token. Reuse the current token when adding this MCP server to ChatGPT.", []string{"auth", "mcp", "token", "rotate", "create", "direct"}, []string{"auth", "mcp", "rotate"}, tuipage.AuthMCPRotate, false),
		systemAction("auth.admin.enable", "Enable admin authentication", "Enable admin token authentication", []string{"auth", "admin", "enable"}, []string{"auth", "admin", "enable"}, tuipage.AuthAdminEnable, false),
		systemAction("auth.admin.disable", "Disable admin authentication", "Disable admin token authentication", []string{"auth", "admin", "disable"}, []string{"auth", "admin", "disable"}, tuipage.AuthAdminDisable, false),
		systemAction("auth.admin.rotate", "Rotate admin token", "Rotate the admin token and reveal the replacement once", []string{"auth", "admin", "token", "rotate", "create"}, []string{"auth", "admin", "create"}, tuipage.AuthAdminRotate, false),
		systemAction("alias.install", "Install cgm alias", "Install the cgm alias for the managed direct installation", []string{"alias", "cgm", "install"}, []string{"alias", "install"}, tuipage.AliasInstall, false),
		systemAction("alias.remove", "Remove cgm alias", "Remove the cgm alias without removing the managed installation", []string{"alias", "cgm", "remove"}, []string{"alias", "remove"}, tuipage.AliasRemove, false),
		editorNavigationAction("install.run", "Install managed binary", "System", "Install this binary into the versioned managed layout", []string{"install", "managed", "binary"}, []string{"install"}, func(ctx action.Context) bool { return ctx.Route == string(RouteRuntime) }, func(action.Context) Route { return Route{Kind: RouteRuntime, Action: "install"} }),
		systemAction("install.cleanup", "Clean legacy installations", "Remove verified legacy standalone installations from PATH", []string{"install", "cleanup", "migrate", "legacy"}, []string{"install", "cleanup"}, tuipage.InstallCleanup, false),
		systemAction("update.check", "Check for upgrades", "Check the latest available verified release", []string{"upgrade", "update", "check", "latest", "release"}, []string{"upgrade", "check"}, tuipage.UpdateCheck, false),
		editorNavigationAction("update.apply", "Apply upgrade", "System", "Download, verify, install, and activate an upgrade", []string{"upgrade", "update", "apply", "install", "release"}, []string{"upgrade"}, func(ctx action.Context) bool { return ctx.Route == string(RouteRuntime) }, func(action.Context) Route { return Route{Kind: RouteRuntime, Action: "update"} }),
	}
}

func systemAction(id, title, description string, keywords, commandPath []string, command tuipage.SystemCommand, systemOnly bool) action.Action {
	return action.Action{
		ID: id, Title: title, Category: "System", Description: description, Keywords: keywords, CommandPath: commandPath, Capabilities: capabilitiesForCommandPath(commandPath), Scope: action.ScopeGlobal,
		Available: func(ctx action.Context) bool {
			return ctx.Route == string(RouteRuntime) && (!systemOnly || runtime.GOOS != "windows")
		},
		Run: func(context.Context, action.Context) tea.Cmd {
			return func() tea.Msg { return tuipage.SystemCommandMsg{Command: command} }
		},
	}
}

func logsActions() []action.Action {
	return []action.Action{
		logsAction("logs.refresh", "Refresh logs", "Reload journal history and reconnect the live stream", []string{"logs", "refresh", "history", "reconnect"}, []string{"logs"}, tuipage.LogsRefresh),
		editorNavigationAction("logs.filter", "Filter logs", "Logs", "Configure structured runtime log filters", []string{"logs", "filter", "grep", "session", "level"}, []string{"logs"}, func(ctx action.Context) bool { return ctx.Route == string(RouteLogs) }, func(action.Context) Route { return Route{Kind: RouteLogs, Action: "filter"} }),
		logsAction("logs.toggle", "Pause or resume logs", "Toggle live tail following without dropping buffered events", []string{"logs", "pause", "resume", "follow"}, []string{"logs", "follow"}, tuipage.LogsToggle),
		logsAction("logs.info", "Show logs info", "Show the runtime journal path, file count, and size", []string{"logs", "path", "info", "journal"}, []string{"logs", "path"}, tuipage.LogsInfo),
		logsAction("logs.clear", "Clear logs", "Clear current and rotated runtime logs after confirmation", []string{"logs", "clear", "delete"}, []string{"logs", "clear"}, tuipage.LogsClear),
	}
}

func logsAction(id, title, description string, keywords, commandPath []string, command tuipage.LogsCommand) action.Action {
	return action.Action{
		ID: id, Title: title, Category: "Logs", Description: description, Keywords: keywords, CommandPath: commandPath, Capabilities: capabilitiesForCommandPath(commandPath), Scope: action.ScopeGlobal,
		Available: func(ctx action.Context) bool { return ctx.Route == string(RouteLogs) },
		Run: func(context.Context, action.Context) tea.Cmd {
			return func() tea.Msg { return tuipage.LogsCommandMsg{Command: command} }
		},
	}
}

func configActions() []action.Action {
	return []action.Action{
		configAction("config.refresh", "Refresh config", "Reload persisted configuration and runtime state", []string{"config", "refresh", "reload", "view"}, []string{"config", "list"}, tuipage.ConfigRefresh),
		configAction("config.edit", "Edit config field", "Edit the selected typed configuration field", []string{"config", "edit", "set", "field"}, []string{"config", "set"}, tuipage.ConfigEdit),
		configAction("config.verify", "Verify config", "Verify structured config/state format consistency and configuration validity", []string{"config", "verify", "validate"}, []string{"config", "verify"}, tuipage.ConfigVerify),
		configAction("config.migrate", "Migrate config secrets", "Migrate legacy plaintext credentials into the secret store", []string{"config", "migrate", "secrets"}, []string{"config", "migrate"}, tuipage.ConfigMigrate),
		configAction("config.migrate.secrets", "Encrypt secret files", "Encrypt plaintext secret-store files at rest", []string{"config", "migrate", "secrets", "encrypt"}, []string{"config", "migrate", "secrets"}, tuipage.ConfigMigrateSecrets),
		editorNavigationAction("config.convert", "Convert config format", "Config", "Convert structured config/state files between JSON, YAML, and TOML", []string{"config", "convert", "transform", "format"}, []string{"config", "convert"}, func(ctx action.Context) bool { return ctx.Route == string(RouteConfig) }, func(action.Context) Route { return Route{Kind: RouteConfig, Section: "storage", Action: "convert"} }),
		editorNavigationAction("config.export", "Export config bundle", "Config", "Export portable configuration, state, and secrets into a sealed bundle", []string{"config", "export", "bundle", "backup"}, []string{"config", "export"}, func(ctx action.Context) bool { return ctx.Route == string(RouteConfig) }, func(action.Context) Route { return Route{Kind: RouteConfig, Section: "storage", Action: "export"} }),
		editorNavigationAction("config.import", "Import config bundle", "Config", "Import a portable configuration bundle and restore its secrets", []string{"config", "import", "bundle", "restore"}, []string{"config", "import"}, func(ctx action.Context) bool { return ctx.Route == string(RouteConfig) }, func(action.Context) Route { return Route{Kind: RouteConfig, Section: "storage", Action: "import"} }),
	}
}

func configAction(id, title, description string, keywords, commandPath []string, command tuipage.ConfigCommand) action.Action {
	return action.Action{
		ID: id, Title: title, Category: "Config", Description: description, Keywords: keywords, CommandPath: commandPath, Capabilities: capabilitiesForCommandPath(commandPath), Scope: action.ScopeGlobal,
		Available: func(ctx action.Context) bool { return ctx.Route == string(RouteConfig) },
		Run: func(_ context.Context, ctx action.Context) tea.Cmd {
			return func() tea.Msg { return tuipage.ConfigCommandMsg{Command: command, ResourceID: ctx.ResourceID} }
		},
	}
}

func requestActions() []action.Action {
	return []action.Action{
		requestAction("request.refresh", "Refresh requests", "Refresh approval requests from the running runtime", []string{"request", "approval", "refresh", "list"}, []string{"request", "list"}, tuipage.RequestRefresh, false),
		editorNavigationAction("request.create.test", "Create test request", "Requests", "Create a synthetic approval request for testing the Requests TUI and approval flow", []string{"request", "approval", "create", "test", "dummy", "synthetic"}, []string{"request", "create-test"}, func(ctx action.Context) bool { return ctx.Route == string(RouteRequests) }, func(action.Context) Route { return Route{Kind: RouteRequests, Action: "create-test"} }),
		requestAction("request.show.pending", "Show pending requests", "Show only pending approval requests", []string{"request", "pending", "filter"}, []string{"request", "list"}, tuipage.RequestShowPending, false),
		requestAction("request.show.history", "Show request history", "Show resolved and expired approval requests", []string{"request", "history", "resolved", "filter"}, []string{"request", "list"}, tuipage.RequestShowHistory, false),
		requestAction("request.show.all", "Show all requests", "Show pending and historical approval requests", []string{"request", "all", "filter"}, []string{"request", "list"}, tuipage.RequestShowAll, false),
		requestAction("request.approve", "Approve request", "Approve the selected pending control request", []string{"request", "approve", "allow", "accept"}, []string{"request", "approve"}, tuipage.RequestApprove, false),
		requestAction("request.deny", "Deny request", "Deny the selected pending control request", []string{"request", "deny", "reject"}, []string{"request", "deny"}, tuipage.RequestDeny, false),
		requestAction("request.grant.list", "List runtime grants", "List active similar-command runtime session grants", []string{"request", "grant", "list", "similar"}, []string{"request", "grant", "list"}, tuipage.RequestGrantList, false),
		requestAction("request.grant.revoke", "Revoke runtime grant", "Revoke the selected similar-command runtime session grant", []string{"request", "grant", "revoke", "similar"}, []string{"request", "grant", "revoke"}, tuipage.RequestGrantRevoke, true),
	}
}

func requestAction(id, title, description string, keywords, commandPath []string, command tuipage.RequestCommand, needsResource bool) action.Action {
	return action.Action{
		ID: id, Title: title, Category: "Requests", Description: description, Keywords: keywords, CommandPath: commandPath, Capabilities: capabilitiesForCommandPath(commandPath), Scope: action.ScopeGlobal,
		Available: func(ctx action.Context) bool {
			if ctx.Route != string(RouteRequests) {
				return false
			}
			return !needsResource || ctx.ResourceID != ""
		},
		Run: func(_ context.Context, ctx action.Context) tea.Cmd {
			return func() tea.Msg { return tuipage.RequestCommandMsg{Command: command, ResourceID: ctx.ResourceID} }
		},
	}
}

func tunnelActions() []action.Action {
	return []action.Action{
		localTunnelAction("tunnel.enable", "Enable tunnel", "Enable the current local tunnel instance", []string{"tunnel", "enable", "runtime"}, []string{"tunnel", "enable"}, tuipage.LocalTunnelEnable),
		localTunnelAction("tunnel.disable", "Disable tunnel", "Disable the current local tunnel instance", []string{"tunnel", "disable", "runtime"}, []string{"tunnel", "disable"}, tuipage.LocalTunnelDisable),
		localTunnelAction("tunnel.start", "Start tunnel", "Start the current local tunnel connection", []string{"tunnel", "start", "connect"}, []string{"tunnel", "start"}, tuipage.LocalTunnelStart),
		localTunnelAction("tunnel.stop", "Stop tunnel", "Stop the current local tunnel connection", []string{"tunnel", "stop", "disconnect"}, []string{"tunnel", "stop"}, tuipage.LocalTunnelStop),
		localTunnelAction("tunnel.detach", "Detach tunnel", "Remove the current local tunnel instance without deleting the remote tunnel", []string{"tunnel", "detach", "remove"}, []string{"tunnel", "detach"}, tuipage.LocalTunnelDetach),
		tunnelAction("tunnel.foreground", "Run foreground tunnel", "Show the foreground tunnel command to run after leaving the TUI", []string{"tunnel", "foreground", "run", "terminal"}, []string{"tunnel", "run"}, tuipage.TunnelForeground, RouteTunnel, true),
		editorNavigationAction("tunnel.admin.add", "Add admin profile", "Tunnel", "Add an OpenAI admin profile for tunnel management", []string{"tunnel", "admin", "profile", "add"}, []string{"tunnel", "admin", "add"}, func(ctx action.Context) bool {
			return ctx.Route == string(RouteTunnel) || ctx.Route == string(RouteTunnelAdmins) || ctx.Route == string(RouteTunnels)
		}, func(action.Context) Route { return Route{Kind: RouteTunnelAdmins, Action: "create"} }),
		editorNavigationAction("tunnel.admin.update", "Update admin profile", "Tunnel", "Update the current tunnel admin profile", []string{"tunnel", "admin", "profile", "edit"}, []string{"tunnel", "admin", "add"}, func(ctx action.Context) bool {
			return ctx.Route == string(RouteTunnelAdmins) && ctx.ResourceID != ""
		}, func(ctx action.Context) Route {
			return Route{Kind: RouteTunnelAdmins, ResourceID: ctx.ResourceID, Action: "edit"}
		}),
		tunnelAdminAction("tunnel.admin.verify", "Verify admin profile", "Verify the current tunnel admin profile against OpenAI", []string{"tunnel", "admin", "verify"}, []string{"tunnel", "admin", "verify"}, tuipage.TunnelAdminVerify, true),
		tunnelAdminAction("tunnel.admin.remove", "Remove admin profile", "Remove the current tunnel admin profile", []string{"tunnel", "admin", "remove", "delete"}, []string{"tunnel", "admin", "remove"}, tuipage.TunnelAdminRemove, true),
		tunnelAction("tunnel.managed.refresh", "Refresh managed tunnels", "Refresh managed tunnels from all readable admin profiles", []string{"tunnel", "managed", "refresh", "list"}, []string{"tunnel", "managed", "list"}, tuipage.TunnelManagedRefresh, RouteTunnels, false),
		editorNavigationAction("tunnel.managed.create", "Create managed tunnel", "Tunnel", "Create a tunnel through an OpenAI admin profile", []string{"tunnel", "managed", "create"}, []string{"tunnel", "managed", "create"}, func(ctx action.Context) bool {
			return ctx.Route == string(RouteTunnels) && tunnelAdminManageAvailable()
		}, func(action.Context) Route { return Route{Kind: RouteTunnels, Action: "create"} }),
		editorNavigationAction("tunnel.managed.update", "Update managed tunnel", "Tunnel", "Update the current managed tunnel", []string{"tunnel", "managed", "update", "edit"}, []string{"tunnel", "managed", "update"}, func(ctx action.Context) bool {
			return ctx.Route == string(RouteTunnels) && ctx.ResourceID != "" && tunnelAdminManageAvailable()
		}, func(ctx action.Context) Route {
			return Route{Kind: RouteTunnels, ResourceID: ctx.ResourceID, Action: "edit"}
		}),
		editorNavigationAction("tunnel.managed.configure", "Attach managed tunnel", "Tunnel", "Attach the current managed tunnel to the shared local runtime", []string{"tunnel", "managed", "attach", "runtime"}, []string{"tunnel", "attach"}, func(ctx action.Context) bool {
			return ctx.Route == string(RouteTunnels) && ctx.ResourceID != "" && tunnelAdminReadAvailable()
		}, func(ctx action.Context) Route {
			return Route{Kind: RouteTunnels, ResourceID: ctx.ResourceID, Action: "configure"}
		}),
		tunnelAction("tunnel.managed.delete", "Delete managed tunnel", "Permanently delete the current managed tunnel", []string{"tunnel", "managed", "delete", "remove"}, []string{"tunnel", "managed", "delete"}, tuipage.TunnelManagedDelete, RouteTunnels, true),
	}
}

func tunnelAdminAction(id, title, description string, keywords, commandPath []string, command tuipage.TunnelAdminCommand, needsResource bool) action.Action {
	return action.Action{
		ID: id, Title: title, Category: "Tunnel", Description: description, Keywords: keywords, CommandPath: commandPath, Capabilities: capabilitiesForCommandPath(commandPath), Scope: action.ScopeGlobal,
		Available: func(ctx action.Context) bool {
			return ctx.Route == string(RouteTunnelAdmins) && (!needsResource || ctx.ResourceID != "")
		},
		Run: func(_ context.Context, ctx action.Context) tea.Cmd {
			return func() tea.Msg { return tuipage.TunnelAdminCommandMsg{Command: command, ResourceID: ctx.ResourceID} }
		},
	}
}

func localTunnelAction(id, title, description string, keywords, commandPath []string, command tuipage.LocalTunnelCommand) action.Action {
	return action.Action{
		ID: id, Title: title, Category: "Tunnel", Description: description, Keywords: keywords, CommandPath: commandPath, Capabilities: capabilitiesForCommandPath(commandPath), Scope: action.ScopeGlobal,
		Available: func(ctx action.Context) bool { return ctx.Route == string(RouteTunnel) && ctx.ResourceID != "" },
		Run: func(_ context.Context, ctx action.Context) tea.Cmd {
			return func() tea.Msg { return tuipage.LocalTunnelCommandMsg{Command: command, ResourceID: ctx.ResourceID} }
		},
	}
}

func tunnelAction(id, title, description string, keywords, commandPath []string, command tuipage.TunnelCommand, route RouteKind, needsResource bool) action.Action {
	return action.Action{
		ID: id, Title: title, Category: "Tunnel", Description: description, Keywords: keywords, CommandPath: commandPath, Capabilities: capabilitiesForCommandPath(commandPath), Scope: action.ScopeGlobal,
		Available: func(ctx action.Context) bool {
			if ctx.Route != string(route) || needsResource && ctx.ResourceID == "" {
				return false
			}
			switch command {
			case tuipage.TunnelManagedRefresh:
				return tunnelAdminReadAvailable()
			case tuipage.TunnelManagedCreate, tuipage.TunnelManagedUpdate, tuipage.TunnelManagedDelete:
				return tunnelAdminManageAvailable()
			case tuipage.TunnelManagedConfigure:
				return tunnelAdminReadAvailable()
			}
			return true
		},
		Run: func(_ context.Context, ctx action.Context) tea.Cmd {
			return func() tea.Msg { return tuipage.TunnelCommandMsg{Command: command, ResourceID: ctx.ResourceID} }
		},
	}
}

func tunnelAdminReadAvailable() bool {
	profiles, err := application.TunnelAdminProfiles()
	if err != nil {
		return false
	}
	for _, profile := range profiles {
		if profile.ReadAccess || profile.ManageAccess {
			return true
		}
	}
	return false
}
func tunnelAdminManageAvailable() bool {
	profiles, err := application.TunnelAdminProfiles()
	if err != nil {
		return false
	}
	for _, profile := range profiles {
		if profile.ManageAccess {
			return true
		}
	}
	return false
}

func mcpActions() []action.Action {
	return []action.Action{
		editorNavigationAction("mcp.server.add", "Add server", "MCP", "Add an upstream MCP server", []string{"mcp", "server", "add", "upstream"}, []string{"mcp", "server", "add"}, nil, func(action.Context) Route { return Route{Kind: RouteMCP, Action: "create"} }),
		editorNavigationAction("mcp.server.configure", "Configure server", "MCP", "Configure the current upstream MCP server", []string{"mcp", "server", "configure", "set"}, []string{"mcp", "server", "configure"}, func(ctx action.Context) bool { return ctx.Route == string(RouteMCP) && ctx.ResourceID != "" }, func(ctx action.Context) Route {
			return Route{Kind: RouteMCP, ResourceID: ctx.ResourceID, Action: "edit"}
		}),
		mcpAction("mcp.server.remove", "Remove server", "Remove the current upstream MCP server", []string{"mcp", "server", "remove", "delete"}, []string{"mcp", "server", "remove"}, tuipage.MCPServerRemove, true),
		mcpAction("mcp.server.enable", "Enable server", "Enable the current upstream MCP server", []string{"mcp", "server", "enable"}, []string{"mcp", "server", "enable"}, tuipage.MCPServerEnable, true),
		mcpAction("mcp.server.disable", "Disable server", "Disable the current upstream MCP server", []string{"mcp", "server", "disable"}, []string{"mcp", "server", "disable"}, tuipage.MCPServerDisable, true),
		mcpAction("mcp.server.status", "Refresh health", "Refresh upstream MCP health and connection status", []string{"mcp", "server", "status", "health", "refresh"}, []string{"mcp", "server", "status"}, tuipage.MCPServerHealth, false),
		mcpAction("mcp.server.tools", "View tools", "Load tools exposed by the current upstream MCP server", []string{"mcp", "server", "tools", "refresh"}, []string{"mcp", "server", "tools"}, tuipage.MCPServerTools, true),
	}
}

func pluginActions() []action.Action {
	return []action.Action{
		pluginAction("plugin.install", "Install plugin", "Install the selected marketplace plugin", []string{"plugin", "install", "marketplace"}, []string{"plugin", "install"}, tuipage.PluginInstall, "marketplace", true),
		pluginAction("plugin.uninstall", "Uninstall plugin", "Uninstall the current plugin", []string{"plugin", "uninstall", "remove"}, []string{"plugin", "uninstall"}, tuipage.PluginUninstall, "", true),
		pluginAction("plugin.enable", "Enable plugin", "Enable the current plugin", []string{"plugin", "enable"}, []string{"plugin", "enable"}, tuipage.PluginEnable, "", true),
		pluginAction("plugin.disable", "Disable plugin", "Disable the current plugin", []string{"plugin", "disable"}, []string{"plugin", "disable"}, tuipage.PluginDisable, "", true),
		pluginAction("plugin.update", "Update plugin", "Update the current plugin to the latest available version", []string{"plugin", "update", "upgrade"}, []string{"plugin", "update"}, tuipage.PluginUpdate, "", true),
		pluginAction("plugin.rollback", "Rollback plugin", "Roll back the current plugin to a retained version", []string{"plugin", "rollback"}, []string{"plugin", "rollback"}, tuipage.PluginRollback, "", true),
		pluginAction("plugin.prune", "Prune plugin versions", "Prune retained inactive versions for the current plugin", []string{"plugin", "prune", "retain"}, []string{"plugin", "prune"}, tuipage.PluginPrune, "", true),
		pluginAction("plugin.verify", "Verify plugin", "Re-verify the current plugin integrity and trust chain", []string{"plugin", "verify"}, []string{"plugin", "verify"}, tuipage.PluginVerify, "", true),
		{
			ID: "plugin.configure", Title: "Configure plugin", Category: "Plugins", Description: "Edit the selected plugin's configuration",
			Keywords: []string{"plugin", "config", "configure", "settings"}, CommandPath: []string{"plugin", "config", "set"},
			Capabilities: []capability.ID{capability.PluginConfigList, capability.PluginConfigGet, capability.PluginConfigSet},
			Scope:        action.ScopeGlobal,
			Available: func(ctx action.Context) bool {
				return ctx.Route == string(RoutePlugins) && ctx.ResourceID != "" && ctx.Section == ""
			},
			Run: func(_ context.Context, ctx action.Context) tea.Cmd {
				return func() tea.Msg {
					return navigateMsg{route: Route{Kind: RoutePlugins, ResourceID: ctx.ResourceID, Action: "configure"}}
				}
			},
		},
		pluginAction("plugin.config.reset", "Reset plugin configuration", "Reset the selected plugin's configuration to schema defaults", []string{"plugin", "config", "reset", "defaults"}, []string{"plugin", "config", "reset"}, tuipage.PluginConfigReset, "", true),
		editorNavigationAction("plugin.registry.add", "Add plugin registry", "Plugins", "Add a trusted third-party plugin registry", []string{"plugin", "registry", "add", "trust"}, []string{"plugin", "registry", "add"}, nil, func(action.Context) Route {
			return Route{Kind: RoutePlugins, Section: "registries", Action: "add"}
		}),
		pluginAction("plugin.registry.remove", "Remove plugin registry", "Remove the selected third-party plugin registry", []string{"plugin", "registry", "remove", "delete"}, []string{"plugin", "registry", "remove"}, tuipage.PluginRegistryRemove, "registries", true),
	}
}

func pluginAction(id, title, description string, keywords, commandPath []string, command tuipage.PluginCommand, section string, needsResource bool) action.Action {
	return action.Action{
		ID: id, Title: title, Category: "Plugins", Description: description, Keywords: keywords, CommandPath: commandPath, Capabilities: capabilitiesForCommandPath(commandPath), Scope: action.ScopeGlobal,
		Available: func(ctx action.Context) bool {
			if ctx.Route != string(RoutePlugins) {
				return false
			}
			if needsResource && ctx.ResourceID == "" {
				return false
			}
			switch section {
			case "marketplace", "registries":
				return ctx.Section == section
			default:
				return ctx.Section == "" || ctx.Section == "updates"
			}
		},
		Run: func(_ context.Context, ctx action.Context) tea.Cmd {
			return func() tea.Msg { return tuipage.PluginCommandMsg{Command: command, TargetID: ctx.ResourceID} }
		},
	}
}

func mcpAction(id, title, description string, keywords, commandPath []string, command tuipage.MCPCommand, needsResource bool) action.Action {
	return action.Action{
		ID: id, Title: title, Category: "MCP", Description: description, Keywords: keywords, CommandPath: commandPath, Capabilities: capabilitiesForCommandPath(commandPath), Scope: action.ScopeGlobal,
		Available: func(ctx action.Context) bool {
			return !needsResource || ctx.Route == string(RouteMCP) && ctx.ResourceID != ""
		},
		Run: func(_ context.Context, ctx action.Context) tea.Cmd {
			return func() tea.Msg { return tuipage.MCPCommandMsg{Command: command, ResourceID: ctx.ResourceID} }
		},
	}
}

func workspaceActions() []action.Action {
	return []action.Action{
		workspaceContextNavigationAction("workspace.context.configure", "Configure Workspace Project Context", "Configure build and preview parameters for the current workspace", "context"),
		workspaceContextNavigationAction("workspace.context.preview", "Preview Workspace Project Context", "Open the last successful Project Context build for the current workspace", "context-preview"),
		editorNavigationAction("workspace.register", "Register", "Workspace", "Register a workspace root", []string{"workspace", "register"}, []string{"workspace", "register"}, nil, func(action.Context) Route { return Route{Kind: RouteWorkspaces, Action: "register"} }),
		editorNavigationAction("workspace.relocate", "Relocate", "Workspace", "Rebind the current workspace after its project directory was renamed or moved", []string{"workspace", "relocate", "move", "rename", "root"}, []string{"workspace", "relocate"}, func(ctx action.Context) bool { return ctx.Route == string(RouteWorkspaces) && ctx.ResourceID != "" }, func(ctx action.Context) Route {
			return Route{Kind: RouteWorkspaces, ResourceID: ctx.ResourceID, Action: "relocate"}
		}),
		workspaceAction("workspace.unregister", "Unregister", "Unregister the current workspace without deleting .cgm or project files", []string{"workspace", "unregister"}, []string{"workspace", "unregister"}, tuipage.WorkspaceUnregister, true, false),
		workspaceAction("workspace.purge", "Delete local state", "Unregister the current workspace and delete its .cgm directory", []string{"workspace", "purge", "delete-state", "cgm"}, []string{"workspace", "purge"}, tuipage.WorkspaceDeleteState, true, false),
		editorNavigationAction("workspace.access.add", "Add access directory", "Workspace", "Grant the current workspace access to an additional directory", []string{"workspace", "access", "add"}, []string{"workspace", "access", "add"}, func(ctx action.Context) bool { return ctx.Route == string(RouteWorkspaces) && ctx.ResourceID != "" }, func(ctx action.Context) Route {
			return Route{Kind: RouteWorkspaces, ResourceID: ctx.ResourceID, Section: "access", Action: "add"}
		}),
		editorNavigationAction("workspace.access.remove", "Remove access directory", "Workspace", "Revoke an additional directory from the current workspace", []string{"workspace", "access", "remove"}, []string{"workspace", "access", "remove"}, func(ctx action.Context) bool { return ctx.Route == string(RouteWorkspaces) && ctx.ResourceID != "" }, func(ctx action.Context) Route {
			return Route{Kind: RouteWorkspaces, ResourceID: ctx.ResourceID, Section: "access", Action: "remove"}
		}),
		editorNavigationAction("workspace.container.create", "Create container", "Workspace", "Create a workspace container", []string{"workspace", "container", "create"}, []string{"workspace", "container", "create"}, nil, func(action.Context) Route { return Route{Kind: RouteContainers, Action: "create"} }),
		editorNavigationAction("workspace.container.rename", "Rename container", "Workspace", "Rename the current workspace container", []string{"workspace", "container", "rename"}, []string{"workspace", "container", "rename"}, func(ctx action.Context) bool { return ctx.Route == string(RouteContainers) && ctx.ResourceID != "" }, func(ctx action.Context) Route {
			return Route{Kind: RouteContainers, ResourceID: ctx.ResourceID, Action: "edit"}
		}),
		workspaceAction("workspace.container.delete", "Delete container", "Delete the current container without unregistering workspaces", []string{"workspace", "container", "delete"}, []string{"workspace", "container", "delete"}, tuipage.WorkspaceContainerDelete, true, true),
		workspaceAction("workspace.container.add", "Add container members", "Edit workspace membership for the current container", []string{"workspace", "container", "add", "members"}, []string{"workspace", "container", "add"}, tuipage.WorkspaceContainerMembers, true, true),
		workspaceAction("workspace.container.remove", "Remove container members", "Edit workspace membership for the current container", []string{"workspace", "container", "remove", "members"}, []string{"workspace", "container", "remove"}, tuipage.WorkspaceContainerMembers, true, true),
	}
}

func instructionNavigationActions() []action.Action {
	return []action.Action{
		instructionNavigationAction("instruction.open.context", "Open Global Context", "Open managed global instruction context", "context", []string{"instruction", "global", "context"}),
		instructionNavigationAction("instruction.open.rules", "Open Global Rules", "Open managed global instruction rules", "rules", []string{"instruction", "global", "rules"}),
		instructionNavigationAction("instruction.open.sources", "Open Instruction Sources", "Open detected user-level instruction sources and source policy", "sources", []string{"instruction", "sources", "policy", "agents", "claude"}),
	}
}

func instructionNavigationAction(id, title, description, section string, keywords []string) action.Action {
	return action.Action{
		ID: id, Title: title, Category: "Instruction", Description: description, Keywords: keywords, Scope: action.ScopeGlobal,
		Run: func(context.Context, action.Context) tea.Cmd {
			return func() tea.Msg {
				return navigateMsg{route: Route{Kind: RouteInstruction, Section: section}, sibling: true}
			}
		},
	}
}

func workspaceContextNavigationAction(id, title, description, section string) action.Action {
	return action.Action{
		ID: id, Title: title, Category: "Workspace", Description: description, Keywords: []string{"workspace", "project", "context", section}, Scope: action.ScopeGlobal,
		Available: func(ctx action.Context) bool { return ctx.Route == string(RouteWorkspaces) && ctx.ResourceID != "" },
		Run: func(_ context.Context, ctx action.Context) tea.Cmd {
			return func() tea.Msg {
				return navigateMsg{route: Route{Kind: RouteWorkspaces, ResourceID: ctx.ResourceID, Section: section}}
			}
		},
	}
}

func workspaceAction(id, title, description string, keywords, commandPath []string, command tuipage.WorkspaceCommand, needsResource, container bool) action.Action {
	return action.Action{
		ID: id, Title: title, Category: "Workspace", Description: description, Keywords: keywords, CommandPath: commandPath, Capabilities: capabilitiesForCommandPath(commandPath), Scope: action.ScopeGlobal,
		Available: func(ctx action.Context) bool {
			if !needsResource {
				return true
			}
			want := string(RouteWorkspaces)
			if container {
				want = string(RouteContainers)
			}
			return ctx.Route == want && ctx.ResourceID != ""
		},
		Run: func(_ context.Context, ctx action.Context) tea.Cmd {
			return func() tea.Msg { return tuipage.WorkspaceCommandMsg{Command: command, ResourceID: ctx.ResourceID} }
		},
	}
}

func navigationAction(id, title string, route Route, keywords []string, capabilities ...capability.ID) action.Action {
	return action.Action{
		ID: id, Title: "Go to " + title, Category: "App", Description: "Open the " + title + " page", Keywords: keywords,
		CommandPath: []string{"tui", string(route.Kind)}, Capabilities: capabilities, Scope: action.ScopeGlobal,
		Run: func(context.Context, action.Context) tea.Cmd {
			return func() tea.Msg { return navigateMsg{route: route, sibling: true} }
		},
	}
}

func editorNavigationAction(id, title, category, description string, keywords, commandPath []string, available func(action.Context) bool, route func(action.Context) Route) action.Action {
	return action.Action{
		ID: id, Title: title, Category: category, Description: description, Keywords: keywords, CommandPath: commandPath, Capabilities: capabilitiesForCommandPath(commandPath), Scope: action.ScopeGlobal, Available: available,
		Run: func(_ context.Context, ctx action.Context) tea.Cmd {
			return func() tea.Msg { return navigateMsg{route: route(ctx)} }
		},
	}
}

func capabilitiesForCommandPath(commandPath []string) []capability.ID {
	id, ok := capability.ForPath(strings.Join(commandPath, " "))
	if !ok {
		return nil
	}
	return []capability.ID{id}
}

func actionContext(route Route) action.Context {
	return action.Context{Route: string(route.Kind), Mode: route.Mode, ResourceID: route.ResourceID, Section: route.Section, Action: route.Action}
}
