package upstream

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCallWithInputRelaysMRTRParamsAndCapabilities(t *testing.T) {
	var sawCapabilities bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "server/discover":
			meta, _ := request.Params["_meta"].(map[string]any)
			capabilities, _ := meta["io.modelcontextprotocol/clientCapabilities"].(map[string]any)
			_, sawCapabilities = capabilities["elicitation"]
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"capabilities": map[string]any{}}})
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": request.ID,
				"result": map[string]any{"tools": []any{map[string]any{"name": "confirm", "inputSchema": map[string]any{"type": "object"}}}},
			})
		case "tools/call":
			if request.Params["requestState"] != "opaque-state" {
				t.Fatalf("requestState = %#v", request.Params["requestState"])
			}
			responses, _ := request.Params["inputResponses"].(map[string]any)
			confirm, _ := responses["confirm"].(map[string]any)
			if confirm["action"] != "accept" {
				t.Fatalf("inputResponses = %#v", responses)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": request.ID,
				"result": map[string]any{
					"resultType": "complete",
					"content":    []any{map[string]any{"type": "text", "text": "done"}},
				},
			})
		default:
			t.Fatalf("unexpected method %q", request.Method)
		}
	}))
	defer server.Close()

	client := NewNativeClient()
	ctx := WithRequestMeta(context.Background(), map[string]any{
		"io.modelcontextprotocol/clientCapabilities": map[string]any{"elicitation": map[string]any{}},
	})
	if err := client.Connect(ctx, Server{ID: "test", Enabled: true, Transport: "http", URL: server.URL}); err != nil {
		t.Fatal(err)
	}
	result, err := client.CallWithInput(ctx, "test", "confirm", map[string]any{}, "opaque-state", map[string]any{
		"confirm": map[string]any{"action": "accept", "content": map[string]any{"confirmed": true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sawCapabilities {
		t.Fatal("outer client capabilities were not propagated to upstream _meta")
	}
	if result.ResultType != "complete" || len(result.Content) != 1 || result.Content[0].Text != "done" {
		t.Fatalf("result = %#v", result)
	}
}
