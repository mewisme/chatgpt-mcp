package cli

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/config"
	managed "go.mewis.me/chatgpt-mcp/internal/service"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func statusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show local configuration, runtime, service, workspaces, and upstreams",
		RunE: func(cmd *cobra.Command, args []string) error {
			scope := managed.DetectScope()
			account, err := managed.InvokingAccount(scope)
			if err != nil {
				return err
			}
			if err := resolveManagedConfigRoot(cmd, scope, account); err != nil {
				return err
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			workspaces, err := workspace.NewManager(workspace.DefaultStorePath()).List()
			if err != nil {
				return err
			}
			upstreams := upstream.NewManager(upstream.NewStore(upstream.Path()))
			if err := upstreams.Load(); err != nil {
				return err
			}
			source, err := config.Source()
			if err != nil {
				return err
			}
			log := commandLogger(cmd)
			log.Info("STATUS", "local runtime configuration")
			log.Detail("initialized", source.Exists)
			log.Detail("config", source.Path)
			log.Detail("format", source.Format)
			logEndpointDetails(log, cfg)
			log.Detail("auth", fmt.Sprintf("mcp=%t admin=%t", cfg.Auth.MCPEnabled, cfg.Auth.AdminEnabled))
			ctx, cancel := context.WithTimeout(cmd.Context(), time.Second)
			runtimeStatus, running, runtimeErr := managedRuntimeStatus(ctx)
			cancel()
			if runtimeErr != nil {
				return runtimeErr
			}
			if running {
				log.Detail("runtime", "running")
				log.Detail("managed", runtimeStatus.Managed)
				if runtimeStatus.RunID != "" {
					log.Detail("session", shortSessionID(runtimeStatus.RunID))
				}
				log.Detail("pid", runtimeStatus.PID)
				if !runtimeStatus.StartedAt.IsZero() {
					log.Detail("started", runtimeStatus.StartedAt.Local().Format(time.RFC3339))
				}
				if runtimeStatus.Managed {
					log.Detail("scope", runtimeStatus.ServiceScope)
					log.Detail("backend", runtimeBackendLabel(runtimeStatus.ServiceScope))
					log.Detail("service", runtimeStatus.ServiceID)
				}
				log.Detail("tunnel", runtimeTunnelSummary(runtimeStatus))
				if runtimeStatus.TunnelID != "" {
					log.Detail("tunnel id", runtimeStatus.TunnelID)
				}
			} else {
				log.Detail("runtime", "stopped")
				configured := tunnel.Configured(cfg.Tunnel)
				state := runtimeStatusResult{TunnelEnabled: cfg.Tunnel.Enabled, TunnelConfigured: configured, TunnelID: cfg.Tunnel.ID}
				log.Detail("tunnel", runtimeTunnelSummary(state))
				if cfg.Tunnel.ID != "" {
					log.Detail("tunnel id", cfg.Tunnel.ID)
				}
				for _, item := range installedManagedServices(account) {
					log.Detail("service "+string(item.spec.Scope), fmt.Sprintf("installed (%s)", managedBackendLabel(item.manager, item.spec)))
				}
			}
			log.Detail("workspaces", len(workspaces))
			log.Detail("upstreams", len(upstreams.List()))
			return nil
		},
	}
}

type installedManagedService struct {
	spec    managed.Spec
	manager managed.Manager
}

func installedManagedServices(account managed.Account) []installedManagedService {
	manager := managed.NewManager()
	scopes := []managed.Scope{managed.ScopeUser}
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		scopes = append(scopes, managed.ScopeSystem)
	}
	result := make([]installedManagedService, 0, len(scopes))
	for _, scope := range scopes {
		spec := managed.Spec{ID: managed.ID(config.RootPath(), scope), Scope: scope, ConfigRoot: config.RootPath(), Account: account}
		status, err := manager.Status(spec)
		if err == nil && status.Installed {
			result = append(result, installedManagedService{spec: spec, manager: manager})
		}
	}
	return result
}

func runtimeBackendLabel(scope string) string {
	if runtime.GOOS == "linux" {
		if scope == string(managed.ScopeUser) {
			return "systemd --user"
		}
		return "systemd"
	}
	if runtime.GOOS == "darwin" {
		if scope == string(managed.ScopeUser) {
			return "launchd LaunchAgent"
		}
		return "launchd LaunchDaemon"
	}
	if runtime.GOOS == "windows" {
		return "task-scheduler"
	}
	return "unknown"
}
