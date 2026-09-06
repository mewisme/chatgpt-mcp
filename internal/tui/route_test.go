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
	for _, args := range [][]string{{"missing"}, {"logs", "extra"}, {"tunnel", "extra"}, {"mcp", "a", "b"}} {
		if _, err := ParseRoute(args); err == nil {
			t.Fatalf("ParseRoute(%v) unexpectedly succeeded", args)
		}
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
