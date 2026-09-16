package tunnel

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

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

func TestStatusRedactsAdminKeyAndExposesVerifiedScope(t *testing.T) {
	client := NewConfigured(Config{AdminKey: "sk-admin", AdminWorkspaceID: "ws_admin"}, nil)
	status := client.Status()
	if !status.AdminKeyConfigured || status.AdminScope == nil || status.AdminScope.WorkspaceID != "ws_admin" {
		t.Fatalf("status = %#v", status)
	}
}

func TestSeedMetadataPopulatesStatusWithoutFetch(t *testing.T) {
	client := NewConfigured(Config{ID: "tunnel_test", APIKey: "runtime-key"}, nil)
	client.metadataFetch = func(context.Context, Config) (Metadata, error) {
		t.Fatal("seeded metadata should not fetch")
		return Metadata{}, nil
	}
	metadata := Metadata{ID: "tunnel_test", Name: "Persisted tunnel", FetchedAt: time.Now().UTC().Add(-time.Hour)}
	if err := client.SeedMetadata(metadata); err != nil {
		t.Fatal(err)
	}
	status := client.Status()
	if status.Metadata == nil || status.Metadata.Name != "Persisted tunnel" {
		t.Fatalf("status = %#v", status)
	}
}

func TestManagedAdminRequiresPlugin(t *testing.T) {
	SetAdminBackend(nil)
	SetAdminResolver(nil)
	if _, err := FetchMetadata(context.Background(), Config{ID: "tunnel_test", APIKey: "sk-runtime"}); !errors.Is(err, ErrPluginMissing) {
		t.Fatalf("err=%v", err)
	}
	if _, err := CreateManaged(context.Background(), Config{AdminKey: "sk-admin"}, CreateRequest{Name: "n", Description: "d", WorkspaceIDs: []string{"ws"}}); !errors.Is(err, ErrPluginMissing) {
		t.Fatalf("err=%v", err)
	}
}
