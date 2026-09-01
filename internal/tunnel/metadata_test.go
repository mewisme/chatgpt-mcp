package tunnel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestFetchMetadataWithRuntimeKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/tunnels/tunnel_test" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-runtime" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("x-request-id", "req_test")
		_, _ = w.Write([]byte(`{"id":"tunnel_test","name":"Mew WSL","description":"Local development tunnel","creator":"user_test","workspace_ids":["ws_openai"],"organization_ids":["org_test"],"tenant_ids":["tenant_test"]}`))
	}))
	defer server.Close()

	metadata, err := FetchMetadata(context.Background(), Config{ID: "tunnel_test", APIKey: "sk-runtime", ControlPlaneBaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if metadata.ID != "tunnel_test" || metadata.Name != "Mew WSL" || metadata.Description != "Local development tunnel" || metadata.Creator != "user_test" || metadata.RequestID != "req_test" {
		t.Fatalf("metadata = %#v", metadata)
	}
	if len(metadata.WorkspaceIDs) != 1 || metadata.WorkspaceIDs[0] != "ws_openai" || len(metadata.OrganizationIDs) != 1 || metadata.OrganizationIDs[0] != "org_test" {
		t.Fatalf("metadata scopes = %#v", metadata)
	}
	if metadata.FetchedAt.IsZero() {
		t.Fatal("fetched_at is zero")
	}
}

func TestCreateWithAdminAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/tunnels" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer sk-admin" {
			t.Fatalf("authorization = %q", r.Header.Get("Authorization"))
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body["name"] != "Mew Tunnel" || body["description"] != "Created from chatgpt-mcp" {
			t.Fatalf("body = %#v", body)
		}
		w.Header().Set("x-request-id", "req_create")
		_, _ = w.Write([]byte(`{"id":"tunnel_created","name":"Mew Tunnel","description":"Created from chatgpt-mcp","workspace_ids":["ws_openai"],"organization_ids":["org_test"]}`))
	}))
	defer server.Close()

	metadata, err := Create(context.Background(), Config{ControlPlaneBaseURL: server.URL}, "sk-admin", CreateRequest{Name: "Mew Tunnel", Description: "Created from chatgpt-mcp", WorkspaceIDs: []string{"ws_openai"}, OrganizationIDs: []string{"org_test"}})
	if err != nil {
		t.Fatal(err)
	}
	if metadata.ID != "tunnel_created" || metadata.Name != "Mew Tunnel" || metadata.RequestID != "req_create" {
		t.Fatalf("metadata = %#v", metadata)
	}
}

func TestCreateRequiresAdminKeyAndScope(t *testing.T) {
	if _, err := Create(context.Background(), Config{}, "", CreateRequest{Name: "n", Description: "d", WorkspaceIDs: []string{"ws"}}); err == nil {
		t.Fatal("expected missing admin key error")
	}
	if _, err := Create(context.Background(), Config{}, "sk-admin", CreateRequest{Name: "n", Description: "d"}); err == nil {
		t.Fatal("expected missing scope error")
	}
}

func TestRefreshMetadataCachesSuccessfulLookup(t *testing.T) {
	client := NewConfigured(Config{ID: "tunnel_test", APIKey: "sk-runtime"}, nil)
	var calls atomic.Int32
	client.metadataFetch = func(context.Context, Config) (Metadata, error) {
		calls.Add(1)
		return Metadata{ID: "tunnel_test", Name: "Cached tunnel", FetchedAt: time.Now().UTC()}, nil
	}
	first, err := client.RefreshMetadata(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.RefreshMetadata(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 || first.Name != "Cached tunnel" || second.Name != "Cached tunnel" || client.Status().Metadata == nil {
		t.Fatalf("calls=%d first=%#v second=%#v status=%#v", calls.Load(), first, second, client.Status())
	}
}
