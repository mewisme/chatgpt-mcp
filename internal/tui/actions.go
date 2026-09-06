package tui

import (
	"context"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"go.mewis.me/chatgpt-mcp/internal/tui/action"
)

type navigateMsg struct{ route Route }

func defaultActionRegistry() *action.Registry {
	registry, err := action.NewRegistry(
		navigationAction("app.go.workspaces", "Workspaces", "1", Route{Kind: RouteWorkspaces}, []string{"workspace", "workspaces", "ws"}),
		navigationAction("app.go.containers", "Containers", "", Route{Kind: RouteContainers}, []string{"workspace", "container", "containers"}),
		navigationAction("app.go.mcp", "MCP Servers", "2", Route{Kind: RouteMCP}, []string{"mcp", "server", "upstream"}),
		navigationAction("app.go.tunnel", "Tunnel", "3", Route{Kind: RouteTunnel}, []string{"tunnel", "secure"}),
		navigationAction("app.go.requests", "Requests", "4", Route{Kind: RouteRequests}, []string{"request", "approval"}),
		navigationAction("app.go.logs", "Logs", "5", Route{Kind: RouteLogs}, []string{"logs", "events", "journal"}),
		navigationAction("app.go.config", "Config", "6", Route{Kind: RouteConfig}, []string{"config", "settings", "cfg"}),
		navigationAction("app.go.runtime", "Runtime", "7", Route{Kind: RouteRuntime}, []string{"runtime", "status", "service"}),
	)
	if err != nil {
		panic(err)
	}
	return registry
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
