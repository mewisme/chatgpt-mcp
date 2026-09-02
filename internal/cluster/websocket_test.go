package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func testRelayURL(server *httptest.Server) string {
	return "ws" + strings.TrimPrefix(server.URL, "http")
}

type trackedTransport struct {
	inner   Transport
	mu      sync.Mutex
	session Session
}

func (t *trackedTransport) Connect(ctx context.Context, advertisement Advertisement) (Session, error) {
	session, err := t.inner.Connect(ctx, advertisement)
	if err != nil {
		return nil, err
	}
	t.mu.Lock()
	t.session = session
	t.mu.Unlock()
	return session, nil
}

func (t *trackedTransport) Drop() {
	t.mu.Lock()
	session := t.session
	t.mu.Unlock()
	if session != nil {
		_ = session.Close()
	}
}

type relaySwitch struct {
	mu      sync.RWMutex
	handler http.Handler
}

func newRelaySwitch(token string) *relaySwitch {
	return &relaySwitch{handler: NewRelayServer(token)}
}

func (s *relaySwitch) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	handler := s.handler
	s.mu.RUnlock()
	handler.ServeHTTP(w, r)
}

func (s *relaySwitch) Restart(token string) {
	s.mu.Lock()
	s.handler = NewRelayServer(token)
	s.mu.Unlock()
}

func waitNodeConnected(t *testing.T, node *Node, connected bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if node.Connected() == connected {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("node connected = %v, want %v, last error = %v", node.Connected(), connected, node.LastError())
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

func TestNodeReconnectsAfterWebSocketRelayRestartAndReadvertises(t *testing.T) {
	switcher := newRelaySwitch("secret")
	server := httptest.NewServer(switcher)
	defer server.Close()
	transport := &trackedTransport{inner: NewWebSocketTransport(testRelayURL(server), "secret")}
	var advertisementMu sync.RWMutex
	advertisement := Advertisement{InstanceID: "inst_a", Name: "a", CatalogHash: "cat_one", Workspaces: []string{"ws_one"}}
	node := NewNode(transport, advertisement, nil)
	node.SetAdvertisementProvider(func() (Advertisement, error) {
		advertisementMu.RLock()
		defer advertisementMu.RUnlock()
		value := advertisement
		value.Workspaces = append([]string(nil), value.Workspaces...)
		return value, nil
	})
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if err := node.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer node.Close()
	advertisementMu.Lock()
	advertisement.CatalogHash = "cat_two"
	advertisement.Workspaces = []string{"ws_two"}
	advertisementMu.Unlock()
	switcher.Restart("secret")
	transport.Drop()
	waitNodeConnected(t, node, false)
	waitNodeConnected(t, node, true)
	snapshot, err := node.Snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Members) != 1 || snapshot.Members[0].CatalogHash != "cat_two" || len(snapshot.Workspaces) != 1 || snapshot.Workspaces[0].WorkspaceID != "ws_two" || !snapshot.Workspaces[0].Online {
		t.Fatalf("reconnected snapshot = %#v", snapshot)
	}
}

func TestNodeFailsPendingRPCWhenWebSocketSessionDrops(t *testing.T) {
	server := httptest.NewServer(NewRelayServer("secret"))
	defer server.Close()
	url := testRelayURL(server)
	transport := &trackedTransport{inner: NewWebSocketTransport(url, "secret")}
	started := make(chan struct{})
	release := make(chan struct{})
	first := NewNode(transport, Advertisement{InstanceID: "inst_a", Name: "a", CatalogHash: "cat_same"}, nil)
	second := NewNode(NewWebSocketTransport(url, "secret"), Advertisement{InstanceID: "inst_b", Name: "b", CatalogHash: "cat_same"}, func(_ context.Context, method string, payload json.RawMessage) (json.RawMessage, error) {
		close(started)
		<-release
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
	result := make(chan error, 1)
	go func() {
		_, err := first.Call(ctx, "inst_b", "echo", json.RawMessage(`{"value":"pending"}`))
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("remote RPC handler did not start")
	}
	transport.Drop()
	select {
	case err := <-result:
		if err == nil || !strings.Contains(err.Error(), ErrClosed.Error()) {
			t.Fatalf("pending RPC error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("pending RPC did not fail after session drop")
	}
	close(release)
}
