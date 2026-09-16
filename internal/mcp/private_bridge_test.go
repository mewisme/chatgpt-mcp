package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
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
	session, err := client.Connect(ctx, &sdkmcp.StreamableClientTransport{Endpoint: bridge.URL, DisableStandaloneSSE: true}, nil)
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

func TestPrivateHTTPBridgeAcceptsChatGPTPingHeaders(t *testing.T) {
	runtime := &tools.Runtime{Registry: tools.NewRegistry(), LoopGuard: tools.NewToolLoopGuard()}
	bridge, err := StartPrivateBridge(runtime)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bridge.Close() })

	req, err := http.NewRequest(http.MethodPost, bridge.URL, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	if !bytes.Contains(body, []byte("jsonrpc")) {
		t.Fatalf("non-MCP body %q", body)
	}
}

func TestPrivateHTTPBridgeAllowsToolCallWithoutInitialize(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegister("bridge_probe", tools.Schema{
		Name: "bridge_probe", Description: "Probe the private MCP bridge.",
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
	}, func(ctx context.Context, _ map[string]any) (tools.Result, error) {
		return tools.JSONResult(map[string]any{"ok": true}), nil
	})
	bridge, err := StartPrivateBridge(&tools.Runtime{Registry: registry, LoopGuard: tools.NewToolLoopGuard()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = bridge.Close() })

	req, err := http.NewRequest(http.MethodPost, bridge.URL, strings.NewReader(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"bridge_probe","arguments":{}}}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Mcp-Session-Id", "chatgpt-uninitialized")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%s", resp.StatusCode, body)
	}
	if bytes.Contains(body, []byte("invalid during session initialization")) {
		t.Fatalf("still initializing: %s", body)
	}
	if !bytes.Contains(body, []byte(`"ok"`)) && !bytes.Contains(body, []byte("ok")) {
		t.Fatalf("missing tool result: %s", body)
	}
}
