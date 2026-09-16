package tui

import (
	"runtime"
	"sort"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/capability"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/tui/action"
	"go.mewis.me/chatgpt-mcp/internal/tui/palette"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

func TestEveryPublicCapabilityHasTUIRepresentation(t *testing.T) {
	registry := defaultActionRegistry()
	represented := map[capability.ID][]action.Action{}
	for _, item := range registry.All() {
		for _, id := range item.Capabilities {
			represented[id] = append(represented[id], item)
		}
	}
	missing := []string{}
	for _, spec := range capability.All() {
		if len(represented[spec.ID]) == 0 {
			missing = append(missing, spec.CanonicalPath)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("missing TUI mappings:\n  %s", strings.Join(missing, "\n  "))
	}
}

func TestMappedCapabilitiesArePaletteDiscoverableByCanonicalCLIPath(t *testing.T) {
	registry := defaultActionRegistry()
	for _, spec := range capability.All() {
		var mapped []action.Action
		for _, item := range registry.All() {
			for _, id := range item.Capabilities {
				if id == spec.ID {
					mapped = append(mapped, item)
					break
				}
			}
		}
		if len(mapped) == 0 {
			continue
		}
		found := false
		for _, item := range mapped {
			if len(palette.Rank([]action.Action{item}, spec.CanonicalPath, action.Context{})) > 0 {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("capability %s is not discoverable by %q", spec.ID, spec.CanonicalPath)
		}
	}
}

func TestCapabilityActionsHaveReachableContexts(t *testing.T) {
	previous := configformat.RootPath()
	t.Cleanup(func() { _ = configformat.SetRootPath(previous) })
	if err := configformat.SetRootPath(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Tunnel = tunnel.Config{AdminKey: "admin-secret", AdminWorkspaceID: "ws_admin", AdminReadAccess: true, AdminManageAccess: true}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	contexts := []action.Context{
		{},
		{Route: string(RouteWorkspaces)}, {Route: string(RouteWorkspaces), ResourceID: "resource"},
		{Route: string(RouteContainers)}, {Route: string(RouteContainers), ResourceID: "resource"},
		{Route: string(RouteMCP)}, {Route: string(RouteMCP), ResourceID: "resource"},
		{Route: string(RoutePlugins)}, {Route: string(RoutePlugins), ResourceID: "resource"},
		{Route: string(RoutePlugins), Section: "marketplace", ResourceID: "resource"},
		{Route: string(RoutePlugins), Section: "updates", ResourceID: "resource"},
		{Route: string(RoutePlugins), Section: "registries", ResourceID: "resource"},
		{Route: string(RouteTunnel)}, {Route: string(RouteTunnel), ResourceID: "resource"}, {Route: string(RouteTunnelAdmins)}, {Route: string(RouteTunnelAdmins), ResourceID: "resource"}, {Route: string(RouteTunnels)}, {Route: string(RouteTunnels), ResourceID: "resource"},
		{Route: string(RouteRequests)}, {Route: string(RouteRequests), ResourceID: "resource"},
		{Route: string(RouteLogs)}, {Route: string(RouteConfig)}, {Route: string(RouteRuntime)}, {Route: string(RouteAbout)},
	}
	for _, item := range defaultActionRegistry().All() {
		if len(item.Capabilities) == 0 || (runtime.GOOS == "windows" && (item.ID == "runtime.up.system" || item.ID == "runtime.down.system" || item.ID == "runtime.restart.system")) {
			continue
		}
		reachable := false
		for _, ctx := range contexts {
			if item.IsAvailable(ctx) {
				reachable = true
				break
			}
		}
		if !reachable {
			t.Errorf("capability action %s has no reachable route context", item.ID)
		}
	}
}

func TestAuthActionsDoNotCollapseCredentialTypes(t *testing.T) {
	for _, item := range defaultActionRegistry().All() {
		hasMCP, hasAdmin, hasTunnel, hasAuthStatus := false, false, false, false
		for _, id := range item.Capabilities {
			switch {
			case strings.HasPrefix(string(id), "auth.mcp."):
				hasMCP = true
			case strings.HasPrefix(string(id), "auth.admin."):
				hasAdmin = true
			case strings.HasPrefix(string(id), "tunnel."):
				hasTunnel = true
			case id == capability.AuthStatus:
				hasAuthStatus = true
			}
			if strings.Contains(strings.ToLower(string(id)), "oauth") {
				t.Errorf("action %s maps leftover oauth capability %s", item.ID, id)
			}
		}
		if hasMCP && hasAdmin {
			t.Errorf("action %s mixes Direct MCP HTTP and admin capabilities", item.ID)
		}
		if (hasMCP || hasAdmin) && hasTunnel {
			t.Errorf("action %s mixes app auth and tunnel capabilities", item.ID)
		}
		if hasAuthStatus && (hasMCP || hasAdmin) {
			t.Errorf("action %s mixes generic auth.status with credential-specific capabilities", item.ID)
		}
	}
}

func TestAuthActionTitlesMatchCredentialType(t *testing.T) {
	for _, item := range defaultActionRegistry().All() {
		for _, id := range item.Capabilities {
			switch {
			case strings.HasPrefix(string(id), "auth.mcp."):
				if !strings.Contains(item.Title, "Direct MCP HTTP") {
					t.Errorf("%s title %q missing Direct MCP HTTP", item.ID, item.Title)
				}
			case strings.HasPrefix(string(id), "auth.admin."):
				if strings.Contains(item.Title, "Direct MCP HTTP") {
					t.Errorf("%s admin action uses Direct MCP HTTP title %q", item.ID, item.Title)
				}
				if !strings.Contains(strings.ToLower(item.Title), "admin") {
					t.Errorf("%s admin title %q missing admin", item.ID, item.Title)
				}
			case strings.HasPrefix(string(id), "tunnel."):
				if strings.Contains(item.Title, "Direct MCP HTTP") {
					t.Errorf("%s tunnel action requires Direct MCP HTTP: %q", item.ID, item.Title)
				}
			}
		}
	}
}

func TestTunnelTUIActionsMapToCanonicalCLICommands(t *testing.T) {
	want := map[string]capability.ID{
		"tunnel.add":               capability.TunnelAdd,
		"tunnel.update":            capability.TunnelUpdate,
		"tunnel.detach":            capability.TunnelDetach,
		"tunnel.enable":            capability.TunnelEnable,
		"tunnel.disable":           capability.TunnelDisable,
		"tunnel.start":             capability.TunnelStart,
		"tunnel.stop":              capability.TunnelStop,
		"tunnel.foreground":        capability.TunnelForeground,
		"tunnel.admin.add":         capability.TunnelAdminAdd,
		"tunnel.admin.update":      capability.TunnelAdminUpdate,
		"tunnel.admin.verify":      capability.TunnelAdminVerify,
		"tunnel.admin.remove":      capability.TunnelAdminRemove,
		"tunnel.managed.refresh":   capability.TunnelManagedList,
		"tunnel.managed.create":    capability.TunnelManagedCreate,
		"tunnel.managed.update":    capability.TunnelManagedUpdate,
		"tunnel.managed.configure": capability.TunnelAttach,
		"tunnel.managed.delete":    capability.TunnelManagedDelete,
	}
	registry := defaultActionRegistry()
	for id, cap := range want {
		spec, ok := capability.Lookup(cap)
		if !ok {
			t.Fatalf("missing capability %s", cap)
		}
		var item action.Action
		for _, candidate := range registry.All() {
			if candidate.ID == id {
				item = candidate
				break
			}
		}
		if item.ID == "" {
			t.Fatalf("missing TUI action %s", id)
		}
		got := capability.NormalizePath(strings.Join(item.CommandPath, " "))
		if got != capability.NormalizePath(spec.CanonicalPath) {
			t.Errorf("%s CommandPath=%q want %q", id, got, spec.CanonicalPath)
		}
		if len(item.Capabilities) != 1 || item.Capabilities[0] != cap {
			t.Errorf("%s capabilities=%v want %s", id, item.Capabilities, cap)
		}
	}
}
