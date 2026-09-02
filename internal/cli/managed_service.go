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
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

const serviceReadyTimeout = 15 * time.Second

func upCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "up", Short: "Install and start the managed MCP service", Args: cobra.NoArgs, RunE: runUp}
	cmd.Flags().Bool("system", false, "use a machine-level service on Linux/macOS; elevates with sudo when needed")
	cmd.Flags().String("service-environment-hash", "", "internal managed environment snapshot hash")
	_ = cmd.Flags().MarkHidden("service-environment-hash")
	return cmd
}

func downCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "down", Short: "Stop and remove the managed MCP service", Args: cobra.NoArgs, RunE: runDown}
	cmd.Flags().Bool("system", false, "use the machine-level service on Linux/macOS; elevates with sudo when needed")
	return cmd
}

func restartCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "restart", Short: "Restart the managed MCP service", Args: cobra.NoArgs, RunE: runRestart}
	cmd.Flags().Bool("system", false, "use the machine-level service on Linux/macOS; elevates with sudo when needed")
	cmd.Flags().String("service-environment-hash", "", "internal managed environment snapshot hash")
	_ = cmd.Flags().MarkHidden("service-environment-hash")
	return cmd
}

func runUp(cmd *cobra.Command, _ []string) error {
	scope, err := managedScopeForCommand(cmd)
	if err != nil {
		return err
	}
	spec, manager, err := managedServiceForCommand(cmd, scope)
	if err != nil {
		return err
	}
	environmentHash, _ := cmd.Flags().GetString("service-environment-hash")
	if environmentHash == "" {
		source, err := config.Source()
		if err != nil {
			return err
		}
		if !source.Exists {
			return errors.New("chatgpt-mcp is not initialized; run chatgpt-mcp init first")
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		environmentHash, err = managed.SaveEnvironment(spec.ConfigRoot, managed.CaptureEnvironment(spec.Account, cfg.Shell.Path))
		if err != nil {
			return err
		}
	}
	spec.EnvironmentHash = environmentHash
	if scope == managed.ScopeSystem && managed.DetectScope() == managed.ScopeUser {
		return elevateManagedCommand(cmd, "up", environmentHash)
	}
	return runManagedUp(cmd, spec, manager)
}

func runDown(cmd *cobra.Command, _ []string) error {
	scope, err := managedScopeForCommand(cmd)
	if err != nil {
		return err
	}
	if scope == managed.ScopeSystem && managed.DetectScope() == managed.ScopeUser {
		return elevateManagedCommand(cmd, "down", "")
	}
	spec, manager, err := managedServiceForCommand(cmd, scope)
	if err != nil {
		return err
	}
	return runManagedDown(cmd, spec, manager)
}

func runRestart(cmd *cobra.Command, _ []string) error {
	scope, err := managedScopeForCommand(cmd)
	if err != nil {
		return err
	}
	spec, manager, err := managedServiceForCommand(cmd, scope)
	if err != nil {
		return err
	}
	environmentHash, _ := cmd.Flags().GetString("service-environment-hash")
	if environmentHash == "" {
		source, err := config.Source()
		if err != nil {
			return err
		}
		if !source.Exists {
			return errors.New("chatgpt-mcp is not initialized; run chatgpt-mcp init first")
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		environmentHash, err = managed.SaveEnvironment(spec.ConfigRoot, managed.CaptureEnvironment(spec.Account, cfg.Shell.Path))
		if err != nil {
			return err
		}
	}
	spec.EnvironmentHash = environmentHash
	if scope == managed.ScopeSystem && managed.DetectScope() == managed.ScopeUser {
		return elevateManagedCommand(cmd, "restart", environmentHash)
	}
	return runManagedRestart(cmd, spec, manager)
}

func runManagedRestart(cmd *cobra.Command, spec managed.Spec, manager managed.Manager) error {
	if err := runManagedDown(cmd, spec, manager); err != nil {
		return err
	}
	return runManagedUp(cmd, spec, manager)
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
	cfg, err := config.Load()
	if err != nil {
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
		logManagedAlreadyRunning(cmd, spec, manager, runtimeStatus, cfg.Tunnel)
		return nil
	}
	action := "installed"
	if backendStatus.Installed {
		if matches {
			action = "started"
		} else {
			action = "updated"
		}
	}
	log := commandLogger(cmd)
	defer log.Close()
	log.Action("SERVICE", managedServiceActionEvent(action), managedServiceActionMessage(action, spec.Scope))
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
	logManagedUp(log, spec, manager, status, action)
	if status.TunnelEnabled && status.TunnelConfigured && transientTunnelState(statusTunnelState(status, true)) && logger.CanAnimate(cmd.OutOrStdout()) {
		status = animateRuntimeTunnelState(cmd, status, statusTunnelWatchTimeout)
	}
	logRuntimeTunnelResult(log, status)
	logRuntimeTunnelMetadata(log, cfg.Tunnel, status, config.LoadTunnelMetadata)
	logManagedHints(log, spec)
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
	defer log.Close()
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

func logManagedAlreadyRunning(cmd *cobra.Command, spec managed.Spec, manager managed.Manager, status runtimeStatusResult, cfg tunnel.Config) {
	if status.TunnelEnabled && status.TunnelConfigured && transientTunnelState(statusTunnelState(status, true)) && logger.CanAnimate(cmd.OutOrStdout()) {
		status = animateRuntimeTunnelState(cmd, status, statusTunnelWatchTimeout)
	}
	log := commandLogger(cmd)
	log.Ready("SERVICE", "service.already-running", "Managed service already running")
	logManagedDetails(log, spec, manager)
	logRuntimeDetails(log, status)
	logRuntimeTunnelResult(log, status)
	logRuntimeTunnelMetadata(log, cfg, status, config.LoadTunnelMetadata)
	logManagedHints(log, spec)
}

func logManagedUp(log *logger.Logger, spec managed.Spec, manager managed.Manager, status runtimeStatusResult, action string) {
	message := "Managed service " + action
	if spec.Scope == managed.ScopeSystem {
		message = "System service " + action
	}
	log.Ready("SERVICE", "service."+action, message)
	logManagedDetails(log, spec, manager)
	log.Ready("SERVER", "server.started", "Server started")
	logRuntimeDetails(log, status)
}

func managedServiceActionMessage(action string, scope managed.Scope) string {
	prefix := "Managed service"
	if scope == managed.ScopeSystem {
		prefix = "System service"
	}
	switch action {
	case "installed":
		return "Installing " + strings.ToLower(prefix)
	case "updated":
		return "Updating " + strings.ToLower(prefix)
	default:
		return "Starting " + strings.ToLower(prefix)
	}
}

func managedServiceActionEvent(action string) string {
	switch action {
	case "installed":
		return "service.installing"
	case "updated":
		return "service.updating"
	default:
		return "service.starting"
	}
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
}

func logRuntimeTunnelResult(log *logger.Logger, status runtimeStatusResult) {
	state := statusTunnelState(status, true)
	switch state {
	case "connected":
		log.Ready("TUNNEL", "tunnel.connected", "OpenAI Secure MCP Tunnel connected")
	case "failed":
		var err error
		if status.TunnelLastError != "" {
			err = errors.New(status.TunnelLastError)
		}
		log.Failure("TUNNEL", "tunnel.failed", "OpenAI Secure MCP Tunnel failed", err)
	case "starting", "connecting", "reconnecting":
		log.Warning("TUNNEL", "tunnel.pending", "OpenAI Secure MCP Tunnel is still "+state, nil)
	default:
		log.Notice("TUNNEL", "tunnel."+strings.ReplaceAll(state, " ", "-"), "OpenAI Secure MCP Tunnel is "+state)
	}
	if status.TunnelID != "" {
		log.Detail("tunnel id", status.TunnelID)
	}
}

type tunnelMetadataLoadFunc func(string) (tunnel.Metadata, error)

func logRuntimeTunnelMetadata(log *logger.Logger, cfg tunnel.Config, status runtimeStatusResult, load tunnelMetadataLoadFunc) {
	if log == nil || load == nil || statusTunnelState(status, true) != "connected" {
		return
	}
	id := strings.TrimSpace(status.TunnelID)
	if id == "" {
		id = strings.TrimSpace(cfg.ID)
	}
	metadata, err := load(id)
	if err != nil {
		log.Verbose("TUNNEL", "tunnel.metadata.unavailable", "Tunnel metadata unavailable", logger.WithVerbose("error", err.Error()))
		return
	}
	if metadata.Name != "" {
		log.Detail("tunnel name", metadata.Name)
	}
	if metadata.Description != "" {
		log.Detail("tunnel description", metadata.Description)
	}
	if scope := tunnelMetadataScope(metadata); scope != "" {
		log.Detail("tunnel scope", scope)
	}
}

func tunnelMetadataScope(metadata tunnel.Metadata) string {
	parts := make([]string, 0, 3)
	if len(metadata.OrganizationIDs) > 0 {
		parts = append(parts, "organization:"+strings.Join(metadata.OrganizationIDs, ","))
	}
	if len(metadata.WorkspaceIDs) > 0 {
		parts = append(parts, "workspace:"+strings.Join(metadata.WorkspaceIDs, ","))
	}
	if len(metadata.TenantIDs) > 0 {
		parts = append(parts, "tenant:"+strings.Join(metadata.TenantIDs, ","))
	}
	return strings.Join(parts, " · ")
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
			parts = append(parts, "starting")
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
