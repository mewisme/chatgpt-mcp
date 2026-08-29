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

func TestMCPHeaderValueEncoding(t *testing.T) {
	cases := map[string]string{
		"us-west1":           "us-west1",
		"Hello, 世界":          "=?base64?SGVsbG8sIOS4lueVjA==?=",
		" padded ":           "=?base64?IHBhZGRlZCA=?=",
		"=?base64?literal?=": "=?base64?PT9iYXNlNjQ/bGl0ZXJhbD89?=",
		"":                   "",
	}
	for value, want := range cases {
		if got := encodeMCPHeaderValue(value); got != want {
			t.Fatalf("encode %q = %q, want %q", value, got, want)
		}
	}
}

func TestMirroredToolHeaders(t *testing.T) {
	tool := Tool{InputSchema: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"region":  map[string]any{"type": "string", "x-mcp-header": "Region"},
			"count":   map[string]any{"type": "integer", "x-mcp-header": "Count"},
			"enabled": map[string]any{"type": "boolean", "x-mcp-header": "Enabled"},
			"ignored": map[string]any{"type": "string"},
		},
	}}
	headers, err := mirroredToolHeaders(tool, map[string]any{"region": "Hello, 世界", "count": float64(42), "enabled": true, "ignored": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if headers["Mcp-Param-Region"] != "=?base64?SGVsbG8sIOS4lueVjA==?=" || headers["Mcp-Param-Count"] != "42" || headers["Mcp-Param-Enabled"] != "true" {
		t.Fatalf("headers = %#v", headers)
	}
	if _, ok := headers["Mcp-Param-Ignored"]; ok {
		t.Fatalf("unexpected mirrored header: %#v", headers)
	}
}

func TestToolHeaderSpecsRejectNestedAndNonPrimitiveAnnotations(t *testing.T) {
	for name, schema := range map[string]map[string]any{
		"nested": {
			"type": "object",
			"properties": map[string]any{"outer": map[string]any{
				"type":       "object",
				"properties": map[string]any{"inner": map[string]any{"type": "string", "x-mcp-header": "Inner"}},
			}},
		},
		"object": {
			"type":       "object",
			"properties": map[string]any{"value": map[string]any{"type": "object", "x-mcp-header": "Value"}},
		},
		"duplicate": {
			"type": "object",
			"properties": map[string]any{
				"a": map[string]any{"type": "string", "x-mcp-header": "Region"},
				"b": map[string]any{"type": "string", "x-mcp-header": "region"},
			},
		},
	} {
		if _, err := toolHeaderSpecs(schema); err == nil {
			t.Fatalf("%s schema was accepted", name)
		}
	}
}

func TestHTTPModernMirrorsParamHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "server/discover":
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"capabilities": map[string]any{}}})
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": request.ID,
				"result": map[string]any{"tools": []any{map[string]any{
					"name": "echo", "inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"region": map[string]any{"type": "string", "x-mcp-header": "Region"},
						},
					},
				}}},
			})
		case "tools/call":
			if r.Header.Get("Mcp-Param-Region") != "us-west1" {
				t.Fatalf("param header = %q", r.Header.Get("Mcp-Param-Region"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"content": []any{}}})
		default:
			t.Fatalf("unexpected method %q", request.Method)
		}
	}))
	defer server.Close()

	client := NewNativeClient()
	if err := client.Connect(context.Background(), Server{ID: "test", Enabled: true, Transport: "http", URL: server.URL}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Call(context.Background(), "test", "echo", map[string]any{"region": "us-west1"}); err != nil {
		t.Fatal(err)
	}
}

func TestHTTPModernHeaderMismatchRefreshesSchemaAndRetries(t *testing.T) {
	listCount := 0
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "server/discover":
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"capabilities": map[string]any{}}})
		case "tools/list":
			listCount++
			header := "Old"
			if listCount > 1 {
				header = "New"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": request.ID,
				"result": map[string]any{"tools": []any{map[string]any{
					"name": "echo", "inputSchema": map[string]any{
						"type":       "object",
						"properties": map[string]any{"value": map[string]any{"type": "string", "x-mcp-header": header}},
					},
				}}},
			})
		case "tools/call":
			callCount++
			if callCount == 1 {
				if r.Header.Get("Mcp-Param-Old") != "x" {
					t.Fatalf("old header = %q", r.Header.Get("Mcp-Param-Old"))
				}
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "error": map[string]any{"code": -32020, "message": "HeaderMismatch"}})
				return
			}
			if r.Header.Get("Mcp-Param-New") != "x" || r.Header.Get("Mcp-Param-Old") != "" {
				t.Fatalf("retry headers old=%q new=%q", r.Header.Get("Mcp-Param-Old"), r.Header.Get("Mcp-Param-New"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"content": []any{}}})
		default:
			t.Fatalf("unexpected method %q", request.Method)
		}
	}))
	defer server.Close()

	client := NewNativeClient()
	if err := client.Connect(context.Background(), Server{ID: "test", Enabled: true, Transport: "http", URL: server.URL}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Tools(context.Background(), "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Call(context.Background(), "test", "echo", map[string]any{"value": "x"}); err != nil {
		t.Fatal(err)
	}
	if listCount != 2 || callCount != 2 {
		t.Fatalf("list=%d call=%d", listCount, callCount)
	}
}

func TestParseHTTPRPCSelectsResponseFromMultipleSSEEvents(t *testing.T) {
	body := []byte("event: message\ndata: {\"jsonrpc\":\"2.0\",\"method\":\"notifications/progress\",\"params\":{}}\n\nevent: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":7,\"result\":{\"ok\":true}}\n\n")
	response, err := parseHTTPRPC(body, "text/event-stream")
	if err != nil {
		t.Fatal(err)
	}
	if rawID(response.ID) != "7" || len(response.Result) == 0 {
		t.Fatalf("response = %#v", response)
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
