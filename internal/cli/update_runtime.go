package cli

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/install"
	managed "go.mewis.me/chatgpt-mcp/internal/service"
)

type updateRuntimeState struct {
	Running bool
	Status  runtimeStatusResult
}

type updateRuntimeRestartFunc func(*cobra.Command, install.Layout, runtimeStatusResult) error

func captureUpdateRuntimeState(parent context.Context) (updateRuntimeState, error) {
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	status, running, err := managedRuntimeStatus(ctx)
	if err != nil {
		return updateRuntimeState{}, err
	}
	return updateRuntimeState{Running: running, Status: status}, nil
}

func coordinateUpdatedRuntime(cmd *cobra.Command, installed install.Result, state updateRuntimeState, noRestart bool) error {
	return coordinateUpdatedRuntimeWith(cmd, installed, state, noRestart, restartManagedRuntimeAfterUpdate)
}

func coordinateUpdatedRuntimeWith(cmd *cobra.Command, installed install.Result, state updateRuntimeState, noRestart bool, restart updateRuntimeRestartFunc) error {
	if !state.Running {
		return nil
	}
	log := commandLogger(cmd)
	if noRestart {
		log.Notice("UPDATE", "update.restart-skipped", "Runtime restart skipped")
		log.Detail("pid", state.Status.PID)
		return nil
	}
	if !state.Status.Managed {
		log.Notice("UPDATE", "update.foreground-running", "Foreground runtime is still using the previous version; restart it manually")
		log.Detail("pid", state.Status.PID)
		return nil
	}
	log.Action("UPDATE", "update.runtime-restarting", "Restarting managed runtime")
	if err := restart(cmd, installed.Layout, state.Status); err != nil {
		log.Warning("UPDATE", "update.runtime-restart-failed", "Managed runtime restart failed; rolling back", err)
		if rollbackErr := install.RollbackResult(installed); rollbackErr != nil {
			return fmt.Errorf("managed runtime restart failed: %w; rollback failed: %v", err, rollbackErr)
		}
		previous := installed.Activation.PreviousVersion
		log.Ready("UPDATE", "update.rollback-complete", "Previous version restored")
		if previous != "" {
			log.Detail("current", previous)
		}
		if rollbackRestartErr := restart(cmd, installed.Layout, state.Status); rollbackRestartErr != nil {
			return fmt.Errorf("managed runtime restart failed: %w; rolled back to %s but previous runtime restart failed: %v", err, previous, rollbackRestartErr)
		}
		log.Ready("UPDATE", "update.rollback-runtime-restarted", "Previous managed runtime restarted")
		return fmt.Errorf("managed runtime restart failed: %w; rolled back to %s", err, previous)
	}
	log.Ready("UPDATE", "update.runtime-restarted", "Managed runtime restarted")
	return nil
}

func restartManagedRuntimeAfterUpdate(cmd *cobra.Command, layout install.Layout, status runtimeStatusResult) error {
	if filepath.Clean(status.ConfigRoot) != filepath.Clean(config.RootPath()) {
		return fmt.Errorf("managed runtime config root mismatch: runtime %s, selected %s", status.ConfigRoot, config.RootPath())
	}
	scope := managed.Scope(status.ServiceScope)
	if scope != managed.ScopeUser && scope != managed.ScopeSystem {
		return fmt.Errorf("managed runtime has invalid service scope %q", status.ServiceScope)
	}
	account, err := managed.InvokingAccount(scope)
	if err != nil {
		return err
	}
	spec, err := managed.NewSpec(status.ConfigRoot, layout.CanonicalBinary, scope, account)
	if err != nil {
		return err
	}
	if status.ServiceID == "" || spec.ID != status.ServiceID {
		return fmt.Errorf("managed runtime service mismatch: runtime %s, expected %s", status.ServiceID, spec.ID)
	}
	if scope == managed.ScopeSystem && managed.DetectScope() == managed.ScopeUser {
		environmentHash, err := saveManagedEnvironment(spec)
		if err != nil {
			return err
		}
		return elevateManagedCommandWithBinary(cmd, "restart", environmentHash, layout.CanonicalBinary)
	}
	return restartManagedRuntimeInPlace(cmd.Context(), spec, managed.NewManager())
}

func restartManagedRuntimeInPlace(parent context.Context, spec managed.Spec, manager managed.Manager) error {
	backendStatus, err := manager.Status(spec)
	if err != nil {
		return err
	}
	if !backendStatus.Installed {
		return errors.New("managed service is not installed")
	}
	if err := manager.Stop(spec); err != nil {
		return err
	}
	if err := waitRuntimeStopped(parent, serviceReadyTimeout); err != nil {
		return err
	}
	if err := manager.Start(spec); err != nil {
		return err
	}
	_, err = waitManagedRuntimeReady(parent, spec, serviceReadyTimeout)
	return err
}
