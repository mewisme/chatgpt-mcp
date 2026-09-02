package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testRelayURL(server *httptest.Server) string {
	return "ws" + strings.TrimPrefix(server.URL, "http")
}

func TestWebSocketRelayRejectsWrongToken(t *testing.T) {
	server := httptest.NewServer(NewRelayServer("secret"))
	defer server.Close()
	transport := NewWebSocketTransport(testRelayURL(server), "wrong")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := transport.Connect(ctx, Advertisement{InstanceID: "inst_a", Name: "a"}); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("connect error = %v", err)
	}
}

func TestWebSocketRelayRoutesRPCAndDiscovery(t *testing.T) {
	relay := NewRelayServer("secret")
	server := httptest.NewServer(relay)
	defer server.Close()
	url := testRelayURL(server)
	first := NewNode(NewWebSocketTransport(url, "secret"), Advertisement{InstanceID: "inst_a", Name: "a", CatalogHash: "cat_same", Workspaces: []string{"ws_a"}}, nil)
	second := NewNode(NewWebSocketTransport(url, "secret"), Advertisement{InstanceID: "inst_b", Name: "b", CatalogHash: "cat_same", Workspaces: []string{"ws_b"}}, func(_ context.Context, method string, payload json.RawMessage) (json.RawMessage, error) {
		if method != "echo" {
			return nil, errors.New("unexpected method")
		}
		return payload, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := first.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if err := second.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	snapshot, err := first.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Members) != 2 || len(snapshot.Workspaces) != 2 || !snapshot.CatalogCompatible || snapshot.CatalogHash != "cat_same" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	payload := json.RawMessage(`{"message":"hello"}`)
	result, err := first.Call(ctx, "inst_b", "echo", payload)
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != string(payload) {
		t.Fatalf("RPC result = %s", result)
	}
	owner, err := first.WorkspaceOwner(ctx, "ws_b")
	if err != nil || owner.InstanceID != "inst_b" || !owner.Online {
		t.Fatalf("owner = %#v err=%v", owner, err)
	}
}

func TestWebSocketRelayCoordinatesLeaderLeaseAndHandover(t *testing.T) {
	relay := NewRelayServer("secret")
	server := httptest.NewServer(relay)
	defer server.Close()
	url := testRelayURL(server)
	first := NewNode(NewWebSocketTransport(url, "secret"), Advertisement{InstanceID: "inst_a", Name: "a", CatalogHash: "cat_same"}, nil)
	second := NewNode(NewWebSocketTransport(url, "secret"), Advertisement{InstanceID: "inst_b", Name: "b", CatalogHash: "cat_same"}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := first.Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := second.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	lease, acquired, err := first.TryAcquireLeadership(ctx, "tunnel_test", time.Second)
	if err != nil || !acquired || lease.InstanceID != "inst_a" || lease.Epoch != 1 {
		t.Fatalf("first lease = %#v acquired=%v err=%v", lease, acquired, err)
	}
	current, acquired, err := second.TryAcquireLeadership(ctx, "tunnel_test", time.Second)
	if err != nil || acquired || current != lease {
		t.Fatalf("second acquire = %#v acquired=%v err=%v", current, acquired, err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		next, ok, acquireErr := second.TryAcquireLeadership(ctx, "tunnel_test", time.Second)
		if acquireErr == nil && ok {
			if next.InstanceID != "inst_b" || next.Epoch != 2 {
				t.Fatalf("handover lease = %#v", next)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("handover failed: lease=%#v acquired=%v err=%v", next, ok, acquireErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
