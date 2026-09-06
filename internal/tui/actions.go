package tui

import (
	"context"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"go.mewis.me/chatgpt-mcp/internal/tui/action"
	tuipage "go.mewis.me/chatgpt-mcp/internal/tui/page"
)

type navigateMsg struct{ route Route }

func defaultActionRegistry() *action.Registry {
	actions := []action.Action{
		navigationAction("app.go.workspaces", "Workspaces", "1", Route{Kind: RouteWorkspaces}, []string{"workspace", "workspaces", "ws"}),
		navigationAction("app.go.containers", "Containers", "", Route{Kind: RouteContainers}, []string{"workspace", "container", "containers"}),
		navigationAction("app.go.mcp", "MCP Servers", "2", Route{Kind: RouteMCP}, []string{"mcp", "server", "upstream"}),
		navigationAction("app.go.tunnel", "Tunnel", "3", Route{Kind: RouteTunnel}, []string{"tunnel", "secure"}),
		navigationAction("app.go.requests", "Requests", "4", Route{Kind: RouteRequests}, []string{"request", "approval"}),
		navigationAction("app.go.logs", "Logs", "5", Route{Kind: RouteLogs}, []string{"logs", "events", "journal"}),
		navigationAction("app.go.config", "Config", "6", Route{Kind: RouteConfig}, []string{"config", "settings", "cfg"}),
		navigationAction("app.go.runtime", "Runtime", "7", Route{Kind: RouteRuntime}, []string{"runtime", "status", "service"}),
	}
	actions = append(actions, workspaceActions()...)
	actions = append(actions, mcpActions()...)
	registry, err := action.NewRegistry(actions...)
	if err != nil {
		panic(err)
	}
	return registry
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
		ID: id, Title: title, Category: "MCP", Description: description, Keywords: keywords, CommandPath: commandPath, Scope: action.ScopeGlobal,
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
		ID: id, Title: title, Category: "Workspace", Description: description, Keywords: keywords, CommandPath: commandPath, Scope: action.ScopeGlobal,
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

func navigationAction(id, title, shortcut string, route Route, keywords []string) action.Action {
	result := action.Action{
		ID: id, Title: "Go to " + title, Category: "App", Description: "Open the " + title + " page", Keywords: keywords,
		CommandPath: []string{"tui", string(route.Kind)}, Scope: action.ScopeGlobal,
		Run: func(context.Context, action.Context) tea.Cmd {
			return func() tea.Msg { return navigateMsg{route: route} }
		},
	}
	if shortcut != "" {
		result.Shortcut = key.NewBinding(key.WithKeys(shortcut), key.WithHelp(shortcut, title))
	}
	return result
}

func actionContext(route Route) action.Context {
	return action.Context{Route: string(route.Kind), ResourceID: route.ResourceID}
}
