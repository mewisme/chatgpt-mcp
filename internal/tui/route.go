package tui

import (
	"fmt"
	"strings"
)

type RouteKind string

const (
	RouteHome        RouteKind = "home"
	RouteWorkspaces  RouteKind = "workspaces"
	RouteContainers  RouteKind = "containers"
	RouteMCP         RouteKind = "mcp"
	RouteTunnel      RouteKind = "tunnel"
	RouteTunnels     RouteKind = "tunnels"
	RouteRequests    RouteKind = "requests"
	RouteLogs        RouteKind = "logs"
	RouteLogsExec    RouteKind = "logs-exec"
	RouteConfig      RouteKind = "config"
	RouteInstruction RouteKind = "instruction"
	RouteRuntime     RouteKind = "runtime"
	RouteAbout       RouteKind = "about"
)

type Route struct {
	Kind       RouteKind
	Mode       string
	ResourceID string
	Section    string
}

type headerPage struct {
	Kind         RouteKind
	Label        string
	CompactLabel string
}

var headerPages = []headerPage{
	{Kind: RouteWorkspaces, Label: "Workspaces", CompactLabel: "Work"},
	{Kind: RouteMCP, Label: "MCP"},
	{Kind: RouteTunnel, Label: "Tunnel", CompactLabel: "Tun"},
	{Kind: RouteRequests, Label: "Requests", CompactLabel: "Req"},
	{Kind: RouteLogs, Label: "Logs"},
	{Kind: RouteConfig, Label: "Config", CompactLabel: "Cfg"},
	{Kind: RouteInstruction, Label: "Instruction", CompactLabel: "Instr"},
	{Kind: RouteRuntime, Label: "Runtime", CompactLabel: "Run"},
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
	if kind == RouteRequests {
		return parseRequestsRoute(parts)
	}
	if len(parts) > 3 {
		return Route{}, fmt.Errorf("TUI path accepts at most one resource id and one child section: %s", strings.Join(parts, " "))
	}
	resourceID := ""
	if len(parts) >= 2 {
		if !routeAcceptsResource(kind) {
			return Route{}, fmt.Errorf("TUI path %q does not accept a resource id", parts[0])
		}
		resourceID = parts[1]
	}
	section := ""
	if len(parts) == 3 {
		if resourceID == "" {
			return Route{}, fmt.Errorf("TUI path %q requires a resource id before a child section", parts[0])
		}
		var ok bool
		section, ok = normalizeRouteSection(kind, parts[2])
		if !ok {
			return Route{}, fmt.Errorf("unsupported %s child section %q", kind, parts[2])
		}
	}
	return Route{Kind: kind, ResourceID: resourceID, Section: section}, nil
}

func parseRequestsRoute(parts []string) (Route, error) {
	route := Route{Kind: RouteRequests}
	if len(parts) == 1 {
		return route, nil
	}
	if len(parts) > 4 {
		return Route{}, fmt.Errorf("requests path accepts an optional mode, resource id, and child section: %s", strings.Join(parts, " "))
	}
	index := 1
	if mode, ok := normalizeRequestRouteMode(parts[index]); ok {
		route.Mode = mode
		index++
		if index == len(parts) {
			return route, nil
		}
	}
	route.ResourceID = parts[index]
	index++
	if route.Mode == "" {
		route.Mode = "all"
	}
	if index < len(parts) {
		section, ok := normalizeRouteSection(RouteRequests, parts[index])
		if !ok {
			return Route{}, fmt.Errorf("unsupported requests child section %q", parts[index])
		}
		route.Section = section
		index++
	}
	if index != len(parts) {
		return Route{}, fmt.Errorf("unsupported requests path %q", strings.Join(parts, " "))
	}
	return route, nil
}

func normalizeRequestRouteMode(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "pending":
		return "pending", true
	case "history":
		return "history", true
	case "all":
		return "all", true
	default:
		return "", false
	}
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
	case "logs-exec", "exec-logs", "command-execution", "command-execution-logs":
		return RouteLogsExec, true
	case "config", "cfg":
		return RouteConfig, true
	case "instruction", "instructions", "instr":
		return RouteInstruction, true
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
		RouteRequests: "Requests", RouteLogs: "Logs", RouteLogsExec: "Logs · Command Execution", RouteConfig: "Config", RouteInstruction: "Instruction", RouteRuntime: "Runtime", RouteAbout: "About",
	}[route.Kind]
	if route.ResourceID != "" {
		base += " · " + route.ResourceID
	} else if route.Kind == RouteRequests && route.Mode != "" {
		base += " · " + routeSectionTitle(route.Mode)
	}
	if route.Section != "" {
		base += " · " + routeSectionTitle(route.Section)
	}
	return base
}

func routeAcceptsResource(kind RouteKind) bool {
	switch kind {
	case RouteWorkspaces, RouteContainers, RouteMCP, RouteTunnels, RouteRequests, RouteLogs, RouteConfig, RouteRuntime:
		return true
	default:
		return false
	}
}

func normalizeRouteSection(kind RouteKind, value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "overview" {
		return "", true
	}
	allowed := map[RouteKind]map[string]bool{
		RouteWorkspaces: {"access": true, "containers": true, "context": true, "context-preview": true},
		RouteContainers: {"workspaces": true},
		RouteMCP:        {"health": true, "tools": true, "oauth": true},
		RouteTunnels:    {"scope": true},
		RouteRequests:   {"arguments": true, "guard": true},
		RouteLogs:       {"fields": true},
	}
	return value, allowed[kind][value]
}

func routeSectionTitle(section string) string {
	if section == "" {
		return "Overview"
	}
	words := strings.Fields(strings.NewReplacer("-", " ", "_", " ", ".", " ").Replace(section))
	for index := range words {
		if words[index] != "" {
			words[index] = strings.ToUpper(words[index][:1]) + words[index][1:]
		}
	}
	return strings.Join(words, " ")
}

type Router struct {
	stack []Route
}

func NewRouter(initial Route) Router { return Router{stack: routeStack(initial)} }

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
	router.stack = routeStack(route)
}

func (router *Router) Switch(route Route) {
	if router == nil {
		return
	}
	router.stack = routeStack(route)
}

func (router *Router) Back() bool {
	if router == nil || len(router.stack) < 2 {
		return false
	}
	router.stack = router.stack[:len(router.stack)-1]
	return true
}

func routeStack(route Route) []Route {
	if route.Kind == RouteHome {
		return []Route{{Kind: RouteHome}}
	}
	main := Route{Kind: route.Kind}
	if route.Kind == RouteRequests {
		main.Mode = route.Mode
	}
	stack := []Route{main}
	if route.ResourceID == "" {
		return stack
	}
	resource := main
	resource.ResourceID = route.ResourceID
	stack = append(stack, resource)
	if route.Section == "" {
		return stack
	}
	resource.Section = route.Section
	return append(stack, resource)
}

func headerOwner(kind RouteKind) RouteKind {
	switch kind {
	case RouteContainers:
		return RouteWorkspaces
	case RouteTunnels:
		return RouteTunnel
	case RouteLogsExec:
		return RouteLogs
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
