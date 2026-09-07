package tui

import "testing"

func TestParseRoute(t *testing.T) {
	tests := []struct {
		args []string
		want Route
	}{
		{nil, Route{Kind: RouteHome}},
		{[]string{"workspace"}, Route{Kind: RouteWorkspaces}},
		{[]string{"ws", "ws_abc"}, Route{Kind: RouteWorkspaces, ResourceID: "ws_abc"}},
		{[]string{"containers", "wsc_abc"}, Route{Kind: RouteContainers, ResourceID: "wsc_abc"}},
		{[]string{"mcp", "github"}, Route{Kind: RouteMCP, ResourceID: "github"}},
		{[]string{"tunnel"}, Route{Kind: RouteTunnel}},
		{[]string{"tunnels"}, Route{Kind: RouteTunnels}},
		{[]string{"tunnels", "tunnel_abc"}, Route{Kind: RouteTunnels, ResourceID: "tunnel_abc"}},
		{[]string{"logs"}, Route{Kind: RouteLogs}},
		{[]string{"logs-exec"}, Route{Kind: RouteLogsExec}},
		{[]string{"command-execution"}, Route{Kind: RouteLogsExec}},
		{[]string{"logs", "event_abc"}, Route{Kind: RouteLogs, ResourceID: "event_abc"}},
		{[]string{"logs", "event_abc", "fields"}, Route{Kind: RouteLogs, ResourceID: "event_abc", Section: "fields"}},
		{[]string{"ws", "ws_abc", "access"}, Route{Kind: RouteWorkspaces, ResourceID: "ws_abc", Section: "access"}},
		{[]string{"ws", "ws_abc", "overview"}, Route{Kind: RouteWorkspaces, ResourceID: "ws_abc"}},
		{[]string{"containers", "wsc_abc", "workspaces"}, Route{Kind: RouteContainers, ResourceID: "wsc_abc", Section: "workspaces"}},
		{[]string{"mcp", "github", "health"}, Route{Kind: RouteMCP, ResourceID: "github", Section: "health"}},
		{[]string{"tunnels", "tunnel_abc", "scope"}, Route{Kind: RouteTunnels, ResourceID: "tunnel_abc", Section: "scope"}},
		{[]string{"requests", "req_abc", "guard"}, Route{Kind: RouteRequests, ResourceID: "req_abc", Section: "guard"}},
		{[]string{"config", "runtime.port"}, Route{Kind: RouteConfig, ResourceID: "runtime.port"}},
		{[]string{"runtime", "service.user"}, Route{Kind: RouteRuntime, ResourceID: "service.user"}},
		{[]string{"cfg"}, Route{Kind: RouteConfig}},
		{[]string{"status"}, Route{Kind: RouteRuntime}},
		{[]string{"version"}, Route{Kind: RouteAbout}},
	}
	for _, test := range tests {
		got, err := ParseRoute(test.args)
		if err != nil || got != test.want {
			t.Fatalf("ParseRoute(%v) = %#v, %v; want %#v", test.args, got, err, test.want)
		}
	}
	for _, args := range [][]string{{"missing"}, {"tunnel", "extra"}, {"mcp", "a", "missing"}, {"config", "key", "extra"}, {"logs-exec", "extra"}, {"mcp", "a", "health", "extra"}} {
		if _, err := ParseRoute(args); err == nil {
			t.Fatalf("ParseRoute(%v) unexpectedly succeeded", args)
		}
	}
}

func TestRouteTitleIncludesChildSection(t *testing.T) {
	route := Route{Kind: RouteMCP, ResourceID: "github", Section: "oauth"}
	if got, want := route.Title(), "MCP Servers · github · Oauth"; got != want {
		t.Fatalf("title=%q want=%q", got, want)
	}
}

func TestHeaderOwnerTreatsLogsExecAsLogsChild(t *testing.T) {
	if got := headerOwner(RouteLogsExec); got != RouteLogs {
		t.Fatalf("header owner=%s want=%s", got, RouteLogs)
	}
}

func TestRouterBackStackHandlesNestedChildPages(t *testing.T) {
	router := NewRouter(Route{Kind: RouteMCP})
	router.Navigate(Route{Kind: RouteMCP, ResourceID: "github"})
	router.Navigate(Route{Kind: RouteMCP, ResourceID: "github", Section: "tools"})
	if !router.Back() || router.Current().ResourceID != "github" || router.Current().Section != "" {
		t.Fatalf("back to overview=%#v", router.Current())
	}
	if !router.Back() || router.Current() != (Route{Kind: RouteMCP}) {
		t.Fatalf("back to list=%#v", router.Current())
	}
}

func TestRouterBackStack(t *testing.T) {
	router := NewRouter(Route{Kind: RouteHome})
	router.Navigate(Route{Kind: RouteMCP})
	router.Navigate(Route{Kind: RouteMCP, ResourceID: "github"})
	if router.Current().ResourceID != "github" {
		t.Fatalf("current = %#v", router.Current())
	}
	if !router.Back() || router.Current().Kind != RouteMCP || router.Current().ResourceID != "" {
		t.Fatalf("back = %#v", router.Current())
	}
	if !router.Back() || router.Current().Kind != RouteHome {
		t.Fatalf("back home = %#v", router.Current())
	}
	if router.Back() {
		t.Fatal("router backed past root")
	}
}
