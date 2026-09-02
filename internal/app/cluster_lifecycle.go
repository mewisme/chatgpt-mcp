package app

import (
	"context"
	"errors"

	"go.mewis.me/chatgpt-mcp/internal/cluster"
	"go.mewis.me/chatgpt-mcp/internal/config"
)

func (a *App) newClusterTransport(cfg config.ClusterConfig) cluster.Transport {
	if a.clusterTransportFactory != nil {
		return a.clusterTransportFactory(cfg)
	}
	return cluster.NewWebSocketTransport(cfg.RelayURL, cfg.RelayToken)
}

func (a *App) startCluster(ctx context.Context, cfg config.ClusterConfig) error {
	if !cfg.Enabled {
		if a.Tools != nil {
			a.Tools.SetClusterNode(nil)
		}
		return nil
	}
	if a.Tools == nil {
		return errors.New("tool runtime is unavailable")
	}
	advertisement, err := a.Tools.ClusterAdvertisement()
	if err != nil {
		return err
	}
	node := cluster.NewNode(a.newClusterTransport(cfg), advertisement, a.Tools.ClusterRPCHandler)
	a.Tools.SetClusterNode(node)
	if err := node.Start(ctx); err != nil {
		a.Tools.SetClusterNode(nil)
		_ = node.Close()
		return err
	}
	if a.Logger != nil {
		a.Logger.Ready("CLUSTER", "cluster.connected", "Cluster relay connected")
	}
	return nil
}

func (a *App) stopCluster() error {
	if a == nil || a.Tools == nil {
		return nil
	}
	node := a.Tools.ClusterNode()
	a.Tools.SetClusterNode(nil)
	if node == nil {
		return nil
	}
	if a.Logger != nil {
		a.Logger.Verbose("CLUSTER", "cluster.stopping", "Stopping cluster relay")
	}
	err := node.Close()
	if err == nil && a.Logger != nil {
		a.Logger.Verbose("CLUSTER", "cluster.stopped", "Cluster relay stopped")
	}
	return err
}

func clusterConfigEqual(left, right config.ClusterConfig) bool {
	return left.Enabled == right.Enabled && left.RelayURL == right.RelayURL && left.RelayToken == right.RelayToken
}
