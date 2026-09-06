package tui

import (
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/tui/action"
)

func TestWorkspaceActionAvailabilityFollowsRouteContext(t *testing.T) {
	registry := defaultActionRegistry()
	has := func(ctx action.Context, id string) bool {
		for _, item := range registry.Actions(ctx) {
			if item.ID == id {
				return true
			}
		}
		return false
	}
	if !has(action.Context{Route: string(RouteHome)}, "workspace.register") || !has(action.Context{Route: string(RouteHome)}, "workspace.container.create") {
		t.Fatal("global workspace actions are unavailable")
	}
	if has(action.Context{Route: string(RouteWorkspaces)}, "workspace.unregister") {
		t.Fatal("workspace unregister available without a resource")
	}
	if !has(action.Context{Route: string(RouteWorkspaces), ResourceID: "ws_demo"}, "workspace.unregister") || !has(action.Context{Route: string(RouteWorkspaces), ResourceID: "ws_demo"}, "workspace.access.add") {
		t.Fatal("workspace context actions missing")
	}
	if !has(action.Context{Route: string(RouteContainers), ResourceID: "wsc_demo"}, "workspace.container.rename") || !has(action.Context{Route: string(RouteContainers), ResourceID: "wsc_demo"}, "workspace.container.remove") {
		t.Fatal("container context actions missing")
	}
}
