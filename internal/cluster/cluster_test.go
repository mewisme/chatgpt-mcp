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

func TestLeaderLeaseHasSingleOwnerAndFencingEpoch(t *testing.T) {
	relay := NewMemoryRelay()
	first := NewNode(relay, Advertisement{InstanceID: "inst_a", Name: "a"}, nil)
	second := NewNode(relay, Advertisement{InstanceID: "inst_b", Name: "b"}, nil)
	if err := first.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if err := second.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	lease, acquired, err := first.TryAcquireLeadership(context.Background(), "tunnel_test", 10*time.Second)
	if err != nil || !acquired || lease.InstanceID != "inst_a" || lease.Epoch != 1 {
		t.Fatalf("first lease = %#v acquired=%v err=%v", lease, acquired, err)
	}
	current, acquired, err := second.TryAcquireLeadership(context.Background(), "tunnel_test", 10*time.Second)
	if err != nil || acquired || current != lease {
		t.Fatalf("second acquire = %#v acquired=%v err=%v", current, acquired, err)
	}
	renewed, err := first.RenewLeadership(context.Background(), lease, 20*time.Second)
	if err != nil || renewed.Epoch != lease.Epoch || !renewed.ExpiresAt.After(lease.ExpiresAt) {
		t.Fatalf("renewed lease = %#v err=%v", renewed, err)
	}
	if err := first.ReleaseLeadership(context.Background(), lease); err != nil {
		t.Fatal(err)
	}
	next, acquired, err := second.TryAcquireLeadership(context.Background(), "tunnel_test", 10*time.Second)
	if err != nil || !acquired || next.InstanceID != "inst_b" || next.Epoch != 2 {
		t.Fatalf("next lease = %#v acquired=%v err=%v", next, acquired, err)
	}
	if _, err := first.RenewLeadership(context.Background(), lease, 10*time.Second); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale renew error = %v", err)
	}
	if err := first.ReleaseLeadership(context.Background(), lease); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale release error = %v", err)
	}
}

func TestLeaderLeaseExpiresAndHandsOver(t *testing.T) {
	now := time.Date(2026, time.September, 2, 12, 0, 0, 0, time.UTC)
	relay := NewMemoryRelay()
	relay.now = func() time.Time { return now }
	first := NewNode(relay, Advertisement{InstanceID: "inst_a", Name: "a"}, nil)
	second := NewNode(relay, Advertisement{InstanceID: "inst_b", Name: "b"}, nil)
	if err := first.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if err := second.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	lease, acquired, err := first.TryAcquireLeadership(context.Background(), "tunnel_test", 5*time.Second)
	if err != nil || !acquired {
		t.Fatalf("first lease = %#v acquired=%v err=%v", lease, acquired, err)
	}
	now = now.Add(6 * time.Second)
	if _, ok, err := second.Leadership(context.Background(), "tunnel_test"); err != nil || ok {
		t.Fatalf("expired leadership still visible: ok=%v err=%v", ok, err)
	}
	next, acquired, err := second.TryAcquireLeadership(context.Background(), "tunnel_test", 5*time.Second)
	if err != nil || !acquired || next.Epoch != lease.Epoch+1 {
		t.Fatalf("handover lease = %#v acquired=%v err=%v", next, acquired, err)
	}
}

func TestClosingLeaderMakesLeaseImmediatelyAvailable(t *testing.T) {
	relay := NewMemoryRelay()
	first := NewNode(relay, Advertisement{InstanceID: "inst_a", Name: "a"}, nil)
	second := NewNode(relay, Advertisement{InstanceID: "inst_b", Name: "b"}, nil)
	if err := first.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := second.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	lease, acquired, err := first.TryAcquireLeadership(context.Background(), "tunnel_test", time.Minute)
	if err != nil || !acquired {
		t.Fatalf("first lease = %#v acquired=%v err=%v", lease, acquired, err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	next, acquired, err := second.TryAcquireLeadership(context.Background(), "tunnel_test", time.Minute)
	if err != nil || !acquired || next.Epoch != lease.Epoch+1 {
		t.Fatalf("close handover = %#v acquired=%v err=%v", next, acquired, err)
	}
}
