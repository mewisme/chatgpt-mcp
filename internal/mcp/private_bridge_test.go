package mcp

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"go.mewis.me/chatgpt-mcp/internal/tools"
)

func TestPrivateHTTPBridgeHonorsSessionMeta(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegister("bridge_probe", tools.Schema{Name: "bridge_probe", Description: "Probe the private MCP bridge.", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`)}, func(ctx context.Context, _ map[string]any) (tools.Result, error) {
		return tools.JSONResult(map[string]any{"session": tools.MCPSessionID(ctx), "source": tools.CallSource(ctx)}), nil
	})
	runtime := &tools.Runtime{Registry: registry, LoopGuard: tools.NewToolLoopGuard()}
	bridge, err := StartPrivateBridge(runtime)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bridge.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "private-bridge", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: bridge.URL, HTTPClient: BearerHTTPClient(bridge.Token), DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "bridge_probe", Arguments: map[string]any{},
		Meta: sdkmcp.Meta{SessionMetaKey: "openai-session"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || len(result.Content) != 1 {
		t.Fatalf("result=%#v", result)
	}
	text, ok := result.Content[0].(*sdkmcp.TextContent)
	if !ok {
		t.Fatalf("content=%#v", result.Content[0])
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(text.Text), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["source"] != "tunnel" {
		t.Fatalf("source=%#v", payload["source"])
	}
	if payload["session"] != "openai-session" {
		t.Fatalf("session=%#v", payload["session"])
	}
}
