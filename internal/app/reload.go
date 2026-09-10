package app

import (
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
	shellApprovalPolicyChanged := previous.Shell.ApprovalPolicy != next.Shell.ApprovalPolicy || !slices.Equal(previous.Shell.ApprovalAllowCommands, next.Shell.ApprovalAllowCommands) || !slices.Equal(previous.Shell.ApprovalDenyCommands, next.Shell.ApprovalDenyCommands)
	shellEnvironmentChanged := previous.Shell.EnvironmentPolicy != next.Shell.EnvironmentPolicy || previous.Shell.SandboxPolicy != next.Shell.SandboxPolicy || previous.Shell.NetworkPolicy != next.Shell.NetworkPolicy || !slices.Equal(previous.Shell.EnvironmentAllow, next.Shell.EnvironmentAllow) || !slices.Equal(previous.Shell.Path, next.Shell.Path)
	tunnelChanged := previous.Tunnel != next.Tunnel
	tunnelRuntimeChanged := tunnelChanged && !tunnel.RuntimeConfigEqual(previous.Tunnel, next.Tunnel)

	if _, err := a.Config.Update(func(config.Config) (config.Config, error) { return next, nil }); err != nil {
		return err
	}
	if reloadTestAfterCommit != nil {
		reloadTestAfterCommit()
	}
	if err := a.applyRuntimeConfig(next, httpChanged, featuresChanged, permissionsChanged, shellApprovalPolicyChanged, shellEnvironmentChanged, tunnelChanged, tunnelRuntimeChanged); err != nil {
		_, restoreErr := a.Config.Update(func(config.Config) (config.Config, error) { return previous, nil })
		return errors.Join(err, restoreErr, a.rollbackRuntimeConfig(previous, httpChanged, featuresChanged, permissionsChanged, shellApprovalPolicyChanged, shellEnvironmentChanged, tunnelChanged, tunnelRuntimeChanged))
	}
	return nil
}

func (a *App) applyRuntimeConfig(next config.Config, httpChanged, featuresChanged, permissionsChanged, shellApprovalPolicyChanged, shellEnvironmentChanged, tunnelChanged, tunnelRuntimeChanged bool) error {
	if featuresChanged {
		if err := a.Tools.SyncFeatures(next.Features); err != nil {
			return err
		}
	}
	if permissionsChanged {
		a.Tools.SetGlobalAllowDirs(next.Permissions.AllowDirs)
	}
	if shellApprovalPolicyChanged {
		if err := a.Tools.SetShellApprovalPolicy(next.Shell.ApprovalPolicy); err != nil {
			return err
		}
		if err := a.Tools.SetShellApprovalCommands(next.Shell.ApprovalAllowCommands, next.Shell.ApprovalDenyCommands); err != nil {
			return err
		}
	}
	if shellEnvironmentChanged {
		if err := a.Tools.SetShellEnvironmentPolicy(next.Shell.EnvironmentPolicy); err != nil {
			return err
		}
		if err := a.Tools.SetShellSandboxPolicy(next.Shell.SandboxPolicy); err != nil {
			return err
		}
		if err := a.Tools.SetShellNetworkPolicy(next.Shell.NetworkPolicy); err != nil {
			return err
		}
		a.Tools.SetShellEnvironmentAllow(next.Shell.EnvironmentAllow)
		a.Tools.SetShellPath(next.Shell.Path)
	}
	if httpChanged {
		a.syncMCPHTTP(next.Server.Enabled)
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
			return err
		}
		if tunnelRuntimeChanged {
			if metadata, loadErr := config.LoadTunnelMetadata(next.Tunnel.ID); loadErr == nil {
				_ = a.Tunnel.SeedMetadata(metadata)
			}
		}
	}
	return nil
}

func (a *App) rollbackRuntimeConfig(previous config.Config, httpChanged, featuresChanged, permissionsChanged, shellApprovalPolicyChanged, shellEnvironmentChanged, tunnelChanged, tunnelRuntimeChanged bool) error {
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
	if shellApprovalPolicyChanged {
		rollbackErr = errors.Join(rollbackErr, a.Tools.SetShellApprovalPolicy(previous.Shell.ApprovalPolicy))
		rollbackErr = errors.Join(rollbackErr, a.Tools.SetShellApprovalCommands(previous.Shell.ApprovalAllowCommands, previous.Shell.ApprovalDenyCommands))
	}
	if shellEnvironmentChanged {
		rollbackErr = errors.Join(rollbackErr, a.Tools.SetShellEnvironmentPolicy(previous.Shell.EnvironmentPolicy))
		rollbackErr = errors.Join(rollbackErr, a.Tools.SetShellSandboxPolicy(previous.Shell.SandboxPolicy))
		rollbackErr = errors.Join(rollbackErr, a.Tools.SetShellNetworkPolicy(previous.Shell.NetworkPolicy))
		a.Tools.SetShellEnvironmentAllow(previous.Shell.EnvironmentAllow)
		a.Tools.SetShellPath(previous.Shell.Path)
	}
	if httpChanged {
		a.syncMCPHTTP(previous.Server.Enabled)
	}
	return rollbackErr
}
