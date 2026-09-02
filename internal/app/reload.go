package app

import (
	"errors"
	"slices"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

func (a *App) ReloadConfig(next config.Config) error {
	if a == nil || a.Config == nil || a.Tools == nil {
		return errors.New("runtime is unavailable")
	}
	if err := config.Validate(next); err != nil {
		return err
	}
	previous := a.Config.Snapshot()
	featuresChanged := previous.Features != next.Features
	permissionsChanged := !slices.Equal(previous.Permissions.AllowDirs, next.Permissions.AllowDirs)
	tunnelChanged := previous.Tunnel != next.Tunnel
	tunnelRuntimeChanged := tunnelChanged && !tunnel.RuntimeConfigEqual(previous.Tunnel, next.Tunnel)
	if featuresChanged {
		if err := a.Tools.SyncFeatures(next.Features); err != nil {
			return err
		}
	}
	if permissionsChanged {
		a.Tools.SetGlobalAllowDirs(next.Permissions.AllowDirs)
	}
	if tunnelChanged && a.Tunnel != nil {
		var err error
		if tunnelRuntimeChanged {
			if a.running {
				err = a.Tunnel.Reconfigure(next.Tunnel, func() error { return nil })
			} else {
				err = a.Tunnel.Configure(next.Tunnel)
			}
		} else {
			err = a.Tunnel.SyncManagementConfig(next.Tunnel)
		}
		if err != nil {
			return errors.Join(err, a.rollbackRuntimeConfig(previous, featuresChanged, permissionsChanged, false, false))
		}
		if tunnelRuntimeChanged {
			if metadata, loadErr := config.LoadTunnelMetadata(next.Tunnel.ID); loadErr == nil {
				_ = a.Tunnel.SeedMetadata(metadata)
			}
		}
	}
	if _, err := a.Config.Update(func(config.Config) (config.Config, error) { return next, nil }); err != nil {
		return errors.Join(err, a.rollbackRuntimeConfig(previous, featuresChanged, permissionsChanged, tunnelChanged, tunnelRuntimeChanged))
	}
	return nil
}

func (a *App) rollbackRuntimeConfig(previous config.Config, featuresChanged, permissionsChanged, tunnelChanged, tunnelRuntimeChanged bool) error {
	var rollbackErr error
	if tunnelChanged && a.Tunnel != nil {
		if tunnelRuntimeChanged {
			if a.running {
				rollbackErr = errors.Join(rollbackErr, a.Tunnel.Reconfigure(previous.Tunnel, func() error { return nil }))
			} else {
				rollbackErr = errors.Join(rollbackErr, a.Tunnel.Configure(previous.Tunnel))
			}
		} else {
			rollbackErr = errors.Join(rollbackErr, a.Tunnel.SyncManagementConfig(previous.Tunnel))
		}
	}
	if featuresChanged {
		rollbackErr = errors.Join(rollbackErr, a.Tools.SyncFeatures(previous.Features))
	}
	if permissionsChanged {
		a.Tools.SetGlobalAllowDirs(previous.Permissions.AllowDirs)
	}
	return rollbackErr
}
