package tools

import (
	"context"
	"encoding/json"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/upstream"
)

type bridgeClient struct {
	tools  []upstream.Tool
	result upstream.CallResult
}

func (*bridgeClient) Connect(context.Context, upstream.Server) error { return nil }
func (*bridgeClient) Close(context.Context, string) error            { return nil }
func (c *bridgeClient) Tools(context.Context, string) ([]upstream.Tool, error) {
	return append([]upstream.Tool(nil), c.tools...), nil
}
func (c *bridgeClient) Call(context.Context, string, string, map[string]any) (upstream.CallResult, error) {
	return c.result, nil
}
func (*bridgeClient) PID(string) int { return 0 }

func TestMCPBridgeAndProxyRegistration(t *testing.T) {
	client := &bridgeClient{
		tools: []upstream.Tool{{
			Name: "echo", Description: "Echo",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"text": map[string]any{"type": "string"}}},
		}},
		result: upstream.CallResult{Content: []upstream.Content{{Type: "text", Text: "hello"}}, StructuredContent: map[string]any{"value": "hello"}},
	}
	manager := upstream.NewManagerWithClient(nil, client)
	if err := manager.Add(upstream.Server{
		ID: "demo", Name: "Demo", Enabled: true, Transport: "http", URL: "http://example.test", Expose: "all",
	}); err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	RegisterUpstreamTools(registry, manager)

	toolsResult, err := registry.Call(context.Background(), "mcp_tools", map[string]any{"server_id": "demo"})
	if err != nil || toolsResult.IsError {
		t.Fatalf("mcp_tools failed: %#v %v", toolsResult, err)
	}
	found := false
	for _, schema := range registry.ListSchemas() {
		if schema.Name == "demo__echo" {
			found = true
			var input map[string]any
			if err := json.Unmarshal(schema.InputSchema, &input); err != nil {
				t.Fatal(err)
			}
		}
	}
	if !found {
		t.Fatal("dynamic proxy was not registered")
	}
	proxy, err := registry.Call(context.Background(), "demo__echo", map[string]any{"text": "hello"})
	if err != nil || proxy.IsError {
		t.Fatalf("proxy failed: %#v %v", proxy, err)
	}
	if proxy.StructuredContent == nil {
		t.Fatal("proxy did not forward structured content")
	}
}

func TestMCPCallNormalizesUpstreamError(t *testing.T) {
	client := &bridgeClient{
		result: upstream.CallResult{Content: []upstream.Content{{Type: "text", Text: "bad"}}, IsError: true},
	}
	manager := upstream.NewManagerWithClient(nil, client)
	if err := manager.Add(upstream.Server{ID: "demo", Enabled: true, Transport: "http", URL: "http://example.test", Expose: "all"}); err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry()
	RegisterUpstreamTools(registry, manager)
	result, err := registry.Call(context.Background(), "mcp_call", map[string]any{"server_id": "demo", "tool": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("expected tool error: %#v", result)
	}
}
