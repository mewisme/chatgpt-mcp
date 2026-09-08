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
	Action     string
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
	switch kind {
	case RouteWorkspaces:
		return parseWorkspaceRoute(parts)
	case RouteContainers:
		return parseContainerRoute(parts)
	case RouteMCP:
		return parseMCPRoute(parts)
	case RouteTunnel:
		return parseTunnelRoute(parts)
	case RouteTunnels:
		return parseManagedTunnelRoute(parts)
	case RouteRequests:
		return parseRequestsRoute(parts)
	case RouteLogs:
		return parseLogsRoute(parts)
	case RouteConfig:
		return parseConfigRoute(parts)
	case RouteInstruction:
		return parseInstructionRoute(parts)
	case RouteRuntime:
		return parseRuntimeRoute(parts)
	default:
		if len(parts) != 1 {
			return Route{}, fmt.Errorf("TUI path %q does not accept child segments", parts[0])
		}
		return Route{Kind: kind}, nil
	}
}

func parseWorkspaceRoute(parts []string) (Route, error) {
	route := Route{Kind: RouteWorkspaces}
	if len(parts) == 1 {
		return route, nil
	}
	if len(parts) == 2 && parts[1] == "register" {
		route.Action = "register"
		return route, nil
	}
	if len(parts) > 4 {
		return Route{}, fmt.Errorf("workspace path is too deep: %s", strings.Join(parts, " "))
	}
	route.ResourceID = parts[1]
	if len(parts) == 2 {
		return route, nil
	}
	section, ok := normalizeRouteSection(RouteWorkspaces, parts[2])
	if !ok {
		return Route{}, fmt.Errorf("unsupported workspaces child section %q", parts[2])
	}
	route.Section = section
	if len(parts) == 3 {
		return route, nil
	}
	if route.Section != "access" || parts[3] != "add" && parts[3] != "remove" {
		return Route{}, fmt.Errorf("unsupported workspace editor action %q", parts[3])
	}
	route.Action = parts[3]
	return route, nil
}

func parseContainerRoute(parts []string) (Route, error) {
	route := Route{Kind: RouteContainers}
	if len(parts) == 1 {
		return route, nil
	}
	if len(parts) == 2 && parts[1] == "create" {
		route.Action = "create"
		return route, nil
	}
	if len(parts) > 4 {
		return Route{}, fmt.Errorf("container path is too deep: %s", strings.Join(parts, " "))
	}
	route.ResourceID = parts[1]
	if len(parts) == 2 {
		return route, nil
	}
	if len(parts) == 3 && parts[2] == "edit" {
		route.Action = "edit"
		return route, nil
	}
	section, ok := normalizeRouteSection(RouteContainers, parts[2])
	if !ok {
		return Route{}, fmt.Errorf("unsupported containers child section %q", parts[2])
	}
	route.Section = section
	if len(parts) == 3 {
		return route, nil
	}
	if route.Section != "workspaces" || parts[3] != "edit" {
		return Route{}, fmt.Errorf("unsupported container editor action %q", parts[3])
	}
	route.Action = "edit"
	return route, nil
}

func parseMCPRoute(parts []string) (Route, error) {
	route := Route{Kind: RouteMCP}
	if len(parts) == 1 {
		return route, nil
	}
	if len(parts) == 2 && parts[1] == "create" {
		route.Action = "create"
		return route, nil
	}
	if len(parts) > 4 {
		return Route{}, fmt.Errorf("mcp path is too deep: %s", strings.Join(parts, " "))
	}
	route.ResourceID = parts[1]
	if len(parts) == 2 {
		return route, nil
	}
	if len(parts) == 3 && parts[2] == "edit" {
		route.Action = "edit"
		return route, nil
	}
	section, ok := normalizeRouteSection(RouteMCP, parts[2])
	if !ok {
		return Route{}, fmt.Errorf("unsupported mcp child section %q", parts[2])
	}
	route.Section = section
	if len(parts) == 3 {
		return route, nil
	}
	if route.Section != "oauth" || parts[3] != "login" {
		return Route{}, fmt.Errorf("unsupported mcp editor action %q", parts[3])
	}
	route.Action = "login"
	return route, nil
}

func parseTunnelRoute(parts []string) (Route, error) {
	route := Route{Kind: RouteTunnel}
	switch {
	case len(parts) == 1:
		return route, nil
	case len(parts) == 2 && parts[1] == "edit":
		route.Action = "edit"
		return route, nil
	case len(parts) == 3 && parts[1] == "admin-key" && parts[2] == "edit":
		route.Section, route.Action = "admin-key", "edit"
		return route, nil
	default:
		return Route{}, fmt.Errorf("unsupported tunnel path %q", strings.Join(parts, " "))
	}
}

func parseManagedTunnelRoute(parts []string) (Route, error) {
	route := Route{Kind: RouteTunnels}
	if len(parts) == 1 {
		return route, nil
	}
	if len(parts) == 2 && parts[1] == "create" {
		route.Action = "create"
		return route, nil
	}
	if len(parts) > 3 {
		return Route{}, fmt.Errorf("managed tunnel path is too deep: %s", strings.Join(parts, " "))
	}
	route.ResourceID = parts[1]
	if len(parts) == 2 {
		return route, nil
	}
	if parts[2] == "edit" || parts[2] == "configure" {
		route.Action = parts[2]
		return route, nil
	}
	section, ok := normalizeRouteSection(RouteTunnels, parts[2])
	if !ok {
		return Route{}, fmt.Errorf("unsupported tunnels child section %q", parts[2])
	}
	route.Section = section
	return route, nil
}

func parseLogsRoute(parts []string) (Route, error) {
	route := Route{Kind: RouteLogs}
	if len(parts) == 1 {
		return route, nil
	}
	if len(parts) == 2 && parts[1] == "filter" {
		route.Action = "filter"
		return route, nil
	}
	if len(parts) > 3 {
		return Route{}, fmt.Errorf("logs path is too deep: %s", strings.Join(parts, " "))
	}
	route.ResourceID = parts[1]
	if len(parts) == 2 {
		return route, nil
	}
	section, ok := normalizeRouteSection(RouteLogs, parts[2])
	if !ok {
		return Route{}, fmt.Errorf("unsupported logs child section %q", parts[2])
	}
	route.Section = section
	return route, nil
}

func parseConfigRoute(parts []string) (Route, error) {
	route := Route{Kind: RouteConfig}
	if len(parts) == 1 {
		return route, nil
	}
	if len(parts) > 3 {
		return Route{}, fmt.Errorf("config path is too deep: %s", strings.Join(parts, " "))
	}
	if len(parts) == 3 && parts[1] == "storage" {
		switch parts[2] {
		case "convert", "export", "import":
			route.Section, route.Action = "storage", parts[2]
			return route, nil
		default:
			return Route{}, fmt.Errorf("unsupported config storage action %q", parts[2])
		}
	}
	route.ResourceID = parts[1]
	if len(parts) == 2 {
		return route, nil
	}
	if parts[2] != "edit" {
		return Route{}, fmt.Errorf("unsupported config editor action %q", parts[2])
	}
	route.Action = "edit"
	return route, nil
}

func parseRuntimeRoute(parts []string) (Route, error) {
	route := Route{Kind: RouteRuntime}
	if len(parts) == 1 {
		return route, nil
	}
	if len(parts) != 2 {
		return Route{}, fmt.Errorf("runtime path is too deep: %s", strings.Join(parts, " "))
	}
	if parts[1] == "install" || parts[1] == "update" {
		route.Action = parts[1]
		return route, nil
	}
	route.ResourceID = parts[1]
	return route, nil
}

func parseInstructionRoute(parts []string) (Route, error) {
	route := Route{Kind: RouteInstruction}
	if len(parts) == 1 {
		return route, nil
	}
	section, ok := normalizeRouteSection(RouteInstruction, parts[1])
	if !ok {
		return Route{}, fmt.Errorf("unsupported instruction tab %q", parts[1])
	}
	route.Section = section
	if len(parts) == 2 {
		return route, nil
	}
	if route.Section == "context" && len(parts) == 3 && parts[2] == "edit" {
		route.Action = "edit"
		return route, nil
	}
	if route.Section != "rules" {
		return Route{}, fmt.Errorf("instruction tab %q does not accept editor routes", route.Section)
	}
	if len(parts) == 3 && parts[2] == "create" {
		route.Action = "create"
		return route, nil
	}
	if len(parts) == 4 && parts[2] != "" && parts[3] == "edit" {
		route.ResourceID, route.Action = parts[2], "edit"
		return route, nil
	}
	return Route{}, fmt.Errorf("unsupported instruction editor path %q", strings.Join(parts, " "))
}

func parseRequestsRoute(parts []string) (Route, error) {
	route := Route{Kind: RouteRequests}
	if len(parts) == 1 {
		return route, nil
	}
	if len(parts) == 2 && parts[1] == "create-test" {
		route.Action = "create-test"
		return route, nil
	}
	if len(parts) > 4 {
		return Route{}, fmt.Errorf("requests path is too deep: %s", strings.Join(parts, " "))
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
		if route.Mode != "all" || normalizeExplicitRequestMode(parts[1]) {
			if parts[index] == "approve" || parts[index] == "deny" {
				route.Action = parts[index]
				index++
			}
		}
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

func normalizeExplicitRequestMode(value string) bool {
	_, ok := normalizeRequestRouteMode(value)
	return ok
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
	if route.Kind == RouteRequests && route.Mode != "" {
		base += " · " + routeSectionTitle(route.Mode)
	}
	if route.Kind == RouteInstruction {
		if route.Section != "" {
			base += " · " + routeSectionTitle(route.Section)
		}
		if route.ResourceID != "" {
			base += " · " + route.ResourceID
		}
	} else {
		if route.ResourceID != "" {
			base += " · " + route.ResourceID
		}
		if route.Section != "" {
			base += " · " + routeSectionTitle(route.Section)
		}
	}
	if route.Action != "" {
		base += " · " + routeSectionTitle(route.Action)
	}
	return base
}

func normalizeRouteSection(kind RouteKind, value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "overview" {
		return "", true
	}
	allowed := map[RouteKind]map[string]bool{
		RouteWorkspaces:  {"access": true, "containers": true, "context": true, "context-preview": true},
		RouteInstruction: {"context": true, "rules": true, "sources": true},
		RouteContainers:  {"workspaces": true},
		RouteMCP:         {"health": true, "tools": true, "oauth": true},
		RouteTunnels:     {"scope": true},
		RouteRequests:    {"arguments": true, "guard": true},
		RouteLogs:        {"fields": true},
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
	if route.Kind == RouteInstruction {
		if route.Action == "" {
			return []Route{route}
		}
		return []Route{{Kind: RouteInstruction, Section: route.Section}, route}
	}
	main := Route{Kind: route.Kind}
	if route.Kind == RouteRequests {
		main.Mode = route.Mode
	}
	stack := []Route{main}
	parent := main
	if route.ResourceID != "" {
		parent.ResourceID = route.ResourceID
		stack = append(stack, parent)
	}
	if route.Section != "" && routeSectionCreatesAncestry(route.Kind, route.Section) {
		parent.Section = route.Section
		stack = append(stack, parent)
	}
	if route.Action != "" {
		return append(stack, route)
	}
	if route.Section != "" && (len(stack) == 0 || stack[len(stack)-1] != route) {
		return append(stack, route)
	}
	return stack
}

func routeSectionCreatesAncestry(kind RouteKind, section string) bool {
	return !(kind == RouteTunnel && section == "admin-key" || kind == RouteConfig && section == "storage")
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
