package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/logger"
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
	resolver := shellruntime.DefaultProviderResolver()
	if err := resolver.SetConfiguredExecutable(cfg.Shell.Executable); err != nil {
		return err
	}
	provider, err := resolver.Resolve()
	if err != nil {
		return err
	}
	log := commandLogger(cmd)
	log.Success("DOCTOR", "Bash shell provider ready")
	log.Detail("provider", provider.Label())
	log.Detail("executable", provider.Executable)
	if provider.Version != "" {
		log.Detail("version", string(provider.Version))
	}
	log.Verbose("DOCTOR", "doctor.shell.ready", "Bash shell provider ready", logger.WithVerbose("provider", provider.Label()), logger.WithVerbose("executable", provider.Executable))
	return nil
}
