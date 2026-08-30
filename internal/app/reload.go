package app

import (
	"errors"
	"slices"

	"go.mewis.me/chatgpt-mcp/internal/config"
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
	if featuresChanged {
		if err := a.Tools.SyncFeatures(next.Features); err != nil {
			return err
		}
	}
	if permissionsChanged {
		a.Tools.SetGlobalAllowDirs(next.Permissions.AllowDirs)
	}
	if tunnelChanged && a.Tunnel != nil {
		if err := a.Tunnel.Reconfigure(next.Tunnel, func() error { return nil }); err != nil {
			return errors.Join(err, a.rollbackRuntimeConfig(previous, featuresChanged, permissionsChanged, false))
		}
	}
	if _, err := a.Config.Update(func(config.Config) (config.Config, error) { return next, nil }); err != nil {
		return errors.Join(err, a.rollbackRuntimeConfig(previous, featuresChanged, permissionsChanged, tunnelChanged))
	}
	return nil
}

func (a *App) rollbackRuntimeConfig(previous config.Config, featuresChanged, permissionsChanged, tunnelChanged bool) error {
	var rollbackErr error
	if tunnelChanged && a.Tunnel != nil {
		rollbackErr = errors.Join(rollbackErr, a.Tunnel.Reconfigure(previous.Tunnel, func() error { return nil }))
	}
	if featuresChanged {
		rollbackErr = errors.Join(rollbackErr, a.Tools.SyncFeatures(previous.Features))
	}
	if permissionsChanged {
		a.Tools.SetGlobalAllowDirs(previous.Permissions.AllowDirs)
	}
	return rollbackErr
}
