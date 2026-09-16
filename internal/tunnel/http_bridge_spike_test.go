package tunnel

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	tunnelclient "github.com/openai/tunnel-client"

	localmcp "go.mewis.me/chatgpt-mcp/internal/mcp"
	"go.mewis.me/chatgpt-mcp/internal/tools"
)

func TestOpenAITunnelClientAcceptsStreamableHTTPTransport(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegister("bridge_probe", tools.Schema{Name: "bridge_probe", Description: "Probe the private MCP bridge.", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`)}, func(ctx context.Context, _ map[string]any) (tools.Result, error) {
		return tools.JSONResult(map[string]any{"session": tools.MCPSessionID(ctx), "source": tools.CallSource(ctx)}), nil
	})
	handler, err := localmcp.NewSDKHTTPHandler(&tools.Runtime{Registry: registry, LoopGuard: tools.NewToolLoopGuard()}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	transport := &sdkmcp.StreamableClientTransport{Endpoint: server.URL + "/mcp", DisableStandaloneSSE: true}
	client, err := tunnelclient.New(tunnelclient.Config{TunnelID: "tunnel_0123456789abcdef0123456789abcdef", APIKey: "sk-spike"}, transport)
	if err != nil {
		t.Fatal(err)
	}
	if client == nil {
		t.Fatal("tunnel client is nil")
	}
}

func TestPrivateHTTPBridgeSeesHTTPSessionNotTunnelContext(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegister("bridge_probe", tools.Schema{Name: "bridge_probe", Description: "Probe the private MCP bridge.", InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`)}, func(ctx context.Context, _ map[string]any) (tools.Result, error) {
		return tools.JSONResult(map[string]any{"session": tools.MCPSessionID(ctx), "source": tools.CallSource(ctx)}), nil
	})
	handler, err := localmcp.NewSDKHTTPHandler(&tools.Runtime{Registry: registry, LoopGuard: tools.NewToolLoopGuard()}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "secure-mcp-spike", Version: "1.0.0"}, nil)
	session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: server.URL + "/mcp", DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	result, err := session.CallTool(ctx, &sdkmcp.CallToolParams{
		Name: "bridge_probe", Arguments: map[string]any{},
		Meta: sdkmcp.Meta{sessionMetaKey: "openai-session"},
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
	if payload["source"] != "http" {
		t.Fatalf("source=%#v", payload["source"])
	}
	if payload["session"] == "openai-session" {
		t.Fatal("public HTTP MCP already honors tunnel session meta; private-bridge adapter is unnecessary")
	}
	if payload["session"] == "" {
		t.Fatal("HTTP MCP session id missing")
	}
}
