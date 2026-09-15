package app

import (
	"context"
	"errors"
	"slices"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

// reloadTestAfterCommit runs after Config.Update and before runtime apply. Tests only.
var reloadTestAfterCommit func()

func (a *App) ReloadConfig(next config.Config) error {
	if a == nil || a.Config == nil || a.Tools == nil {
		return errors.New("runtime is unavailable")
	}
	if err := config.Validate(next); err != nil {
		return err
	}
	previous := a.Config.Snapshot()
	httpChanged := previous.Server.Enabled != next.Server.Enabled
	featuresChanged := previous.Features != next.Features
	permissionsChanged := !slices.Equal(previous.Permissions.AllowDirs, next.Permissions.AllowDirs)
	shellExecutableChanged := previous.Shell.Executable != next.Shell.Executable
	shellPathChanged := !slices.Equal(previous.Shell.Path, next.Shell.Path)
	tunnelChanged := !tunnel.ConfigEqual(previous.Tunnel, next.Tunnel)
	tunnelRuntimeChanged := tunnelChanged && (!tunnel.RuntimeConfigEqual(previous.Tunnel, next.Tunnel) || !slices.Equal(previous.RuntimeTunnels().Instances, next.RuntimeTunnels().Instances))

	if _, err := a.Config.Update(func(config.Config) (config.Config, error) { return next, nil }); err != nil {
		return err
	}
	if reloadTestAfterCommit != nil {
		reloadTestAfterCommit()
	}
	if err := a.applyRuntimeConfig(next, httpChanged, featuresChanged, permissionsChanged, shellExecutableChanged, shellPathChanged, tunnelChanged, tunnelRuntimeChanged); err != nil {
		_, restoreErr := a.Config.Update(func(config.Config) (config.Config, error) { return previous, nil })
		return errors.Join(err, restoreErr, a.rollbackRuntimeConfig(previous, httpChanged, featuresChanged, permissionsChanged, shellExecutableChanged, shellPathChanged, tunnelChanged, tunnelRuntimeChanged))
	}
	return nil
}

func (a *App) applyRuntimeConfig(next config.Config, httpChanged, featuresChanged, permissionsChanged, shellExecutableChanged, shellPathChanged, tunnelChanged, tunnelRuntimeChanged bool) error {
	if featuresChanged {
		if err := a.Tools.SyncFeatures(next.Features); err != nil {
			return err
		}
	}
	if permissionsChanged {
		a.Tools.SetGlobalAllowDirs(next.Permissions.AllowDirs)
	}
	if shellExecutableChanged {
		if err := a.Tools.SetShellExecutable(next.Shell.Executable); err != nil {
			return err
		}
	}
	if shellPathChanged {
		a.Tools.SetShellPath(next.Shell.Path)
	}
	if httpChanged {
		a.syncMCPHTTP(next.Server.Enabled)
	}
	if tunnelChanged && a.Tunnels != nil {
		if tunnelRuntimeChanged {
			if err := a.Tunnels.Reconcile(reloadContext(a), next.RuntimeTunnels()); err != nil {
				return err
			}
			for _, status := range a.Tunnels.Statuses() {
				client, _ := a.Tunnels.Client(status.ID)
				if metadata, loadErr := config.LoadTunnelMetadata(status.ID); loadErr == nil {
					_ = client.SeedMetadata(metadata)
				}
			}
		}
		a.syncLegacyTunnel(next)
	}
	return nil
}

func (a *App) rollbackRuntimeConfig(previous config.Config, httpChanged, featuresChanged, permissionsChanged, shellExecutableChanged, shellPathChanged, tunnelChanged, tunnelRuntimeChanged bool) error {
	var rollbackErr error
	if tunnelChanged && a.Tunnels != nil {
		if tunnelRuntimeChanged {
			rollbackErr = errors.Join(rollbackErr, a.Tunnels.Reconcile(reloadContext(a), previous.RuntimeTunnels()))
		}
		a.syncLegacyTunnel(previous)
	}
	if featuresChanged {
		rollbackErr = errors.Join(rollbackErr, a.Tools.SyncFeatures(previous.Features))
	}
	if permissionsChanged {
		a.Tools.SetGlobalAllowDirs(previous.Permissions.AllowDirs)
	}
	if shellExecutableChanged {
		rollbackErr = errors.Join(rollbackErr, a.Tools.SetShellExecutable(previous.Shell.Executable))
	}
	if shellPathChanged {
		a.Tools.SetShellPath(previous.Shell.Path)
	}
	if httpChanged {
		a.syncMCPHTTP(previous.Server.Enabled)
	}
	return rollbackErr
}

func reloadContext(a *App) context.Context {
	if a.runtimeCtx != nil {
		return a.runtimeCtx
	}
	return context.Background()
}

func (a *App) syncLegacyTunnel(cfg config.Config) {
	if cfg.Tunnel.Instances != nil || cfg.Tunnel.Admins != nil {
		a.Tunnel = nil
		return
	}
	if client, ok := a.Tunnels.Client(cfg.Tunnel.ID); ok {
		a.Tunnel = client
		_ = client.SyncManagementConfig(cfg.Tunnel)
	} else {
		a.Tunnel = tunnel.NewConfiguredWithLogger(cfg.Tunnel, a.Tools, a.Logger)
	}
}
