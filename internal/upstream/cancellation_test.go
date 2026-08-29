package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestModernHTTPCallCancellationClosesUpstreamRequest(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request rpcRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch request.Method {
		case "server/discover":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": request.ID,
				"result": map[string]any{
					"supportedVersions": []string{ModernProtocol},
					"capabilities":      map[string]any{"tools": map[string]any{}},
					"ttlMs":             0,
					"cacheScope":        "public",
					"resultType":        "complete",
				},
			})
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": request.ID,
				"result": map[string]any{
					"tools": []any{map[string]any{
						"name":        "block",
						"inputSchema": map[string]any{"type": "object", "additionalProperties": false},
					}},
					"ttlMs": 0, "cacheScope": "public", "resultType": "complete",
				},
			})
		case "tools/call":
			close(started)
			<-r.Context().Done()
			close(canceled)
		default:
			t.Fatalf("unexpected method %q", request.Method)
		}
	}))
	defer server.Close()

	client := NewNativeClient()
	if err := client.Connect(context.Background(), Server{ID: "cancel", Enabled: true, Transport: "http", URL: server.URL, Auth: AuthConfig{Type: "none"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Tools(context.Background(), "cancel"); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := client.Call(ctx, "cancel", "block", map[string]any{})
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("upstream tools/call did not start")
	}
	cancel()

	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("upstream HTTP request context was not canceled")
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("client call error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("upstream client call did not return after cancellation")
	}
}
