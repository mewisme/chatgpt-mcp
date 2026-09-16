package tunnel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tracepkg "go.mewis.me/chatgpt-mcp/internal/trace"
)

func TestGenerateRuntimeKeyTraceIncludesHTTPFactsWithoutGeneratedKey(t *testing.T) {
	const adminKey = "admin-runtime-key-trace-secret"
	const generatedKey = "generated-runtime-key-trace-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+adminKey {
			t.Fatalf("authorization=%q", r.Header.Get("Authorization"))
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/organization/projects":
			writeRuntimeKeyJSON(t, w, map[string]any{"data": []map[string]any{{"id": "proj_default", "name": "Default project", "status": "active"}}})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/organization/projects/proj_default/service_accounts":
			writeRuntimeKeyJSON(t, w, map[string]any{"data": []any{}})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/organization/projects/proj_default/service_accounts":
			writeRuntimeKeyJSON(t, w, map[string]any{"id": "svc_runtime", "name": runtimeServiceAccountName})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/organization/projects/proj_default/service_accounts/svc_runtime/api_keys":
			writeRuntimeKeyJSON(t, w, map[string]any{"id": "key_runtime", "value": generatedKey})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	events := []tracepkg.Event{}
	ctx := tracepkg.WithObserver(context.Background(), func(event tracepkg.Event) { events = append(events, event) })
	result, err := GenerateRuntimeKey(ctx, Config{AdminKey: adminKey, ControlPlaneBaseURL: server.URL}, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Value != generatedKey || result.ProjectID != "proj_default" || result.KeyID != "key_runtime" {
		t.Fatalf("result=%#v", result)
	}
	for _, name := range []string{"tunnel.runtime-key.generate.started", "tunnel.admin.projects.list.completed", "tunnel.runtime-key.project-selected", "tunnel.runtime-key.service-account.completed", "tunnel.admin-platform.request.completed", "http.request.completed", "tunnel.runtime-key.generate.completed"} {
		if !tunnelTraceContains(events, name) {
			t.Fatalf("missing trace event %s: %#v", name, events)
		}
	}
	if !tunnelTraceFieldEquals(events, "tunnel.runtime-key.generate.completed", "generated", true) || !tunnelTraceFieldEquals(events, "tunnel.runtime-key.generate.completed", "project_id", "proj_default") {
		t.Fatalf("runtime-key completion trace=%#v", events)
	}
	if !tunnelTraceHasField(events, "http.request.completed", "status") || !tunnelTraceHasField(events, "http.request.completed", "bytes_read") || !tunnelTraceHasField(events, "http.request.completed", "duration_ms") {
		t.Fatalf("HTTP trace is missing response facts: %#v", events)
	}
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, secret := range []string{adminKey, generatedKey} {
		if strings.Contains(text, secret) {
			t.Fatalf("runtime-key trace leaked authorization material %q: %s", secret, text)
		}
	}
}

func tunnelTraceContains(events []tracepkg.Event, name string) bool {
	for _, event := range events {
		if event.Name == name {
			return true
		}
	}
	return false
}

func tunnelTraceFieldEquals(events []tracepkg.Event, name, key string, want any) bool {
	for _, event := range events {
		if event.Name != name {
			continue
		}
		for _, field := range event.Fields {
			if field.Key == key && field.Value == want {
				return true
			}
		}
	}
	return false
}

func tunnelTraceHasField(events []tracepkg.Event, name, key string) bool {
	for _, event := range events {
		if event.Name != name {
			continue
		}
		for _, field := range event.Fields {
			if field.Key == key {
				return true
			}
		}
	}
	return false
}
