package upstream

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPModernToolsAndCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("MCP-Protocol-Version") != ModernProtocol {
			t.Fatalf("protocol header = %q", r.Header.Get("MCP-Protocol-Version"))
		}
		var request rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "server/discover":
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"capabilities": map[string]any{}}})
		case "tools/list":
			if r.Header.Get("Mcp-Method") != "tools/list" {
				t.Fatalf("method header = %q", r.Header.Get("Mcp-Method"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": request.ID,
				"result": map[string]any{"tools": []any{map[string]any{
					"name": "echo", "description": "Echo", "inputSchema": map[string]any{"type": "object"},
				}}, "ttlMs": 60000, "cacheScope": "public"},
			})
		case "tools/call":
			if r.Header.Get("Mcp-Name") != "echo" {
				t.Fatalf("name header = %q", r.Header.Get("Mcp-Name"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": request.ID,
				"result": map[string]any{"content": []any{map[string]any{"type": "text", "text": "ok"}}, "structuredContent": map[string]any{"ok": true}},
			})
		default:
			t.Fatalf("unexpected method %q", request.Method)
		}
	}))
	defer server.Close()

	client := NewNativeClient()
	config := Server{ID: "test", Name: "Test", Enabled: true, Transport: "http", URL: server.URL, Expose: "all"}
	if err := client.Connect(context.Background(), config); err != nil {
		t.Fatal(err)
	}
	tools, err := client.Tools(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("tools = %#v", tools)
	}
	result, err := client.Call(context.Background(), "test", "echo", map[string]any{"value": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || len(result.Content) != 1 || result.Content[0].Text != "ok" {
		t.Fatalf("result = %#v", result)
	}
}

func TestHTTPModernProtocolError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request rpcRequest
		_ = json.NewDecoder(r.Body).Decode(&request)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0", "id": request.ID,
			"error": map[string]any{"code": -32020, "message": "HeaderMismatch"},
		})
	}))
	defer server.Close()
	client := NewNativeClient()
	err := client.Connect(context.Background(), Server{ID: "test", Enabled: true, Transport: "http", URL: server.URL})
	if err == nil {
		t.Fatal("expected connect error")
	}
}
