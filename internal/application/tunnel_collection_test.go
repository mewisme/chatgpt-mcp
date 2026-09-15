package application

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

func TestLocalTunnelCollectionOperationsKeepOtherInstances(t *testing.T) {
	setupTunnelApplicationRoot(t, tunnel.Config{})
	if _, err := AttachLocalTunnel(context.Background(), tunnel.InstanceConfig{ID: "a", APIKey: "key-a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := AttachLocalTunnel(context.Background(), tunnel.InstanceConfig{ID: "b", APIKey: "key-b"}); err != nil {
		t.Fatal(err)
	}
	if err := DetachLocalTunnel(context.Background(), "b"); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	instances := loaded.RuntimeTunnels().Instances
	if len(instances) != 1 || instances[0].ID != "a" || instances[0].APIKey != "key-a" {
		t.Fatalf("instances=%+v", instances)
	}
	items, err := LocalTunnels()
	if err != nil || len(items) != 1 || !items[0].RuntimeKeyConfigured {
		t.Fatalf("items=%+v err=%v", items, err)
	}
}

func TestDiscoveryDeduplicatesManagedTunnelsAndKeepsProfileProvenance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Header.Get("Authorization") {
		case "Bearer key-a":
			_, _ = w.Write([]byte(`{"tunnels":[{"id":"shared","name":"Shared"},{"id":"only-a","name":"A"}]}`))
		case "Bearer key-b":
			_, _ = w.Write([]byte(`{"tunnels":[{"id":"shared","name":"Shared"},{"id":"only-b","name":"B"}]}`))
		default:
			t.Fatalf("authorization=%q", r.Header.Get("Authorization"))
		}
	}))
	defer server.Close()
	setupTunnelApplicationRoot(t, tunnel.Config{})
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	instances := []tunnel.InstanceConfig{}
	admins := []tunnel.AdminConfig{{ID: "a", AdminKey: "key-a", OrganizationID: "org-a", ReadAccess: true, ControlPlaneBaseURL: server.URL}, {ID: "b", AdminKey: "key-b", OrganizationID: "org-b", ReadAccess: true, ControlPlaneBaseURL: server.URL}}
	cfg.Tunnel = tunnel.Config{Instances: &instances, Admins: &admins}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	items, err := DiscoverManagedTunnels(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || items[0].Metadata.ID != "shared" || len(items[0].AdminProfiles) != 2 || items[0].AdminProfiles[0] != "a" || items[0].AdminProfiles[1] != "b" {
		t.Fatalf("items=%+v", items)
	}
	if _, err := GetManagedTunnelByProfile(context.Background(), "shared", ""); err == nil {
		t.Fatal("ambiguous management profile was accepted")
	}
}
