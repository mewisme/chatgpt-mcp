package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestMemoryRelayTracksMembersAndWorkspaceOwners(t *testing.T) {
	relay := NewMemoryRelay()
	first, err := relay.Connect(context.Background(), Advertisement{InstanceID: "inst_a", Name: "a", Workspaces: []string{"ws_a"}})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := relay.Connect(context.Background(), Advertisement{InstanceID: "inst_b", Name: "b", Workspaces: []string{"ws_b"}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := second.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Members) != 2 || len(snapshot.Workspaces) != 2 || !snapshot.Members[0].Online || !snapshot.Members[1].Online {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	snapshot, err = second.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Members[0].Online || snapshot.Workspaces[0].Online {
		t.Fatalf("offline owner was not retained: %#v", snapshot)
	}
}

func TestMemoryRelayRejectsDuplicateOwnershipAndInstanceConnections(t *testing.T) {
	relay := NewMemoryRelay()
	first, err := relay.Connect(context.Background(), Advertisement{InstanceID: "inst_a", Name: "a", Workspaces: []string{"ws_a"}})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if _, err := relay.Connect(context.Background(), Advertisement{InstanceID: "inst_a", Name: "duplicate"}); err == nil {
		t.Fatal("expected duplicate instance connection to fail")
	}
	if _, err := relay.Connect(context.Background(), Advertisement{InstanceID: "inst_b", Name: "b", Workspaces: []string{"ws_a"}}); err == nil {
		t.Fatal("expected duplicate workspace ownership to fail")
	}
}

func TestNodeRPCBetweenInstances(t *testing.T) {
	relay := NewMemoryRelay()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	nodeA := NewNode(relay, Advertisement{InstanceID: "inst_a", Name: "a", Workspaces: []string{"ws_a"}}, nil)
	nodeB := NewNode(relay, Advertisement{InstanceID: "inst_b", Name: "b", Workspaces: []string{"ws_b"}}, func(_ context.Context, method string, payload json.RawMessage) (json.RawMessage, error) {
		if method != "echo" {
			return nil, errors.New("unexpected method")
		}
		return payload, nil
	})
	if err := nodeA.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer nodeA.Close()
	if err := nodeB.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer nodeB.Close()
	payload := json.RawMessage(`{"value":"ok"}`)
	result, err := nodeA.Call(ctx, "inst_b", "echo", payload)
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != string(payload) {
		t.Fatalf("result = %s", result)
	}
	owner, err := nodeA.WorkspaceOwner(ctx, "ws_b")
	if err != nil || owner.InstanceID != "inst_b" || !owner.Online {
		t.Fatalf("owner = %#v err=%v", owner, err)
	}
}

func TestNodeReportsOfflineWorkspaceOwnerAndReconnect(t *testing.T) {
	relay := NewMemoryRelay()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	nodeA := NewNode(relay, Advertisement{InstanceID: "inst_a", Name: "a"}, nil)
	nodeB := NewNode(relay, Advertisement{InstanceID: "inst_b", Name: "b", Workspaces: []string{"ws_b"}}, nil)
	if err := nodeA.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer nodeA.Close()
	if err := nodeB.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := nodeB.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := nodeA.WorkspaceOwner(ctx, "ws_b"); !errors.Is(err, ErrOwnerOffline) {
		t.Fatalf("offline owner error = %v", err)
	}
	nodeB = NewNode(relay, Advertisement{InstanceID: "inst_b", Name: "b", Workspaces: []string{"ws_b"}}, nil)
	if err := nodeB.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer nodeB.Close()
	owner, err := nodeA.WorkspaceOwner(ctx, "ws_b")
	if err != nil || owner.InstanceID != "inst_b" || !owner.Online {
		t.Fatalf("reconnected owner = %#v err=%v", owner, err)
	}
}

func TestAdvertisementCanMoveLocalWorkspaceSet(t *testing.T) {
	relay := NewMemoryRelay()
	ctx := context.Background()
	node := NewNode(relay, Advertisement{InstanceID: "inst_a", Name: "a", Workspaces: []string{"ws_old"}}, nil)
	if err := node.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer node.Close()
	if err := node.Update(ctx, Advertisement{InstanceID: "inst_a", Name: "a", Workspaces: []string{"ws_new"}}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := node.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Workspaces) != 1 || snapshot.Workspaces[0].WorkspaceID != "ws_new" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}

func TestSnapshotReportsCatalogCompatibility(t *testing.T) {
	relay := NewMemoryRelay()
	first, err := relay.Connect(context.Background(), Advertisement{InstanceID: "inst_a", Name: "a", CatalogHash: "cat_same"})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := relay.Connect(context.Background(), Advertisement{InstanceID: "inst_b", Name: "b", CatalogHash: "cat_same"})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	snapshot, err := first.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.CatalogCompatible || snapshot.CatalogHash != "cat_same" || snapshot.CatalogError != "" {
		t.Fatalf("compatible snapshot = %#v", snapshot)
	}
	if err := second.Advertise(context.Background(), Advertisement{InstanceID: "inst_b", Name: "b", CatalogHash: "cat_other"}); err != nil {
		t.Fatal(err)
	}
	snapshot, err = first.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CatalogCompatible || snapshot.CatalogError == "" {
		t.Fatalf("mismatched snapshot = %#v", snapshot)
	}
}

func TestSnapshotTreatsLegacyCatalogAsDegradedWithMultipleMembers(t *testing.T) {
	relay := NewMemoryRelay()
	first, err := relay.Connect(context.Background(), Advertisement{InstanceID: "inst_a", Name: "a", CatalogHash: "cat_current"})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := relay.Connect(context.Background(), Advertisement{InstanceID: "inst_b", Name: "b"})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	snapshot, err := first.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.CatalogCompatible || snapshot.CatalogError == "" {
		t.Fatalf("legacy snapshot = %#v", snapshot)
	}
}

func TestNodeAdvertisementProviderOverridesStaleInitialAdvertisement(t *testing.T) {
	relay := NewMemoryRelay()
	node := NewNode(relay, Advertisement{InstanceID: "inst_a", Name: "a", CatalogHash: "cat_old"}, nil)
	node.SetAdvertisementProvider(func() (Advertisement, error) {
		return Advertisement{InstanceID: "inst_a", Name: "a", CatalogHash: "cat_new", Workspaces: []string{"ws_new"}}, nil
	})
	if err := node.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer node.Close()
	snapshot, err := node.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Members) != 1 || snapshot.Members[0].CatalogHash != "cat_new" || len(snapshot.Workspaces) != 1 || snapshot.Workspaces[0].WorkspaceID != "ws_new" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
}
