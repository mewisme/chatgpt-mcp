package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	shellruntime "go.mewis.me/chatgpt-mcp/internal/shell"
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
	}
	desired, err := (&pluginpkg.Manager{Store: store}).AssessDesired()
	if err != nil {
		return fmt.Errorf("assess plugin desired state: %w", err)
	}
	if desired.Desired > desired.Satisfied {
		log.Detail("plugin desired state", fmt.Sprintf("%d desired, %d satisfied", desired.Desired, desired.Satisfied))
	}
	if desired.LockError != "" {
		log.Detail("plugin lock status", desired.LockError)
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
	return nil
}
