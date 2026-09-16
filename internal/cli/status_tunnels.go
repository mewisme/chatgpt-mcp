package cli

import (
	"fmt"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/runtimecontrol"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

type statusTunnelView struct {
	ID         string
	Name       string
	Label      string
	State      string
	LastError  string
	Enabled    bool
	Configured bool
	Running    bool
	Ready      bool
	Restarting bool
}

func newTunnelRuntimeStatus(status tunnel.Status, configured bool) runtimecontrol.TunnelRuntimeStatus {
	return runtimecontrol.TunnelRuntimeStatus{
		ID:         status.ID,
		Name:       tunnelStatusName(status.Metadata),
		Enabled:    status.Enabled,
		Configured: configured,
		Running:    status.Running,
		Ready:      status.Ready,
		Restarting: status.Restarting,
		LastError:  status.LastError,
	}
}

func tunnelStatusName(metadata *tunnel.Metadata) string {
	if metadata == nil {
		return ""
	}
	return strings.TrimSpace(metadata.Name)
}

func loadCachedTunnelNames(collection tunnel.CollectionConfig) map[string]string {
	names := make(map[string]string, len(collection.Instances))
	for _, instance := range collection.Instances {
		if strings.TrimSpace(instance.ID) == "" {
			continue
		}
		metadata, err := config.LoadTunnelMetadata(instance.ID)
		if err != nil {
			continue
		}
		if name := strings.TrimSpace(metadata.Name); name != "" {
			names[instance.ID] = name
		}
	}
	return names
}

func runtimeStatusTunnelItems(status runtimeStatusResult) []runtimecontrol.TunnelRuntimeStatus {
	if len(status.Tunnels) > 0 {
		return status.Tunnels
	}
	if status.TunnelID != "" || status.TunnelEnabled || status.TunnelConfigured || status.TunnelRunning || status.TunnelReady || status.TunnelRestarting || status.TunnelLastError != "" {
		return []runtimecontrol.TunnelRuntimeStatus{{
			ID:         status.TunnelID,
			Enabled:    status.TunnelEnabled,
			Configured: status.TunnelConfigured,
			Running:    status.TunnelRunning,
			Ready:      status.TunnelReady,
			Restarting: status.TunnelRestarting,
			LastError:  status.TunnelLastError,
		}}
	}
	return nil
}

func buildStatusTunnelViewsFromItems(items []runtimecontrol.TunnelRuntimeStatus, running bool, names map[string]string) []statusTunnelView {
	counts := make(map[string]int, len(items))
	views := make([]statusTunnelView, 0, len(items))
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" && names != nil {
			name = strings.TrimSpace(names[item.ID])
		}
		if name != "" {
			counts[name]++
		}
		views = append(views, statusTunnelView{
			ID: item.ID, Name: name, State: tunnelRuntimeState(item, running), LastError: item.LastError,
			Enabled: item.Enabled, Configured: item.Configured, Running: item.Running, Ready: item.Ready, Restarting: item.Restarting,
		})
	}
	for i := range views {
		views[i].Label = statusTunnelLabel(views[i].ID, views[i].Name, counts[views[i].Name] > 1)
	}
	return views
}

func statusTunnelLabel(id, name string, duplicate bool) string {
	if duplicate && strings.TrimSpace(name) != "" {
		if short := tunnelShortID(id); short != "" {
			return strings.TrimSpace(name) + " · " + short
		}
	}
	return tunnel.DisplayLabel(id, name)
}

func tunnelShortID(id string) string {
	id = strings.TrimPrefix(strings.TrimSpace(id), "tunnel_")
	if len(id) <= 8 {
		return id
	}
	return id[len(id)-8:]
}

func statusTunnelSummaryLine(running bool, summary runtimecontrol.TunnelSummary, views []statusTunnelView) string {
	total := summary.Total
	if total == 0 {
		total = len(views)
	}
	if total == 0 {
		return "none configured"
	}
	if !running {
		return fmt.Sprintf("runtime offline · %d configured", total)
	}
	parts := []string{fmt.Sprintf("%d/%d ready", summary.Ready, total)}
	extras := map[string]int{}
	for _, view := range views {
		if view.State == "connected" {
			continue
		}
		extras[view.State]++
	}
	for _, state := range []string{"degraded", "reconnecting", "connecting", "starting", "disabled", "not configured", "offline"} {
		if n := extras[state]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, state))
		}
	}
	return strings.Join(parts, " · ")
}
