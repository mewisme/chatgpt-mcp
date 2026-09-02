package app

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/cluster"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

const (
	defaultLeaderLeaseTTL = 15 * time.Second
	defaultLeaderTick     = 4 * time.Second
)

type tunnelLeaderRuntime interface {
	Config() tunnel.Config
	Status() tunnel.Status
	StartContext(context.Context) error
	Stop() error
	Configure(tunnel.Config) error
}

type tunnelLeaderCoordinator struct {
	node     *cluster.Node
	tunnel   tunnelLeaderRuntime
	log      *logger.Logger
	leaseTTL time.Duration
	tick     time.Duration
	opMu     sync.Mutex
	mu       sync.RWMutex
	lease    *cluster.LeaderLease
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
}

func newTunnelLeaderCoordinator(node *cluster.Node, runtime tunnelLeaderRuntime, log *logger.Logger) *tunnelLeaderCoordinator {
	return &tunnelLeaderCoordinator{node: node, tunnel: runtime, log: log, leaseTTL: defaultLeaderLeaseTTL, tick: defaultLeaderTick}
}

func (c *tunnelLeaderCoordinator) Start(ctx context.Context) error {
	if c == nil || c.node == nil || c.tunnel == nil {
		return errors.New("cluster tunnel coordinator is unavailable")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	c.mu.Lock()
	if c.cancel != nil {
		c.mu.Unlock()
		return errors.New("cluster tunnel coordinator is already running")
	}
	runCtx, cancel := context.WithCancel(ctx)
	c.ctx, c.cancel, c.done = runCtx, cancel, make(chan struct{})
	done := c.done
	c.mu.Unlock()
	if err := c.reconcile(runCtx); err != nil {
		cancel()
		c.mu.Lock()
		c.ctx, c.cancel, c.done = nil, nil, nil
		c.mu.Unlock()
		return err
	}
	go c.run(runCtx, done)
	return nil
}

func (c *tunnelLeaderCoordinator) Stop() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	cancel, done := c.cancel, c.done
	c.ctx, c.cancel, c.done = nil, nil, nil
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.demoteLocked(context.Background(), true)
}

func (c *tunnelLeaderCoordinator) Configure(cfg tunnel.Config) error {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if err := c.demoteLocked(context.Background(), true); err != nil {
		return err
	}
	if err := c.tunnel.Configure(cfg); err != nil {
		return err
	}
	c.mu.RLock()
	ctx := c.ctx
	c.mu.RUnlock()
	if ctx == nil {
		return nil
	}
	return c.reconcileLocked(ctx)
}

func (c *tunnelLeaderCoordinator) Lease() (cluster.LeaderLease, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.lease == nil {
		return cluster.LeaderLease{}, false
	}
	return *c.lease, true
}

func (c *tunnelLeaderCoordinator) run(ctx context.Context, done chan struct{}) {
	defer close(done)
	tick := c.tick
	if tick <= 0 {
		tick = defaultLeaderTick
	}
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.reconcile(ctx); err != nil && c.log != nil {
				c.log.Failure("CLUSTER", "cluster.tunnel.reconcile.failed", "Tunnel leadership reconciliation failed", err)
			}
		}
	}
}

func (c *tunnelLeaderCoordinator) reconcile(ctx context.Context) error {
	c.opMu.Lock()
	defer c.opMu.Unlock()
	return c.reconcileLocked(ctx)
}

func (c *tunnelLeaderCoordinator) reconcileLocked(ctx context.Context) error {
	cfg := c.tunnel.Config()
	if !cfg.Enabled || !tunnel.Configured(cfg) {
		return c.demoteLocked(ctx, true)
	}
	snapshot, err := c.node.Snapshot(ctx)
	if err != nil {
		_ = c.demoteLocked(context.Background(), false)
		return err
	}
	if !snapshot.CatalogCompatible {
		if err := c.demoteLocked(ctx, true); err != nil {
			return err
		}
		return nil
	}
	lease, leader := c.Lease()
	if leader {
		renewed, err := c.node.RenewLeadership(ctx, lease, c.leaseTTL)
		if err != nil {
			_ = c.demoteLocked(context.Background(), false)
			return err
		}
		c.setLease(renewed)
		return c.ensureTunnelRunning(ctx)
	}
	lease, acquired, err := c.node.TryAcquireLeadership(ctx, cfg.ID, c.leaseTTL)
	if err != nil {
		_ = c.demoteLocked(context.Background(), false)
		return err
	}
	if !acquired {
		return c.demoteLocked(ctx, false)
	}
	c.setLease(lease)
	if err := c.ensureTunnelRunning(ctx); err != nil {
		_ = c.demoteLocked(context.Background(), true)
		return err
	}
	if c.log != nil {
		c.log.Ready("CLUSTER", "cluster.tunnel.leader", "Tunnel leadership acquired", logger.WithVerbose("instance_id", lease.InstanceID), logger.WithVerbose("tunnel_id", lease.TunnelID), logger.WithVerbose("epoch", lease.Epoch))
	}
	return nil
}

func (c *tunnelLeaderCoordinator) ensureTunnelRunning(ctx context.Context) error {
	status := c.tunnel.Status()
	if status.Running || status.Restarting {
		return nil
	}
	return c.tunnel.StartContext(ctx)
}

func (c *tunnelLeaderCoordinator) demoteLocked(ctx context.Context, release bool) error {
	lease, leader := c.Lease()
	var result error
	status := c.tunnel.Status()
	if status.Running || status.Restarting {
		result = errors.Join(result, c.tunnel.Stop())
	}
	if leader && release {
		if err := c.node.ReleaseLeadership(ctx, lease); err != nil && !errors.Is(err, cluster.ErrLeaseLost) && !errors.Is(err, cluster.ErrClosed) {
			result = errors.Join(result, fmt.Errorf("release tunnel leadership: %w", err))
		}
	}
	if leader {
		c.setLease(cluster.LeaderLease{})
		if c.log != nil {
			c.log.Verbose("CLUSTER", "cluster.tunnel.standby", "Tunnel runtime is in standby", logger.WithVerbose("tunnel_id", lease.TunnelID), logger.WithVerbose("epoch", lease.Epoch))
		}
	}
	return result
}

func (c *tunnelLeaderCoordinator) setLease(lease cluster.LeaderLease) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if lease.TunnelID == "" {
		c.lease = nil
		return
	}
	value := lease
	c.lease = &value
}
