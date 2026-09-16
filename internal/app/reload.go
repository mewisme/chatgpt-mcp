package app

import (
	"errors"
	"slices"

	"go.mewis.me/chatgpt-mcp/internal/config"
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
	permissionsChanged := !slices.Equal(previous.Permissions.AllowDirs, next.Permissions.AllowDirs)
	shellExecutableChanged := previous.Shell.Executable != next.Shell.Executable
	shellPathChanged := !slices.Equal(previous.Shell.Path, next.Shell.Path)

	if _, err := a.Config.Update(func(config.Config) (config.Config, error) { return next, nil }); err != nil {
		return err
	}
	if reloadTestAfterCommit != nil {
		reloadTestAfterCommit()
	}
	if err := a.applyRuntimeConfig(next, httpChanged, permissionsChanged, shellExecutableChanged, shellPathChanged); err != nil {
		_, restoreErr := a.Config.Update(func(config.Config) (config.Config, error) { return previous, nil })
		return errors.Join(err, restoreErr, a.rollbackRuntimeConfig(previous, httpChanged, permissionsChanged, shellExecutableChanged, shellPathChanged))
	}
	return nil
}

func (a *App) applyRuntimeConfig(next config.Config, httpChanged, permissionsChanged, shellExecutableChanged, shellPathChanged bool) error {
	if err := a.Tools.SyncPlugins(); err != nil {
		return err
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
	return nil
}

func (a *App) rollbackRuntimeConfig(previous config.Config, httpChanged, permissionsChanged, shellExecutableChanged, shellPathChanged bool) error {
	rollbackErr := a.Tools.SyncPlugins()
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
