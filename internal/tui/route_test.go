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
		{[]string{"ws", "ws_abc", "context"}, Route{Kind: RouteWorkspaces, ResourceID: "ws_abc", Section: "context"}},
		{[]string{"ws", "ws_abc", "context-preview"}, Route{Kind: RouteWorkspaces, ResourceID: "ws_abc", Section: "context-preview"}},
		{[]string{"ws", "ws_abc", "overview"}, Route{Kind: RouteWorkspaces, ResourceID: "ws_abc"}},
		{[]string{"containers", "wsc_abc", "workspaces"}, Route{Kind: RouteContainers, ResourceID: "wsc_abc", Section: "workspaces"}},
		{[]string{"mcp", "github", "health"}, Route{Kind: RouteMCP, ResourceID: "github", Section: "health"}},
		{[]string{"tunnels", "tunnel_abc", "scope"}, Route{Kind: RouteTunnels, ResourceID: "tunnel_abc", Section: "scope"}},
		{[]string{"requests", "req_abc", "guard"}, Route{Kind: RouteRequests, Mode: "all", ResourceID: "req_abc", Section: "guard"}},
		{[]string{"requests", "pending"}, Route{Kind: RouteRequests, Mode: "pending"}},
		{[]string{"requests", "history", "req_abc"}, Route{Kind: RouteRequests, Mode: "history", ResourceID: "req_abc"}},
		{[]string{"requests", "all", "req_abc", "arguments"}, Route{Kind: RouteRequests, Mode: "all", ResourceID: "req_abc", Section: "arguments"}},
		{[]string{"config", "runtime.port"}, Route{Kind: RouteConfig, ResourceID: "runtime.port"}},
		{[]string{"instruction"}, Route{Kind: RouteInstruction}},
		{[]string{"instructions"}, Route{Kind: RouteInstruction}},
		{[]string{"instr"}, Route{Kind: RouteInstruction}},
		{[]string{"instruction", "context"}, Route{Kind: RouteInstruction, Section: "context"}},
		{[]string{"instruction", "rules"}, Route{Kind: RouteInstruction, Section: "rules"}},
		{[]string{"instructions", "sources"}, Route{Kind: RouteInstruction, Section: "sources"}},
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
	for _, args := range [][]string{{"missing"}, {"tunnel", "extra"}, {"mcp", "a", "missing"}, {"config", "key", "extra"}, {"logs-exec", "extra"}, {"mcp", "a", "health", "extra"}, {"requests", "history", "req", "guard", "extra"}, {"instruction", "missing"}, {"instruction", "rules", "extra"}} {
		if _, err := ParseRoute(args); err == nil {
			t.Fatalf("ParseRoute(%v) unexpectedly succeeded", args)
		}
	}
}

func TestParseEditorRoutes(t *testing.T) {
	tests := []struct {
		args []string
		want Route
	}{
		{[]string{"workspaces", "register"}, Route{Kind: RouteWorkspaces, Action: "register"}},
		{[]string{"workspaces", "ws_1", "access", "add"}, Route{Kind: RouteWorkspaces, ResourceID: "ws_1", Section: "access", Action: "add"}},
		{[]string{"workspaces", "ws_1", "access", "remove"}, Route{Kind: RouteWorkspaces, ResourceID: "ws_1", Section: "access", Action: "remove"}},
		{[]string{"containers", "create"}, Route{Kind: RouteContainers, Action: "create"}},
		{[]string{"containers", "wsc_1", "edit"}, Route{Kind: RouteContainers, ResourceID: "wsc_1", Action: "edit"}},
		{[]string{"containers", "wsc_1", "workspaces", "edit"}, Route{Kind: RouteContainers, ResourceID: "wsc_1", Section: "workspaces", Action: "edit"}},
		{[]string{"mcp", "create"}, Route{Kind: RouteMCP, Action: "create"}},
		{[]string{"mcp", "github", "edit"}, Route{Kind: RouteMCP, ResourceID: "github", Action: "edit"}},
		{[]string{"mcp", "github", "oauth", "login"}, Route{Kind: RouteMCP, ResourceID: "github", Section: "oauth", Action: "login"}},
		{[]string{"tunnel", "edit"}, Route{Kind: RouteTunnel, Action: "edit"}},
		{[]string{"tunnel", "admin-key", "edit"}, Route{Kind: RouteTunnel, Section: "admin-key", Action: "edit"}},
		{[]string{"tunnels", "create"}, Route{Kind: RouteTunnels, Action: "create"}},
		{[]string{"tunnels", "tun_1", "edit"}, Route{Kind: RouteTunnels, ResourceID: "tun_1", Action: "edit"}},
		{[]string{"tunnels", "tun_1", "configure"}, Route{Kind: RouteTunnels, ResourceID: "tun_1", Action: "configure"}},
		{[]string{"config", "runtime.port", "edit"}, Route{Kind: RouteConfig, ResourceID: "runtime.port", Action: "edit"}},
		{[]string{"config", "storage", "convert"}, Route{Kind: RouteConfig, Section: "storage", Action: "convert"}},
		{[]string{"config", "storage", "export"}, Route{Kind: RouteConfig, Section: "storage", Action: "export"}},
		{[]string{"config", "storage", "import"}, Route{Kind: RouteConfig, Section: "storage", Action: "import"}},
		{[]string{"runtime", "install"}, Route{Kind: RouteRuntime, Action: "install"}},
		{[]string{"runtime", "update"}, Route{Kind: RouteRuntime, Action: "update"}},
		{[]string{"requests", "create-test"}, Route{Kind: RouteRequests, Action: "create-test"}},
		{[]string{"requests", "pending", "req_1", "approve"}, Route{Kind: RouteRequests, Mode: "pending", ResourceID: "req_1", Action: "approve"}},
		{[]string{"requests", "all", "req_1", "deny"}, Route{Kind: RouteRequests, Mode: "all", ResourceID: "req_1", Action: "deny"}},
		{[]string{"logs", "filter"}, Route{Kind: RouteLogs, Action: "filter"}},
		{[]string{"instruction", "context", "edit"}, Route{Kind: RouteInstruction, Section: "context", Action: "edit"}},
		{[]string{"instruction", "rules", "create"}, Route{Kind: RouteInstruction, Section: "rules", Action: "create"}},
		{[]string{"instruction", "rules", "rule_1", "edit"}, Route{Kind: RouteInstruction, ResourceID: "rule_1", Section: "rules", Action: "edit"}},
	}
	for _, test := range tests {
		got, err := ParseRoute(test.args)
		if err != nil || got != test.want {
			t.Fatalf("ParseRoute(%v) = %#v, %v; want %#v", test.args, got, err, test.want)
		}
	}
}

func TestParseEditorRoutesRejectsMalformedPaths(t *testing.T) {
	for _, args := range [][]string{
		{"workspaces", "register", "extra"},
		{"workspaces", "ws_1", "access", "edit"},
		{"containers", "wsc_1", "workspaces", "create"},
		{"mcp", "github", "oauth", "edit"},
		{"tunnel", "admin-key"},
		{"tunnels", "tun_1", "create"},
		{"config", "storage", "edit"},
		{"runtime", "install", "extra"},
		{"requests", "req_1", "approve"},
		{"logs", "filter", "extra"},
		{"instruction", "context", "create"},
		{"instruction", "rules", "rule_1", "remove"},
	} {
		if got, err := ParseRoute(args); err == nil {
			t.Fatalf("ParseRoute(%v) unexpectedly succeeded: %#v", args, got)
		}
	}
}

func TestEditorRouteStacksFollowSemanticAncestry(t *testing.T) {
	tests := []struct {
		route Route
		want  []Route
	}{
		{
			Route{Kind: RouteMCP, Action: "create"},
			[]Route{{Kind: RouteMCP}, {Kind: RouteMCP, Action: "create"}},
		},
		{
			Route{Kind: RouteMCP, ResourceID: "github", Action: "edit"},
			[]Route{{Kind: RouteMCP}, {Kind: RouteMCP, ResourceID: "github"}, {Kind: RouteMCP, ResourceID: "github", Action: "edit"}},
		},
		{
			Route{Kind: RouteWorkspaces, ResourceID: "ws_1", Section: "access", Action: "add"},
			[]Route{{Kind: RouteWorkspaces}, {Kind: RouteWorkspaces, ResourceID: "ws_1"}, {Kind: RouteWorkspaces, ResourceID: "ws_1", Section: "access"}, {Kind: RouteWorkspaces, ResourceID: "ws_1", Section: "access", Action: "add"}},
		},
		{
			Route{Kind: RouteInstruction, Section: "rules", Action: "create"},
			[]Route{{Kind: RouteInstruction, Section: "rules"}, {Kind: RouteInstruction, Section: "rules", Action: "create"}},
		},
		{
			Route{Kind: RouteInstruction, ResourceID: "rule_1", Section: "rules", Action: "edit"},
			[]Route{{Kind: RouteInstruction, Section: "rules"}, {Kind: RouteInstruction, ResourceID: "rule_1", Section: "rules", Action: "edit"}},
		},
		{
			Route{Kind: RouteConfig, Section: "storage", Action: "export"},
			[]Route{{Kind: RouteConfig}, {Kind: RouteConfig, Section: "storage", Action: "export"}},
		},
		{
			Route{Kind: RouteTunnel, Section: "admin-key", Action: "edit"},
			[]Route{{Kind: RouteTunnel}, {Kind: RouteTunnel, Section: "admin-key", Action: "edit"}},
		},
	}
	for _, test := range tests {
		got := routeStack(test.route)
		if len(got) != len(test.want) {
			t.Fatalf("routeStack(%#v)=%#v want %#v", test.route, got, test.want)
		}
		for index := range got {
			if got[index] != test.want[index] {
				t.Fatalf("routeStack(%#v)[%d]=%#v want %#v", test.route, index, got[index], test.want[index])
			}
		}
	}
}

func TestEditorRouteTitlesIncludeActionWithoutChangingLegacyOrder(t *testing.T) {
	for route, want := range map[Route]string{
		{Kind: RouteMCP, Action: "create"}:                                               "MCP Servers · Create",
		{Kind: RouteMCP, ResourceID: "github", Action: "edit"}:                           "MCP Servers · github · Edit",
		{Kind: RouteMCP, ResourceID: "github", Section: "oauth", Action: "login"}:        "MCP Servers · github · Oauth · Login",
		{Kind: RouteInstruction, Section: "context", Action: "edit"}:                     "Instruction · Context · Edit",
		{Kind: RouteInstruction, ResourceID: "rule_1", Section: "rules", Action: "edit"}: "Instruction · Rules · rule_1 · Edit",
	} {
		if got := route.Title(); got != want {
			t.Fatalf("%#v title=%q want=%q", route, got, want)
		}
	}
}

func TestInstructionTabsDoNotCreateRouterHistory(t *testing.T) {
	route := Route{Kind: RouteInstruction, Section: "rules"}
	router := NewRouter(route)
	if router.Current() != route || len(router.stack) != 1 {
		t.Fatalf("instruction route stack=%#v", router.stack)
	}
	if router.Back() {
		t.Fatalf("instruction tab became a back-stack level: %#v", router.stack)
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

func TestRouterBackReturnsToTabbedParents(t *testing.T) {
	workspaceRouter := NewRouter(Route{Kind: RouteWorkspaces})
	workspaceRouter.Switch(Route{Kind: RouteContainers})
	workspaceRouter.Navigate(Route{Kind: RouteContainers, ResourceID: "wsc_demo"})
	if !workspaceRouter.Back() || workspaceRouter.Current() != (Route{Kind: RouteContainers}) {
		t.Fatalf("workspace detail back=%#v stack=%#v", workspaceRouter.Current(), workspaceRouter.stack)
	}

	logsRouter := NewRouter(Route{Kind: RouteLogs})
	logsRouter.Navigate(Route{Kind: RouteLogs, ResourceID: "run:1"})
	if !logsRouter.Back() || logsRouter.Current() != (Route{Kind: RouteLogs}) {
		t.Fatalf("logs detail back=%#v stack=%#v", logsRouter.Current(), logsRouter.stack)
	}

	requestsRouter := NewRouter(Route{Kind: RouteRequests, Mode: "history"})
	requestsRouter.Navigate(Route{Kind: RouteRequests, Mode: "history", ResourceID: "req_demo"})
	if !requestsRouter.Back() || requestsRouter.Current() != (Route{Kind: RouteRequests, Mode: "history"}) {
		t.Fatalf("requests detail back=%#v stack=%#v", requestsRouter.Current(), requestsRouter.stack)
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
	if router.Back() {
		t.Fatal("router backed past current main page")
	}
}

func TestRouterCrossPageNavigationDropsPreviousPageHistory(t *testing.T) {
	router := NewRouter(Route{Kind: RouteWorkspaces, ResourceID: "ws_old"})
	router.Navigate(Route{Kind: RouteLogs})
	router.Navigate(Route{Kind: RouteLogs, ResourceID: "event_new"})
	if len(router.stack) != 2 || router.stack[0] != (Route{Kind: RouteLogs}) || router.stack[1].ResourceID != "event_new" {
		t.Fatalf("cross-page stack=%#v", router.stack)
	}
	if !router.Back() || router.Current() != (Route{Kind: RouteLogs}) {
		t.Fatalf("back to current main=%#v stack=%#v", router.Current(), router.stack)
	}
	if router.Back() {
		t.Fatalf("back leaked into previous page history: %#v", router.stack)
	}
}
