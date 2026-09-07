package tui

import (
	"fmt"
	"strings"
)

type RouteKind string

const (
	RouteHome       RouteKind = "home"
	RouteWorkspaces RouteKind = "workspaces"
	RouteContainers RouteKind = "containers"
	RouteMCP        RouteKind = "mcp"
	RouteTunnel     RouteKind = "tunnel"
	RouteTunnels    RouteKind = "tunnels"
	RouteRequests   RouteKind = "requests"
	RouteLogs       RouteKind = "logs"
	RouteConfig     RouteKind = "config"
	RouteRuntime    RouteKind = "runtime"
	RouteAbout      RouteKind = "about"
)

type Route struct {
	Kind       RouteKind
	ResourceID string
}

type headerPage struct {
	Kind  RouteKind
	Label string
}

var headerPages = []headerPage{
	{Kind: RouteWorkspaces, Label: "Workspaces"},
	{Kind: RouteMCP, Label: "MCP"},
	{Kind: RouteTunnel, Label: "Tunnel"},
	{Kind: RouteRequests, Label: "Requests"},
	{Kind: RouteLogs, Label: "Logs"},
	{Kind: RouteConfig, Label: "Config"},
	{Kind: RouteRuntime, Label: "Runtime"},
}

func ParseRoute(args []string) (Route, error) {
	if len(args) == 0 {
		return Route{Kind: RouteHome}, nil
	}
	parts := make([]string, 0, len(args))
	for _, value := range args {
		if value = strings.TrimSpace(value); value != "" {
			parts = append(parts, value)
		}
	}
	if len(parts) == 0 {
		return Route{Kind: RouteHome}, nil
	}
	kind, ok := parseRouteKind(parts[0])
	if !ok {
		return Route{}, fmt.Errorf("unknown TUI path %q", strings.Join(parts, " "))
	}
	if len(parts) > 2 {
		return Route{}, fmt.Errorf("TUI path accepts at most one resource id: %s", strings.Join(parts, " "))
	}
	resourceID := ""
	if len(parts) == 2 {
		if kind != RouteWorkspaces && kind != RouteContainers && kind != RouteMCP && kind != RouteTunnels && kind != RouteRequests {
			return Route{}, fmt.Errorf("TUI path %q does not accept a resource id", parts[0])
		}
		resourceID = parts[1]
	}
	return Route{Kind: kind, ResourceID: resourceID}, nil
}

func parseRouteKind(value string) (RouteKind, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "home":
		return RouteHome, true
	case "workspace", "workspaces", "ws":
		return RouteWorkspaces, true
	case "container", "containers", "workspace-container", "workspace-containers":
		return RouteContainers, true
	case "mcp", "server", "servers":
		return RouteMCP, true
	case "tunnel":
		return RouteTunnel, true
	case "tunnels", "managed-tunnels":
		return RouteTunnels, true
	case "request", "requests", "req":
		return RouteRequests, true
	case "log", "logs":
		return RouteLogs, true
	case "config", "cfg":
		return RouteConfig, true
	case "runtime", "status":
		return RouteRuntime, true
	case "about", "version":
		return RouteAbout, true
	default:
		return "", false
	}
}

func (route Route) Title() string {
	base := map[RouteKind]string{
		RouteHome: "Home", RouteWorkspaces: "Workspaces", RouteContainers: "Workspaces · Containers", RouteMCP: "MCP Servers", RouteTunnel: "Tunnel", RouteTunnels: "Managed Tunnels",
		RouteRequests: "Requests", RouteLogs: "Logs", RouteConfig: "Config", RouteRuntime: "Runtime", RouteAbout: "About",
	}[route.Kind]
	if route.ResourceID != "" {
		return base + " · " + route.ResourceID
	}
	return base
}

type Router struct {
	stack []Route
}

func NewRouter(initial Route) Router { return Router{stack: []Route{initial}} }

func (router Router) Current() Route {
	if len(router.stack) == 0 {
		return Route{Kind: RouteHome}
	}
	return router.stack[len(router.stack)-1]
}

func (router *Router) Navigate(route Route) {
	if router == nil || route == router.Current() {
		return
	}
	router.stack = append(router.stack, route)
}

func (router *Router) Switch(route Route) {
	if router == nil {
		return
	}
	if len(router.stack) == 0 {
		router.stack = []Route{route}
		return
	}
	router.stack[len(router.stack)-1] = route
}

func (router *Router) Back() bool {
	if router == nil || len(router.stack) < 2 {
		return false
	}
	router.stack = router.stack[:len(router.stack)-1]
	return true
}

func headerOwner(kind RouteKind) RouteKind {
	switch kind {
	case RouteContainers:
		return RouteWorkspaces
	case RouteTunnels:
		return RouteTunnel
	default:
		return kind
	}
}

func cycleHeaderRoute(current Route, delta int) Route {
	owner := headerOwner(current.Kind)
	index := -1
	for candidate := range headerPages {
		if headerPages[candidate].Kind == owner {
			index = candidate
			break
		}
	}
	if index < 0 {
		if delta < 0 {
			index = 0
		} else {
			index = -1
		}
	}
	index = ((index+delta)%len(headerPages) + len(headerPages)) % len(headerPages)
	return Route{Kind: headerPages[index].Kind}
}
