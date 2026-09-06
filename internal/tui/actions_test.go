package tui

import (
	"strings"
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

func TestMCPActionAvailabilityFollowsRouteContext(t *testing.T) {
	registry := defaultActionRegistry()
	has := func(ctx action.Context, id string) bool {
		for _, item := range registry.Actions(ctx) {
			if item.ID == id {
				return true
			}
		}
		return false
	}
	if !has(action.Context{Route: string(RouteHome)}, "mcp.server.add") || !has(action.Context{Route: string(RouteHome)}, "mcp.server.status") {
		t.Fatal("global MCP actions are unavailable")
	}
	if has(action.Context{Route: string(RouteMCP)}, "mcp.server.configure") || has(action.Context{Route: string(RouteMCP)}, "mcp.server.tools") {
		t.Fatal("resource MCP actions available without a resource")
	}
	ctx := action.Context{Route: string(RouteMCP), ResourceID: "github"}
	for _, id := range []string{"mcp.server.configure", "mcp.server.remove", "mcp.server.enable", "mcp.server.disable", "mcp.server.tools", "mcp.server.auth.login", "mcp.server.auth.logout"} {
		if !has(ctx, id) {
			t.Fatalf("MCP context action missing: %s", id)
		}
	}
}

func TestTunnelActionAvailabilityFollowsRouteContext(t *testing.T) {
	registry := defaultActionRegistry()
	has := func(ctx action.Context, id string) bool {
		for _, item := range registry.Actions(ctx) {
			if item.ID == id {
				return true
			}
		}
		return false
	}
	if !has(action.Context{Route: string(RouteTunnel)}, "tunnel.configure") || !has(action.Context{Route: string(RouteTunnel)}, "tunnel.admin.key.set") {
		t.Fatal("runtime tunnel actions are unavailable on tunnel route")
	}
	if has(action.Context{Route: string(RouteHome)}, "tunnel.configure") || has(action.Context{Route: string(RouteTunnel)}, "tunnel.managed.create") {
		t.Fatal("tunnel actions leaked into the wrong route")
	}
	if !has(action.Context{Route: string(RouteTunnels)}, "tunnel.managed.create") || !has(action.Context{Route: string(RouteTunnels)}, "tunnel.managed.refresh") {
		t.Fatal("managed tunnel list actions are unavailable")
	}
	if has(action.Context{Route: string(RouteTunnels)}, "tunnel.managed.update") || has(action.Context{Route: string(RouteTunnels)}, "tunnel.managed.delete") {
		t.Fatal("managed tunnel resource actions available without a resource")
	}
	ctx := action.Context{Route: string(RouteTunnels), ResourceID: "tunnel_demo"}
	for _, id := range []string{"tunnel.managed.update", "tunnel.managed.configure", "tunnel.managed.delete"} {
		if !has(ctx, id) {
			t.Fatalf("managed tunnel context action missing: %s", id)
		}
	}
}

func TestRequestActionAvailabilityFollowsRouteContext(t *testing.T) {
	registry := defaultActionRegistry()
	has := func(ctx action.Context, id string) bool {
		for _, item := range registry.Actions(ctx) {
			if item.ID == id {
				return true
			}
		}
		return false
	}
	list := action.Context{Route: string(RouteRequests)}
	for _, id := range []string{"request.refresh", "request.show.pending", "request.show.history", "request.show.all"} {
		if !has(list, id) {
			t.Fatalf("request list action missing: %s", id)
		}
	}
	if !has(list, "request.approve") || !has(list, "request.deny") {
		t.Fatal("request resolution actions unavailable on requests list")
	}
	resource := action.Context{Route: string(RouteRequests), ResourceID: "req_demo"}
	if !has(resource, "request.approve") || !has(resource, "request.deny") {
		t.Fatal("request resource resolution actions unavailable")
	}
	if has(action.Context{Route: string(RouteHome)}, "request.refresh") {
		t.Fatal("request actions leaked outside requests route")
	}
	for _, item := range registry.Actions(list) {
		if item.ID == "request.create.dummy" || strings.Contains(strings.Join(item.CommandPath, " "), "create dummy") {
			t.Fatalf("dummy request action leaked into TUI palette: %#v", item)
		}
	}
}

func TestConfigActionAvailabilityFollowsRouteContext(t *testing.T) {
	registry := defaultActionRegistry()
	has := func(ctx action.Context, id string) bool {
		for _, item := range registry.Actions(ctx) {
			if item.ID == id {
				return true
			}
		}
		return false
	}
	ctx := action.Context{Route: string(RouteConfig)}
	for _, id := range []string{"config.refresh", "config.edit", "config.verify", "config.reload", "config.migrate", "config.convert", "config.export", "config.import"} {
		if !has(ctx, id) {
			t.Fatalf("config action missing: %s", id)
		}
	}
	if has(action.Context{Route: string(RouteHome)}, "config.verify") {
		t.Fatal("config actions leaked outside config route")
	}
}
