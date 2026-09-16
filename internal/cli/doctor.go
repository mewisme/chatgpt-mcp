package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/notification"
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	shellruntime "go.mewis.me/chatgpt-mcp/internal/shell"
	"go.mewis.me/chatgpt-mcp/internal/version"
)

func doctorCommand() *cobra.Command {
	return &cobra.Command{Use: "doctor", Short: "Check local runtime dependencies", Args: cobra.NoArgs, RunE: runDoctor}
}

func runDoctor(cmd *cobra.Command, _ []string) error {
	logCommandStep(cmd, "DOCTOR", "doctor.shell.inspecting", "Checking Bash shell provider")
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	store, err := pluginpkg.NewStore(pluginpkg.DefaultLayout(), pluginpkg.RuntimeContext{})
	if err != nil {
		return fmt.Errorf("open plugin store: %w", err)
	}
	reconcile, err := pluginpkg.Reconcile(store)
	if err != nil {
		return fmt.Errorf("reconcile plugin state: %w", err)
	}
	log := commandLogger(cmd)
	if reconcile.CorruptLock {
		log.Warning("DOCTOR", "doctor.plugin.lock-recovered", "Corrupt plugin lock quarantined; plugins require explicit repair", nil)
		log.Detail("quarantine", reconcile.QuarantinePath)
	}
	if len(reconcile.Disabled) > 0 {
		ids := make([]string, len(reconcile.Disabled))
		for index, id := range reconcile.Disabled {
			ids[index] = string(id)
		}
		log.Warning("DOCTOR", "doctor.plugin.disabled-unsafe", "Unsafe plugin activation state disabled", nil)
		log.Detail("disabled plugins", strings.Join(ids, ", "))
		for _, id := range reconcile.Disabled {
			if reason := reconcile.Issues[id]; reason != "" {
				log.Detail("plugin "+string(id), reason)
			}
		}
	} else if !reconcile.CorruptLock {
		log.Ready("DOCTOR", "doctor.plugin.lock-ready", "Plugin lock and active payload metadata verified")
	}
	manager := &pluginpkg.Manager{Store: store, RegistryClient: pluginpkg.RegistryClient{Layout: pluginpkg.DefaultLayout(), UserAgent: "chatgpt-mcp/" + version.Version}}
	desired, err := manager.AssessDesired()
	if err != nil {
		return fmt.Errorf("assess plugin desired state: %w", err)
	}
	if desired.Desired > desired.Satisfied {
		log.Detail("plugin desired state", fmt.Sprintf("%d desired, %d satisfied", desired.Desired, desired.Satisfied))
	}
	if len(desired.Missing) > 0 {
		log.Detail("plugins missing", pluginIDsCSV(desired.Missing))
	}
	if len(desired.Incompatible) > 0 {
		log.Detail("plugins incompatible", pluginIDsCSV(desired.Incompatible))
	}
	if len(desired.Pending) > 0 {
		log.Detail("plugins pending", pluginIDsCSV(desired.Pending))
	}
	if desired.LockError != "" {
		log.Detail("plugin lock status", desired.LockError)
	}
	conflicts, err := pluginpkg.CapabilityConflicts(store)
	if err != nil {
		return fmt.Errorf("inspect plugin capability conflicts: %w", err)
	}
	for _, conflict := range conflicts {
		log.Warning("DOCTOR", "doctor.plugin.capability-conflict", "Duplicate enabled plugin capability providers", nil)
		log.Detail("capability "+string(conflict.Capability), conflict.Error())
	}
	parentCtx := cmd.Context()
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	registryCtx, cancel := context.WithTimeout(parentCtx, 3*time.Second)
	registryHealth, healthErr := manager.RegistryHealth(registryCtx)
	cancel()
	if healthErr != nil {
		log.Warning("DOCTOR", "doctor.plugin.registry-error", "Plugin registry diagnostics unavailable", healthErr)
	} else {
		for _, health := range registryHealth {
			switch health.Status {
			case "verified":
				log.Detail("plugin registry "+health.Registry.Name, "signed metadata verified")
			case "verified-cache":
				log.Warning("DOCTOR", "doctor.plugin.registry-cache", "Plugin registry using verified cached metadata", nil)
				log.Detail("plugin registry "+health.Registry.Name, health.Error)
			default:
				log.Warning("DOCTOR", "doctor.plugin.registry-unavailable", "Plugin registry signature/metadata unavailable", nil)
				log.Detail("plugin registry "+health.Registry.Name, health.Error)
			}
		}
	}
	resolver := shellruntime.NewProviderResolver(store)
	if err := resolver.SetConfiguredExecutable(cfg.Shell.Executable); err != nil {
		return err
	}
	provider, err := resolver.Resolve()
	if err != nil {
		return err
	}
	log.Success("DOCTOR", "Bash shell provider ready")
	log.Detail("provider", provider.Label())
	log.Detail("executable", provider.Executable)
	if provider.Version != "" {
		log.Detail("version", string(provider.Version))
	}
	log.Verbose("DOCTOR", "doctor.shell.ready", "Bash shell provider ready", logger.WithVerbose("provider", provider.Label()), logger.WithVerbose("executable", provider.Executable))
	notifyCtx, notifyCancel := context.WithTimeout(parentCtx, time.Second)
	available := notification.PlatformProvider().Available(notifyCtx)
	notifyCancel()
	log.Verbose("DOCTOR", "doctor.notifications.inspect", "Desktop notification provider inspected", logger.WithVerbose("available", available), logger.WithVerbose("enabled", cfg.Notifications.Enabled))
	return nil
}

func pluginIDsCSV(ids []pluginpkg.PluginID) string {
	values := make([]string, len(ids))
	for index, id := range ids {
		values[index] = string(id)
	}
	return strings.Join(values, ", ")
}
