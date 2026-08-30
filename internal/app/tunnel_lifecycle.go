package app

import (
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/activity"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

func (a *App) attachTunnelLifecycle() {
	if a == nil || a.Tunnel == nil {
		return
	}
	a.Tunnel.SetLifecycleObserver(func(event tunnel.LifecycleEvent) {
		fields := []any{}
		if event.ID != "" {
			fields = append(fields, "tunnel_id", event.ID)
		}
		if event.Message != "" && event.State == tunnel.LifecycleDegraded {
			fields = append(fields, "error", event.Message)
		}
		if a.Logger != nil {
			switch event.State {
			case tunnel.LifecycleReady, tunnel.LifecycleStopped:
				a.Logger.Success("TUNNEL", string(event.State), fields...)
			case tunnel.LifecycleDegraded:
				a.Logger.Warn("TUNNEL", string(event.State), fields...)
			default:
				a.Logger.Info("TUNNEL", string(event.State), fields...)
			}
		}
		if a.Activity != nil {
			message := strings.TrimSpace(event.Message)
			if event.ID != "" {
				if message != "" {
					message = "tunnel_id=" + event.ID + " " + message
				} else {
					message = "tunnel_id=" + event.ID
				}
			}
			a.Activity.Publish(activity.Event{Kind: "tunnel." + string(event.State), Source: "tunnel", Status: string(event.State), Message: message})
		}
	})
}
