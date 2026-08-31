package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	managed "go.mewis.me/chatgpt-mcp/internal/service"
)

const serviceReadyTimeout = 15 * time.Second

func upCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "up", Short: "Install and start the managed MCP service", Args: cobra.NoArgs, RunE: runUp}
	cmd.Flags().Bool("system", false, "use a machine-level service on Linux/macOS; elevates with sudo when needed")
	return cmd
}

func downCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "down", Short: "Stop and remove the managed MCP service", Args: cobra.NoArgs, RunE: runDown}
	cmd.Flags().Bool("system", false, "use the machine-level service on Linux/macOS; elevates with sudo when needed")
	return cmd
}

func runUp(cmd *cobra.Command, _ []string) error {
	scope, err := managedScopeForCommand(cmd)
	if err != nil {
		return err
	}
	if scope == managed.ScopeSystem && managed.DetectScope() == managed.ScopeUser {
		return elevateManagedCommand(cmd, "up")
	}
	spec, manager, err := managedServiceForCommand(cmd, scope)
	if err != nil {
		return err
	}
	return runManagedUp(cmd, spec, manager)
}

func runDown(cmd *cobra.Command, _ []string) error {
	scope, err := managedScopeForCommand(cmd)
	if err != nil {
		return err
	}
	if scope == managed.ScopeSystem && managed.DetectScope() == managed.ScopeUser {
		return elevateManagedCommand(cmd, "down")
	}
	spec, manager, err := managedServiceForCommand(cmd, scope)
	if err != nil {
		return err
	}
	return runManagedDown(cmd, spec, manager)
}

func managedScopeForCommand(cmd *cobra.Command) (managed.Scope, error) {
	system, err := cmd.Flags().GetBool("system")
	if err != nil {
		return "", err
	}
	if system {
		if runtime.GOOS == "windows" {
			return "", errors.New("system service scope is not supported on Windows; managed services use a per-user Scheduled Task")
		}
		return managed.ScopeSystem, nil
	}
	return managed.DetectScope(), nil
}

func managedServiceForCommand(cmd *cobra.Command, scope managed.Scope) (managed.Spec, managed.Manager, error) {
	account, err := managed.InvokingAccount(scope)
	if err != nil {
		return managed.Spec{}, nil, err
	}
	if err := resolveManagedConfigRoot(cmd, scope, account); err != nil {
		return managed.Spec{}, nil, err
	}
	spec, err := managed.NewSpec(config.RootPath(), os.Args[0], scope, account)
	if err != nil {
		return managed.Spec{}, nil, err
	}
	return spec, managed.NewManager(), nil
}

func resolveManagedConfigRoot(cmd *cobra.Command, scope managed.Scope, account managed.Account) error {
	flagValue, err := cmd.Root().PersistentFlags().GetString("config-dir")
	if err != nil {
		return err
	}
	if strings.TrimSpace(flagValue) != "" || strings.TrimSpace(os.Getenv(configformat.EnvConfigDir)) != "" || scope != managed.ScopeSystem {
		return nil
	}
	return configformat.SetRootPath(managed.DefaultConfigRoot(account))
}

func runManagedUp(cmd *cobra.Command, spec managed.Spec, manager managed.Manager) error {
	source, err := config.Source()
	if err != nil {
		return err
	}
	if !source.Exists {
		return errors.New("chatgpt-mcp is not initialized; run chatgpt-mcp init first")
	}
	if _, err := config.Verify(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Second)
	runtimeStatus, running, runtimeErr := managedRuntimeStatus(ctx)
	cancel()
	if runtimeErr != nil {
		return runtimeErr
	}
	if running {
		if !runtimeStatus.Managed {
			return fmt.Errorf("runtime is already running outside the managed service (pid %d); stop the foreground serve process first", runtimeStatus.PID)
		}
		if runtimeStatus.ServiceID != spec.ID || runtimeStatus.ServiceScope != string(spec.Scope) {
			return managedScopeConflict(runtimeStatus, spec, "up")
		}
	}
	backendStatus, err := manager.Status(spec)
	if err != nil {
		return err
	}
	matches, err := manager.DefinitionMatches(spec)
	if err != nil {
		return err
	}
	if running && backendStatus.Installed && matches {
		logManagedAlreadyRunning(cmd, spec, manager, runtimeStatus)
		return nil
	}
	log := commandLogger(cmd)
	if running {
		log.Action("SERVICE", "service.updating", "Updating managed service")
		if err := requestManagedShutdown(cmd.Context()); err != nil {
			return err
		}
		if err := waitRuntimeStopped(cmd.Context(), serviceReadyTimeout); err != nil {
			return err
		}
	} else if backendStatus.Running {
		if err := manager.Stop(spec); err != nil {
			return err
		}
	}
	action := "installed"
	if backendStatus.Installed {
		if matches {
			action = "started"
		} else {
			action = "updated"
		}
	}
	if !backendStatus.Installed || !matches {
		if err := manager.Install(spec); err != nil {
			return err
		}
	}
	if err := manager.Start(spec); err != nil {
		return err
	}
	status, err := waitManagedRuntimeReady(cmd.Context(), spec, serviceReadyTimeout)
	if err != nil {
		return err
	}
	logManagedUp(cmd, spec, manager, status, action)
	return nil
}

func runManagedDown(cmd *cobra.Command, spec managed.Spec, manager managed.Manager) error {
	ctx, cancel := context.WithTimeout(cmd.Context(), 2*time.Second)
	runtimeStatus, running, runtimeErr := managedRuntimeStatus(ctx)
	cancel()
	if runtimeErr != nil {
		return runtimeErr
	}
	if running {
		if !runtimeStatus.Managed {
			return fmt.Errorf("runtime is running in foreground mode (pid %d); cgm down will not stop it", runtimeStatus.PID)
		}
		if runtimeStatus.ServiceID != spec.ID || runtimeStatus.ServiceScope != string(spec.Scope) {
			return managedScopeConflict(runtimeStatus, spec, "down")
		}
	}
	backendStatus, err := manager.Status(spec)
	if err != nil {
		return err
	}
	if !running && !backendStatus.Installed {
		commandLogger(cmd).Notice("SERVICE", "service.not-installed", "Managed service is not installed")
		return nil
	}
	log := commandLogger(cmd)
	log.Action("SERVICE", "service.stopping", "Stopping managed service")
	if running {
		if err := requestManagedShutdown(cmd.Context()); err != nil {
			return err
		}
		if err := waitRuntimeStopped(cmd.Context(), serviceReadyTimeout); err != nil {
			return err
		}
	} else if backendStatus.Running {
		if err := manager.Stop(spec); err != nil {
			return err
		}
	}
	if backendStatus.Installed {
		if err := manager.Uninstall(spec); err != nil {
			return err
		}
	}
	log.Ready("SERVICE", "service.stopped", "Server stopped")
	log.Ready("SERVICE", "service.removed", "Managed service removed")
	log.Detail("config preserved", spec.ConfigRoot)
	log.Detail("logs preserved", filepath.Join(spec.ConfigRoot, "logs"))
	return nil
}

func managedRuntimeStatus(ctx context.Context) (runtimeStatusResult, bool, error) {
	status, err := requestRuntimeStatus(ctx)
	if err == nil {
		return status, true, nil
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "no running server found") || strings.Contains(message, "control endpoint unavailable") || strings.Contains(message, "connection refused") || strings.Contains(message, "actively refused") {
		return runtimeStatusResult{}, false, nil
	}
	return runtimeStatusResult{}, false, err
}

func requestManagedShutdown(parent context.Context) error {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	return requestRuntimeShutdown(ctx)
}

func waitManagedRuntimeReady(parent context.Context, spec managed.Spec, timeout time.Duration) (runtimeStatusResult, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(parent, time.Second)
		status, running, err := managedRuntimeStatus(ctx)
		cancel()
		if err != nil {
			lastErr = err
		} else if running {
			if !status.Managed || status.ServiceID != spec.ID || status.ServiceScope != string(spec.Scope) {
				return runtimeStatusResult{}, managedScopeConflict(status, spec, "up")
			}
			return status, nil
		}
		select {
		case <-parent.Done():
			return runtimeStatusResult{}, parent.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
	if lastErr != nil {
		return runtimeStatusResult{}, fmt.Errorf("managed service did not become ready: %w", lastErr)
	}
	return runtimeStatusResult{}, errors.New("managed service did not become ready")
}

func waitRuntimeStopped(parent context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(parent, time.Second)
		_, running, err := managedRuntimeStatus(ctx)
		cancel()
		if err != nil {
			return err
		}
		if !running {
			return nil
		}
		select {
		case <-parent.Done():
			return parent.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
	return errors.New("managed runtime did not stop")
}

func managedScopeConflict(status runtimeStatusResult, spec managed.Spec, action string) error {
	if status.ServiceScope == string(managed.ScopeSystem) && spec.Scope == managed.ScopeUser {
		return fmt.Errorf("runtime is managed by a system service; use cgm %s --system", action)
	}
	if status.ServiceScope == string(managed.ScopeUser) && spec.Scope == managed.ScopeSystem {
		return fmt.Errorf("runtime is managed by a user service; use cgm %s", action)
	}
	return fmt.Errorf("another managed service is already running for this config (service %s, pid %d)", status.ServiceID, status.PID)
}

func logManagedAlreadyRunning(cmd *cobra.Command, spec managed.Spec, manager managed.Manager, status runtimeStatusResult) {
	log := commandLogger(cmd)
	log.Ready("SERVICE", "service.already-running", "Managed service already running")
	logManagedDetails(log, spec, manager)
	logRuntimeDetails(log, status)
	logManagedHints(log, spec)
}

func logManagedUp(cmd *cobra.Command, spec managed.Spec, manager managed.Manager, status runtimeStatusResult, action string) {
	log := commandLogger(cmd)
	message := "Managed service " + action
	if spec.Scope == managed.ScopeSystem {
		message = "System service " + action
	}
	log.Ready("SERVICE", "service.installed", message)
	logManagedDetails(log, spec, manager)
	log.Ready("SERVER", "server.started", "Server started")
	logRuntimeDetails(log, status)
	logManagedHints(log, spec)
}

func logManagedDetails(log *logger.Logger, spec managed.Spec, manager managed.Manager) {
	log.Detail("scope", spec.Scope)
	log.Detail("backend", managedBackendLabel(manager, spec))
	if spec.Scope == managed.ScopeSystem && spec.Account.Username != "" {
		log.Detail("user", spec.Account.Username)
	}
	log.Detail("config", spec.ConfigRoot)
	log.Detail("service", spec.ID)
}

func logRuntimeDetails(log *logger.Logger, status runtimeStatusResult) {
	if status.RunID != "" {
		log.Detail("session", shortSessionID(status.RunID))
	}
	log.Detail("pid", status.PID)
	log.Detail("mcp", fmt.Sprintf("http://127.0.0.1:%d/mcp", status.ServerPort))
	if status.AdminEnabled {
		log.Detail("admin", fmt.Sprintf("http://127.0.0.1:%d/", status.AdminPort))
	}
	log.Detail("tunnel", runtimeTunnelSummary(status))
	if status.TunnelID != "" {
		log.Detail("tunnel id", status.TunnelID)
	}
}

func runtimeTunnelSummary(status runtimeStatusResult) string {
	parts := []string{}
	if status.TunnelEnabled {
		parts = append(parts, "enabled")
	} else {
		parts = append(parts, "disabled")
	}
	if status.TunnelConfigured {
		parts = append(parts, "configured")
	} else {
		parts = append(parts, "not configured")
	}
	if status.TunnelEnabled {
		switch {
		case status.TunnelReady:
			parts = append(parts, "connected")
		case status.TunnelRestarting:
			parts = append(parts, "reconnecting")
		case status.TunnelRunning:
			parts = append(parts, "connecting")
		default:
			parts = append(parts, "stopped")
		}
	}
	return strings.Join(parts, " · ")
}

func logManagedHints(log *logger.Logger, spec managed.Spec) {
	if spec.Scope == managed.ScopeSystem {
		log.Notice("SERVICE", "service.machine-start", "Service starts automatically with the machine")
	} else if warning := managed.PersistenceWarning(spec); warning != "" {
		log.Warning("SERVICE", "service.persistence.warning", warning, nil)
		if runtime.GOOS == "linux" && spec.Account.Username != "" {
			log.Detail("machine service", "cgm up --system")
		}
	} else {
		log.Notice("SERVICE", "service.detached", "Runtime will continue independently of this terminal")
	}
	log.Notice("SERVICE", "service.logs-hint", "View logs: cgm logs -f")
	stop := "cgm down"
	if spec.Scope == managed.ScopeSystem && runtime.GOOS != "windows" {
		stop = "cgm down --system"
	}
	log.Notice("SERVICE", "service.stop-hint", "Stop service: "+stop)
}

func managedBackendLabel(manager managed.Manager, spec managed.Spec) string {
	if runtime.GOOS == "linux" && spec.Scope == managed.ScopeUser {
		return "systemd --user"
	}
	return manager.Backend()
}
