package palette

import (
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/tui/action"
)

func TestRankPrefersTitleAndCommandPathMatches(t *testing.T) {
	actions := []action.Action{
		{ID: "logs", Title: "Go to Logs", Category: "App", Keywords: []string{"journal"}, CommandPath: []string{"tui", "logs"}},
		{ID: "verify", Title: "Verify configuration", Category: "Config", Keywords: []string{"validate"}, CommandPath: []string{"config", "verify"}},
		{ID: "workspace", Title: "Register workspace", Category: "Workspace", CommandPath: []string{"workspace", "register"}},
	}
	results := Rank(actions, "config verify", action.Context{})
	if len(results) == 0 || results[0].Action.ID != "verify" {
		t.Fatalf("results = %#v", results)
	}
	results = Rank(actions, "reg ws", action.Context{})
	if len(results) == 0 || results[0].Action.ID != "workspace" {
		t.Fatalf("fuzzy results = %#v", results)
	}
}

func TestRankBoostsCurrentResource(t *testing.T) {
	actions := []action.Action{
		{ID: "generic", Title: "Configure server", Category: "MCP", CommandPath: []string{"mcp", "server", "configure"}},
		{ID: "current", Title: "Configure github", Category: "MCP", Keywords: []string{"github"}, CommandPath: []string{"mcp", "server", "configure", "github"}},
	}
	results := Rank(actions, "configure", action.Context{Route: "mcp", ResourceID: "github"})
	if len(results) != 2 || results[0].Action.ID != "current" {
		t.Fatalf("results = %#v", results)
	}
}

func TestRecentActionsBoostEmptyPaletteButNotStrongQuery(t *testing.T) {
	actions := []action.Action{
		{ID: "logs", Title: "Go to Logs", Category: "App"},
		{ID: "config", Title: "Verify configuration", Category: "Config", CommandPath: []string{"config", "verify"}},
	}
	results := RankWithRecent(actions, "", action.Context{}, []string{"config"})
	if len(results) != 2 || results[0].Action.ID != "config" {
		t.Fatalf("recent results = %#v", results)
	}
	results = RankWithRecent(actions, "logs", action.Context{}, []string{"config"})
	if len(results) != 1 || results[0].Action.ID != "logs" {
		t.Fatalf("query results = %#v", results)
	}
}
