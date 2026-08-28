package app

import (
	"go.mewis.me/chatgpt-mcp/internal/config"
	"testing"
)

func TestNewSharesToolRuntime(t *testing.T) {
	app := New(config.Config{})
	if app.Tools == nil || app.MCP == nil || app.MCP.Server == nil {
		t.Fatal("app runtime was not initialized")
	}
	if app.MCP.Server.Tools != app.Tools {
		t.Fatal("MCP and Admin do not share the same tool runtime")
	}
}

func TestBootstrapRewiresToolRuntime(t *testing.T) {
	app := &App{}
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	if app.Tools == nil || app.MCP == nil || app.MCP.Server == nil {
		t.Fatal("bootstrap did not initialize runtime")
	}
	if app.MCP.Server.Tools != app.Tools {
		t.Fatal("bootstrap did not wire shared tool runtime")
	}
}
