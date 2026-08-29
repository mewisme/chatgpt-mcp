package app

import (
	"context"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
)

func TestNewSharesToolRuntime(t *testing.T) {
	app := New(config.Config{})
	if app.Tools == nil || app.MCP == nil || app.MCP.Server == nil {
		t.Fatal("app runtime was not initialized")
	}
	if app.MCP.Server.Tools != app.Tools {
		t.Fatal("MCP and Admin do not share the same tool runtime")
	}
	if app.Upstream != app.Tools.Upstream {
		t.Fatal("Admin and tool runtime do not share the same upstream manager")
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
	if app.Upstream != app.Tools.Upstream {
		t.Fatal("bootstrap did not wire shared upstream manager")
	}
}

type lifecycleUpstreamClient struct {
	closed []string
}

func (*lifecycleUpstreamClient) Connect(context.Context, upstream.Server) error { return nil }
func (c *lifecycleUpstreamClient) Close(_ context.Context, id string) error {
	c.closed = append(c.closed, id)
	return nil
}
func (*lifecycleUpstreamClient) Tools(context.Context, string) ([]upstream.Tool, error) {
	return nil, nil
}
func (*lifecycleUpstreamClient) Call(context.Context, string, string, map[string]any) (upstream.CallResult, error) {
	return upstream.CallResult{}, nil
}
func (*lifecycleUpstreamClient) PID(string) int { return 0 }

func TestStopShutsDownUpstreamConnections(t *testing.T) {
	client := &lifecycleUpstreamClient{}
	manager := upstream.NewManagerWithClient(nil, client)
	if err := manager.Add(upstream.Server{ID: "one", Name: "One", Enabled: true, Transport: "http", URL: "http://example.test/mcp"}); err != nil {
		t.Fatal(err)
	}
	if err := (&App{Upstream: manager}).Stop(); err != nil {
		t.Fatal(err)
	}
	if len(client.closed) != 1 || client.closed[0] != "one" {
		t.Fatalf("closed = %#v", client.closed)
	}
}
