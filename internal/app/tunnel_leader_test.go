package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/cluster"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

type fakeLeaderTunnel struct {
	mu         sync.Mutex
	config     tunnel.Config
	status     tunnel.Status
	startCount int
	stopCount  int
}

func newFakeLeaderTunnel(id string) *fakeLeaderTunnel {
	return &fakeLeaderTunnel{config: tunnel.Config{Enabled: true, ID: id, APIKey: "secret"}}
}

func (f *fakeLeaderTunnel) Config() tunnel.Config {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.config
}

func (f *fakeLeaderTunnel) Status() tunnel.Status {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.status
}

func (f *fakeLeaderTunnel) StartContext(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.startCount++
	f.status.Running = true
	f.status.Ready = true
	return nil
}

func (f *fakeLeaderTunnel) Stop() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.status.Running || f.status.Restarting {
		f.stopCount++
	}
	f.status.Running = false
	f.status.Ready = false
	f.status.Restarting = false
	return nil
}

func (f *fakeLeaderTunnel) Configure(cfg tunnel.Config) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.config = cfg
	return nil
}

func (f *fakeLeaderTunnel) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.startCount, f.stopCount
}

func testLeaderNode(t *testing.T, relay *cluster.MemoryRelay, id, catalog string) *cluster.Node {
	t.Helper()
	node := cluster.NewNode(relay, cluster.Advertisement{InstanceID: id, Name: id, CatalogHash: catalog}, nil)
	if err := node.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = node.Close() })
	return node
}

func testCoordinator(node *cluster.Node, runtime tunnelLeaderRuntime) *tunnelLeaderCoordinator {
	value := newTunnelLeaderCoordinator(node, runtime, nil)
	value.leaseTTL = 120 * time.Millisecond
	value.tick = 20 * time.Millisecond
	return value
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition did not become true")
}

func TestTunnelLeaderCoordinatorRunsExactlyOneTunnelAndHandsOver(t *testing.T) {
	relay := cluster.NewMemoryRelay()
	firstNode := testLeaderNode(t, relay, "inst_a", "cat_same")
	secondNode := testLeaderNode(t, relay, "inst_b", "cat_same")
	firstTunnel, secondTunnel := newFakeLeaderTunnel("tunnel_test"), newFakeLeaderTunnel("tunnel_test")
	first, second := testCoordinator(firstNode, firstTunnel), testCoordinator(secondNode, secondTunnel)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := first.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer first.Stop()
	if err := second.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer second.Stop()
	if starts, _ := firstTunnel.counts(); starts != 1 {
		t.Fatalf("first starts = %d", starts)
	}
	if starts, _ := secondTunnel.counts(); starts != 0 {
		t.Fatalf("second starts = %d", starts)
	}
	if err := firstNode.Close(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		firstStarts, firstStops := firstTunnel.counts()
		secondStarts, _ := secondTunnel.counts()
		return firstStarts == 1 && firstStops == 1 && secondStarts == 1
	})
	lease, ok := second.Lease()
	if !ok || lease.InstanceID != "inst_b" || lease.Epoch != 2 {
		t.Fatalf("handover lease = %#v ok=%v", lease, ok)
	}
}

func TestTunnelLeaderCoordinatorBlocksIncompatibleCatalog(t *testing.T) {
	relay := cluster.NewMemoryRelay()
	firstNode := testLeaderNode(t, relay, "inst_a", "cat_a")
	secondNode := testLeaderNode(t, relay, "inst_b", "cat_b")
	firstTunnel, secondTunnel := newFakeLeaderTunnel("tunnel_test"), newFakeLeaderTunnel("tunnel_test")
	first, second := testCoordinator(firstNode, firstTunnel), testCoordinator(secondNode, secondTunnel)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := first.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer first.Stop()
	if err := second.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer second.Stop()
	if starts, _ := firstTunnel.counts(); starts != 0 {
		t.Fatalf("first starts = %d", starts)
	}
	if starts, _ := secondTunnel.counts(); starts != 0 {
		t.Fatalf("second starts = %d", starts)
	}
	if _, ok := first.Lease(); ok {
		t.Fatal("incompatible cluster acquired leadership")
	}
}

func TestTunnelLeaderCoordinatorDemotesWhenCatalogChanges(t *testing.T) {
	relay := cluster.NewMemoryRelay()
	firstNode := testLeaderNode(t, relay, "inst_a", "cat_same")
	secondNode := testLeaderNode(t, relay, "inst_b", "cat_same")
	firstTunnel := newFakeLeaderTunnel("tunnel_test")
	first := testCoordinator(firstNode, firstTunnel)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := first.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer first.Stop()
	if err := secondNode.Update(ctx, cluster.Advertisement{InstanceID: "inst_b", Name: "inst_b", CatalogHash: "cat_other"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		starts, stops := firstTunnel.counts()
		_, leader := first.Lease()
		return starts == 1 && stops == 1 && !leader
	})
}

func TestTunnelLeaderCoordinatorConfigureReleasesOldTunnelLease(t *testing.T) {
	relay := cluster.NewMemoryRelay()
	node := testLeaderNode(t, relay, "inst_a", "cat_same")
	runtime := newFakeLeaderTunnel("tunnel_old")
	coordinator := testCoordinator(node, runtime)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := coordinator.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer coordinator.Stop()
	next := runtime.Config()
	next.ID = "tunnel_new"
	if err := coordinator.Configure(next); err != nil {
		t.Fatal(err)
	}
	lease, ok := coordinator.Lease()
	if !ok || lease.TunnelID != "tunnel_new" {
		t.Fatalf("new lease = %#v ok=%v", lease, ok)
	}
	if _, ok, err := node.Leadership(ctx, "tunnel_old"); err != nil || ok {
		t.Fatalf("old lease still active: ok=%v err=%v", ok, err)
	}
}
