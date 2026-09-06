package tui

import (
	"context"
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"
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
		navigationAction("app.go.workspaces", "Workspaces", Route{Kind: RouteWorkspaces}, []string{"workspace", "workspaces", "ws"}, capability.WorkspaceList, capability.WorkspaceShow, capability.WorkspaceAccessList),
		navigationAction("app.go.containers", "Containers", Route{Kind: RouteContainers}, []string{"workspace", "container", "containers"}, capability.WorkspaceContainerList, capability.WorkspaceContainerShow),
		navigationAction("app.go.mcp", "MCP Servers", Route{Kind: RouteMCP}, []string{"mcp", "server", "upstream"}, capability.MCPServerList, capability.MCPServerShow, capability.MCPAuthStatus),
		navigationAction("app.go.tunnel", "Tunnel", Route{Kind: RouteTunnel}, []string{"tunnel", "secure"}, capability.TunnelStatus, capability.TunnelAdminKeyStatus),
		navigationAction("app.go.tunnels", "Managed Tunnels", Route{Kind: RouteTunnels}, []string{"tunnel", "tunnels", "managed", "openai"}, capability.TunnelList, capability.TunnelGet),
		navigationAction("app.go.requests", "Requests", Route{Kind: RouteRequests}, []string{"request", "approval"}, capability.RequestView),
		navigationAction("app.go.logs", "Logs", Route{Kind: RouteLogs}, []string{"logs", "events", "journal"}),
		navigationAction("app.go.config", "Config", Route{Kind: RouteConfig}, []string{"config", "settings", "cfg"}, capability.ConfigPath, capability.ConfigGet),
		navigationAction("app.go.runtime", "Runtime", Route{Kind: RouteRuntime}, []string{"runtime", "status", "service"}, capability.AuthStatus, capability.AliasStatus),
		navigationAction("app.go.about", "About", Route{Kind: RouteAbout}, []string{"about", "version", "build", "uptime"}, capability.VersionAbout),
	}
	actions = append(actions, workspaceActions()...)
	actions = append(actions, mcpActions()...)
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

func systemActions() []action.Action {
	return []action.Action{
		systemAction("system.refresh", "Refresh system status", "Refresh runtime, service, auth, installation, update, and build state", []string{"system", "runtime", "refresh", "status"}, []string{"status"}, tuipage.SystemRefresh, false),
		systemAction("runtime.up.user", "Start user service", "Install or update and start the per-user managed runtime", []string{"runtime", "service", "up", "user", "start"}, []string{"up"}, tuipage.RuntimeUpUser, false),
		systemAction("runtime.up.system", "Start system service", "Install or update and start the machine-level managed runtime", []string{"runtime", "service", "up", "system", "start"}, []string{"up", "--system"}, tuipage.RuntimeUpSystem, true),
		systemAction("runtime.down.user", "Stop user service", "Stop and remove the per-user managed runtime while preserving config and logs", []string{"runtime", "service", "down", "user", "stop"}, []string{"down"}, tuipage.RuntimeDownUser, false),
		systemAction("runtime.down.system", "Stop system service", "Stop and remove the machine-level managed runtime while preserving config and logs", []string{"runtime", "service", "down", "system", "stop"}, []string{"down", "--system"}, tuipage.RuntimeDownSystem, true),
		systemAction("runtime.restart.user", "Restart user service", "Restart the per-user managed runtime", []string{"runtime", "service", "restart", "user"}, []string{"restart"}, tuipage.RuntimeRestartUser, false),
		systemAction("runtime.restart.system", "Restart system service", "Restart the machine-level managed runtime", []string{"runtime", "service", "restart", "system"}, []string{"restart", "--system"}, tuipage.RuntimeRestartSystem, true),
		systemAction("runtime.reload", "Reload runtime config", "Reload persisted configuration into the running runtime", []string{"runtime", "config", "reload"}, []string{"config", "reload"}, tuipage.RuntimeReload, false),
		systemAction("runtime.foreground", "Run foreground runtime", "Show the foreground serve command to run after leaving the TUI", []string{"runtime", "foreground", "serve", "terminal"}, []string{"serve"}, tuipage.RuntimeForeground, false),
		systemAction("transport.mcp-http.enable", "Enable MCP HTTP server", "Enable the local MCP HTTP transport and reload the running runtime", []string{"mcp", "http", "server", "transport", "enable", "listener"}, []string{"config", "set"}, tuipage.MCPHTTPEnable, false),
		systemAction("transport.mcp-http.disable", "Disable MCP HTTP server", "Disable the local MCP HTTP transport and close its listener; the Secure MCP Tunnel must remain enabled", []string{"mcp", "http", "server", "transport", "disable", "listener", "port"}, []string{"config", "set"}, tuipage.MCPHTTPDisable, false),
		systemAction("config.initialize.external", "Initialize configuration", "Show the initialization command that creates configuration and one-time authentication tokens", []string{"config", "init", "initialize", "token"}, []string{"init"}, tuipage.ConfigInitialize, false),
		systemAction("config.uninitialize.external", "Uninitialize configuration", "Show the destructive command that removes local configuration and state", []string{"config", "uninit", "uninitialize", "remove", "state"}, []string{"uninit"}, tuipage.ConfigUninitialize, false),
		systemAction("auth.mcp.enable", "Enable MCP authentication", "Enable MCP token authentication", []string{"auth", "mcp", "enable"}, []string{"auth", "mcp", "enable"}, tuipage.AuthMCPEnable, false),
		systemAction("auth.mcp.disable", "Disable MCP authentication", "Disable MCP token authentication", []string{"auth", "mcp", "disable"}, []string{"auth", "mcp", "disable"}, tuipage.AuthMCPDisable, false),
		systemAction("auth.mcp.rotate", "Rotate MCP token", "Rotate the MCP token and reveal the replacement once", []string{"auth", "mcp", "token", "rotate", "create"}, []string{"auth", "mcp", "create"}, tuipage.AuthMCPRotate, false),
		systemAction("auth.admin.enable", "Enable admin authentication", "Enable admin token authentication", []string{"auth", "admin", "enable"}, []string{"auth", "admin", "enable"}, tuipage.AuthAdminEnable, false),
		systemAction("auth.admin.disable", "Disable admin authentication", "Disable admin token authentication", []string{"auth", "admin", "disable"}, []string{"auth", "admin", "disable"}, tuipage.AuthAdminDisable, false),
		systemAction("auth.admin.rotate", "Rotate admin token", "Rotate the admin token and reveal the replacement once", []string{"auth", "admin", "token", "rotate", "create"}, []string{"auth", "admin", "create"}, tuipage.AuthAdminRotate, false),
		systemAction("alias.install", "Install cgm alias", "Install the cgm alias for the managed direct installation", []string{"alias", "cgm", "install"}, []string{"alias", "install"}, tuipage.AliasInstall, false),
		systemAction("alias.remove", "Remove cgm alias", "Remove the cgm alias without removing the managed installation", []string{"alias", "cgm", "remove"}, []string{"alias", "remove"}, tuipage.AliasRemove, false),
		systemAction("install.run", "Install managed binary", "Install this binary into the versioned managed layout", []string{"install", "managed", "binary"}, []string{"install"}, tuipage.InstallRun, false),
		systemAction("install.cleanup", "Clean legacy installations", "Remove verified legacy standalone installations from PATH", []string{"install", "cleanup", "migrate", "legacy"}, []string{"install", "cleanup"}, tuipage.InstallCleanup, false),
		systemAction("update.check", "Check for updates", "Check the latest available verified release", []string{"update", "check", "latest", "release"}, []string{"update", "check"}, tuipage.UpdateCheck, false),
		systemAction("update.apply", "Apply update", "Download, verify, install, and activate an update", []string{"update", "apply", "install", "release"}, []string{"update"}, tuipage.UpdateApply, false),
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
		logsAction("logs.filter", "Filter logs", "Configure structured runtime log filters", []string{"logs", "filter", "grep", "session", "level"}, []string{"logs"}, tuipage.LogsFilter),
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
		configAction("config.reload", "Reload runtime config", "Reload persisted configuration into the running runtime", []string{"config", "reload", "runtime"}, []string{"config", "reload"}, tuipage.ConfigReload),
		configAction("config.migrate", "Migrate config secrets", "Migrate legacy plaintext credentials into the secret store", []string{"config", "migrate", "secrets"}, []string{"config", "migrate"}, tuipage.ConfigMigrate),
		configAction("config.convert", "Convert config format", "Convert structured config/state files between JSON, YAML, and TOML", []string{"config", "convert", "transform", "format"}, []string{"config", "convert"}, tuipage.ConfigConvert),
		configAction("config.export", "Export config bundle", "Export portable configuration, state, and secrets into a sealed bundle", []string{"config", "export", "bundle", "backup"}, []string{"config", "export"}, tuipage.ConfigExport),
		configAction("config.import", "Import config bundle", "Import a portable configuration bundle and restore its secrets", []string{"config", "import", "bundle", "restore"}, []string{"config", "import"}, tuipage.ConfigImport),
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
		requestAction("request.show.pending", "Show pending requests", "Show only pending approval requests", []string{"request", "pending", "filter"}, []string{"request", "list"}, tuipage.RequestShowPending, false),
		requestAction("request.show.history", "Show request history", "Show resolved and expired approval requests", []string{"request", "history", "resolved", "filter"}, []string{"request", "list"}, tuipage.RequestShowHistory, false),
		requestAction("request.show.all", "Show all requests", "Show pending and historical approval requests", []string{"request", "all", "filter"}, []string{"request", "list"}, tuipage.RequestShowAll, false),
		requestAction("request.approve", "Approve request", "Approve the selected pending control request", []string{"request", "approve", "allow", "accept"}, []string{"request", "approve"}, tuipage.RequestApprove, false),
		requestAction("request.deny", "Deny request", "Deny the selected pending control request", []string{"request", "deny", "reject"}, []string{"request", "deny"}, tuipage.RequestDeny, false),
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
		tunnelAction("tunnel.configure", "Configure runtime tunnel", "Configure the local OpenAI Secure MCP Tunnel", []string{"tunnel", "configure", "runtime"}, []string{"tunnel", "configure"}, tuipage.TunnelConfigure, RouteTunnel, false),
		tunnelAction("tunnel.enable", "Enable runtime tunnel", "Enable the local OpenAI Secure MCP Tunnel", []string{"tunnel", "enable", "runtime"}, []string{"tunnel", "enable"}, tuipage.TunnelEnable, RouteTunnel, false),
		tunnelAction("tunnel.disable", "Disable runtime tunnel", "Disable the local OpenAI Secure MCP Tunnel", []string{"tunnel", "disable", "runtime"}, []string{"tunnel", "disable"}, tuipage.TunnelDisable, RouteTunnel, false),
		tunnelAction("tunnel.foreground", "Run foreground tunnel", "Show the foreground tunnel command to run after leaving the TUI", []string{"tunnel", "foreground", "run", "terminal"}, []string{"tunnel", "run"}, tuipage.TunnelForeground, RouteTunnel, false),
		tunnelAction("tunnel.sync", "Sync tunnel metadata", "Fetch and persist metadata for the configured runtime tunnel", []string{"tunnel", "sync", "metadata"}, []string{"tunnel", "sync"}, tuipage.TunnelSync, RouteTunnel, false),
		tunnelAction("tunnel.admin.key.set", "Set admin key", "Verify and store an OpenAI tunnel admin key", []string{"tunnel", "admin", "key", "set"}, []string{"tunnel", "admin", "key", "set"}, tuipage.TunnelAdminKeySet, RouteTunnel, false),
		tunnelAction("tunnel.admin.key.verify", "Verify admin key", "Re-verify Tunnels Manage access for the stored admin key", []string{"tunnel", "admin", "key", "verify"}, []string{"tunnel", "admin", "key", "verify"}, tuipage.TunnelAdminKeyVerify, RouteTunnel, false),
		tunnelAction("tunnel.admin.key.remove", "Remove admin key", "Remove the stored tunnel admin key and verification scope", []string{"tunnel", "admin", "key", "remove"}, []string{"tunnel", "admin", "key", "remove"}, tuipage.TunnelAdminKeyRemove, RouteTunnel, false),
		tunnelAction("tunnel.managed.refresh", "Refresh managed tunnels", "Refresh managed tunnels from the OpenAI control plane", []string{"tunnel", "managed", "refresh", "list"}, []string{"tunnel", "list"}, tuipage.TunnelManagedRefresh, RouteTunnels, false),
		tunnelAction("tunnel.managed.create", "Create managed tunnel", "Create a tunnel through the OpenAI Tunnel Management API", []string{"tunnel", "managed", "create"}, []string{"tunnel", "create"}, tuipage.TunnelManagedCreate, RouteTunnels, false),
		tunnelAction("tunnel.managed.update", "Update managed tunnel", "Update the current managed tunnel", []string{"tunnel", "managed", "update", "edit"}, []string{"tunnel", "update"}, tuipage.TunnelManagedUpdate, RouteTunnels, true),
		tunnelAction("tunnel.managed.configure", "Use managed tunnel", "Configure cgm to use the current managed tunnel", []string{"tunnel", "managed", "configure", "runtime"}, []string{"tunnel", "get", "--configure"}, tuipage.TunnelManagedConfigure, RouteTunnels, true),
		tunnelAction("tunnel.managed.delete", "Delete managed tunnel", "Permanently delete the current managed tunnel", []string{"tunnel", "managed", "delete", "remove"}, []string{"tunnel", "delete"}, tuipage.TunnelManagedDelete, RouteTunnels, true),
	}
}

func tunnelAction(id, title, description string, keywords, commandPath []string, command tuipage.TunnelCommand, route RouteKind, needsResource bool) action.Action {
	return action.Action{
		ID: id, Title: title, Category: "Tunnel", Description: description, Keywords: keywords, CommandPath: commandPath, Capabilities: capabilitiesForCommandPath(commandPath), Scope: action.ScopeGlobal,
		Available: func(ctx action.Context) bool {
			if ctx.Route != string(route) {
				return false
			}
			return !needsResource || ctx.ResourceID != ""
		},
		Run: func(_ context.Context, ctx action.Context) tea.Cmd {
			return func() tea.Msg { return tuipage.TunnelCommandMsg{Command: command, ResourceID: ctx.ResourceID} }
		},
	}
}

func mcpActions() []action.Action {
	return []action.Action{
		mcpAction("mcp.server.add", "Add server", "Add an upstream MCP server", []string{"mcp", "server", "add", "upstream"}, []string{"mcp", "server", "add"}, tuipage.MCPServerAdd, false),
		mcpAction("mcp.server.configure", "Configure server", "Configure the current upstream MCP server", []string{"mcp", "server", "configure", "set"}, []string{"mcp", "server", "configure"}, tuipage.MCPServerConfigure, true),
		mcpAction("mcp.server.remove", "Remove server", "Remove the current upstream MCP server", []string{"mcp", "server", "remove", "delete"}, []string{"mcp", "server", "remove"}, tuipage.MCPServerRemove, true),
		mcpAction("mcp.server.enable", "Enable server", "Enable the current upstream MCP server", []string{"mcp", "server", "enable"}, []string{"mcp", "server", "enable"}, tuipage.MCPServerEnable, true),
		mcpAction("mcp.server.disable", "Disable server", "Disable the current upstream MCP server", []string{"mcp", "server", "disable"}, []string{"mcp", "server", "disable"}, tuipage.MCPServerDisable, true),
		mcpAction("mcp.server.status", "Refresh health", "Refresh upstream MCP health and connection status", []string{"mcp", "server", "status", "health", "refresh"}, []string{"mcp", "server", "status"}, tuipage.MCPServerHealth, false),
		mcpAction("mcp.server.tools", "View tools", "Load tools exposed by the current upstream MCP server", []string{"mcp", "server", "tools", "refresh"}, []string{"mcp", "server", "tools"}, tuipage.MCPServerTools, true),
		mcpAction("mcp.server.auth.login", "OAuth login", "Authorize the current HTTP MCP server with OAuth", []string{"mcp", "server", "auth", "login", "oauth"}, []string{"mcp", "server", "auth", "login"}, tuipage.MCPAuthLogin, true),
		mcpAction("mcp.server.auth.logout", "OAuth logout", "Remove stored OAuth authorization for the current MCP server", []string{"mcp", "server", "auth", "logout", "oauth"}, []string{"mcp", "server", "auth", "logout"}, tuipage.MCPAuthLogout, true),
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
		workspaceAction("workspace.register", "Register", "Register a workspace root", []string{"workspace", "register"}, []string{"workspace", "register"}, tuipage.WorkspaceRegister, false, false),
		workspaceAction("workspace.unregister", "Unregister", "Unregister the current workspace without deleting project files", []string{"workspace", "unregister"}, []string{"workspace", "unregister"}, tuipage.WorkspaceUnregister, true, false),
		workspaceAction("workspace.access.add", "Add access directory", "Grant the current workspace access to an additional directory", []string{"workspace", "access", "add"}, []string{"workspace", "access", "add"}, tuipage.WorkspaceAccessAdd, true, false),
		workspaceAction("workspace.access.remove", "Remove access directory", "Revoke an additional directory from the current workspace", []string{"workspace", "access", "remove"}, []string{"workspace", "access", "remove"}, tuipage.WorkspaceAccessRemove, true, false),
		workspaceAction("workspace.container.create", "Create container", "Create a workspace container", []string{"workspace", "container", "create"}, []string{"workspace", "container", "create"}, tuipage.WorkspaceContainerCreate, false, true),
		workspaceAction("workspace.container.rename", "Rename container", "Rename the current workspace container", []string{"workspace", "container", "rename"}, []string{"workspace", "container", "rename"}, tuipage.WorkspaceContainerRename, true, true),
		workspaceAction("workspace.container.delete", "Delete container", "Delete the current container without unregistering workspaces", []string{"workspace", "container", "delete"}, []string{"workspace", "container", "delete"}, tuipage.WorkspaceContainerDelete, true, true),
		workspaceAction("workspace.container.add", "Add container members", "Edit workspace membership for the current container", []string{"workspace", "container", "add", "members"}, []string{"workspace", "container", "add"}, tuipage.WorkspaceContainerMembers, true, true),
		workspaceAction("workspace.container.remove", "Remove container members", "Edit workspace membership for the current container", []string{"workspace", "container", "remove", "members"}, []string{"workspace", "container", "remove"}, tuipage.WorkspaceContainerMembers, true, true),
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

func capabilitiesForCommandPath(commandPath []string) []capability.ID {
	id, ok := capability.ForPath(strings.Join(commandPath, " "))
	if !ok {
		return nil
	}
	return []capability.ID{id}
}

func actionContext(route Route) action.Context {
	return action.Context{Route: string(route.Kind), ResourceID: route.ResourceID}
}
