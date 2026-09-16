package tui

import (
	"sort"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/capability"
	"go.mewis.me/chatgpt-mcp/internal/testutil"
	"go.mewis.me/chatgpt-mcp/internal/tui/action"
)

type tuiInventoryAction struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Category     string   `json:"category"`
	CommandPath  string   `json:"command_path,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Kind         string   `json:"kind"`
}

func TestSurfaceInventoryTUIActions(t *testing.T) {
	items := collectTUIInventory()
	if len(items) < 40 {
		t.Fatalf("too few TUI actions: %d", len(items))
	}
	byID := map[string]tuiInventoryAction{}
	for _, item := range items {
		byID[item.ID] = item
	}
	update := byID["tunnel.admin.update"]
	if update.CommandPath != "tunnel admin update" {
		t.Fatalf("tunnel.admin.update command_path=%q", update.CommandPath)
	}
	if byID["runtime.foreground"].Kind != "external-command" {
		t.Fatalf("runtime.foreground kind=%q", byID["runtime.foreground"].Kind)
	}
	if byID["plugin.install"].Kind != "native" {
		t.Fatalf("plugin.install kind=%q", byID["plugin.install"].Kind)
	}
	if byID["tunnel.add"].Kind != "native" {
		t.Fatalf("tunnel.add kind=%q", byID["tunnel.add"].Kind)
	}
	for _, id := range []string{
		"tunnel.update", "tunnel.enable", "tunnel.disable", "tunnel.start", "tunnel.stop", "tunnel.detach",
		"tunnel.admin.add", "tunnel.admin.update", "tunnel.admin.verify", "tunnel.admin.remove",
		"tunnel.managed.create", "tunnel.managed.update", "tunnel.managed.configure", "tunnel.managed.delete",
		"plugin.uninstall", "plugin.enable", "plugin.disable", "plugin.update", "plugin.rollback", "plugin.prune", "plugin.verify",
		"plugin.config.reset", "workspace.relocate", "workspace.access.add", "workspace.access.remove",
		"request.approve", "request.deny", "request.grant.list", "request.grant.revoke",
		"config.verify", "config.export", "config.import",
	} {
		if byID[id].ID == "" {
			t.Fatalf("missing compared TUI action %s", id)
		}
	}
	if byID["config.initialize.external"].Kind != "external-command" {
		t.Fatalf("config.initialize.external kind=%q", byID["config.initialize.external"].Kind)
	}
	testutil.WriteLocalJSON(t, "surface-parity-tui.json", map[string]any{
		"routes": []string{
			string(RouteHome), string(RouteWorkspaces), string(RouteContainers), string(RouteMCP), string(RoutePlugins),
			string(RouteTunnel), string(RouteTunnelAdmins), string(RouteTunnels), string(RouteRequests), string(RouteLogs),
			string(RouteLogsExec), string(RouteLogsTools), string(RouteConfig), string(RouteInstruction), string(RouteRuntime),
			string(RouteAbout), string(RouteGuide),
		},
		"actions": items,
		"native_roots": map[string]string{
			"global_rules":     "<config-root>/rules",
			"global_skills":    "<config-root>/skills",
			"workspace_rules":  ".cgm/rules",
			"workspace_skills": ".cgm/skills",
		},
	})
}

func TestTUISingleCapabilityCommandPathsMatchCatalog(t *testing.T) {
	for _, item := range defaultActionRegistry().All() {
		if len(item.Capabilities) != 1 || len(item.CommandPath) == 0 || item.CommandPath[0] == "tui" {
			continue
		}
		got := capability.NormalizePath(strings.Join(item.CommandPath, " "))
		id, ok := capability.ForPath(got)
		if !ok || id != item.Capabilities[0] {
			t.Errorf("%s CommandPath=%q maps to %s want %s", item.ID, got, id, item.Capabilities[0])
		}
	}
}

func collectTUIInventory() []tuiInventoryAction {
	out := []tuiInventoryAction{}
	for _, item := range defaultActionRegistry().All() {
		caps := make([]string, 0, len(item.Capabilities))
		for _, id := range item.Capabilities {
			caps = append(caps, string(id))
		}
		sort.Strings(caps)
		out = append(out, tuiInventoryAction{
			ID: item.ID, Title: item.Title, Category: item.Category,
			CommandPath:  capability.NormalizePath(strings.Join(item.CommandPath, " ")),
			Capabilities: caps, Kind: tuiActionKind(item),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func TestCLIOnlyBulkPluginOptionsStayOffTUIActions(t *testing.T) {
	forbidden := []string{"--all", "--retain", "--cache", "--strict"}
	for _, item := range defaultActionRegistry().All() {
		path := strings.Join(item.CommandPath, " ")
		for _, flag := range forbidden {
			if strings.Contains(path, flag) {
				t.Errorf("TUI action %s embeds CLI-only flag %s in CommandPath %q", item.ID, flag, path)
			}
		}
	}
}

func tuiActionKind(item action.Action) string {
	if strings.Contains(item.ID, ".external") || strings.HasSuffix(item.ID, ".foreground") {
		return "external-command"
	}
	return "native"
}
