package app

import (
	"context"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/cluster"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

func (a *App) ClusterStatus(ctx context.Context) cluster.RuntimeStatus {
	status := cluster.RuntimeStatus{CatalogCompatible: true}
	if a == nil || a.Config == nil {
		status.LastError = "runtime configuration is unavailable"
		return status
	}
	cfg := a.Config.Snapshot()
	status.Enabled = cfg.Cluster.Enabled
	status.RelayURL = strings.TrimSpace(cfg.Cluster.RelayURL)
	if a.Tools == nil {
		status.LastError = "tool runtime is unavailable"
		return status
	}
	if advertisement, err := a.Tools.ClusterAdvertisement(); err == nil {
		status.InstanceID = advertisement.InstanceID
		status.Name = advertisement.Name
		status.CatalogHash = advertisement.CatalogHash
		status.WorkspaceCount = len(advertisement.Workspaces)
	} else {
		status.LastError = err.Error()
	}
	if !cfg.Cluster.Enabled {
		status.TunnelRole = "standalone"
		return status
	}
	node := a.Tools.ClusterNode()
	if node == nil {
		status.TunnelRole = "standby"
		if status.LastError == "" {
			status.LastError = "cluster relay is not connected"
		}
		return status
	}
	if ctx == nil {
		ctx = context.Background()
	}
	snapshot, err := node.Snapshot(ctx)
	if err != nil {
		status.TunnelRole = "standby"
		status.LastError = err.Error()
		return status
	}
	status.Connected = true
	status.Members = snapshot.Members
	status.Workspaces = snapshot.Workspaces
	status.MemberCount = len(snapshot.Members)
	for _, member := range snapshot.Members {
		if member.Online {
			status.OnlineMemberCount++
		}
	}
	status.WorkspaceCount = len(snapshot.Workspaces)
	status.CatalogHash = snapshot.CatalogHash
	status.CatalogCompatible = snapshot.CatalogCompatible
	status.CatalogError = snapshot.CatalogError
	if !cfg.Tunnel.Enabled {
		status.TunnelRole = "disabled"
		return status
	}
	if !tunnel.Configured(cfg.Tunnel) {
		status.TunnelRole = "not_configured"
		return status
	}
	status.TunnelRole = "standby"
	for _, lease := range snapshot.Leaders {
		if lease.TunnelID != cfg.Tunnel.ID {
			continue
		}
		status.LeaderInstanceID = lease.InstanceID
		status.LeaderEpoch = lease.Epoch
		status.LeaseExpiresAt = lease.ExpiresAt
		if lease.InstanceID == status.InstanceID {
			status.TunnelRole = "leader"
		}
		break
	}
	return status
}
