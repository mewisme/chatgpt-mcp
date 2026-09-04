package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/controlplane"
)

var processCommandArgs = func() []string { return append([]string(nil), os.Args[1:]...) }

func addConfigDirFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().String("config-dir", "", fmt.Sprintf("config/state directory (env: %s)", configformat.EnvConfigDir))
}

func configureConfigDir(cmd *cobra.Command) error {
	value, err := cmd.Root().PersistentFlags().GetString("config-dir")
	if err != nil {
		return err
	}
	if strings.TrimSpace(value) == "" {
		value = os.Getenv(configformat.EnvConfigDir)
	}
	return configformat.SetRootPath(value)
}

func prepareCommand(cmd *cobra.Command, args []string) error {
	if controlplane.ToolContextActive() && !controlplane.IsReadOnlyPath(relativeCommandPath(cmd)) {
		if err := verifyControlApproval(cmd.Context(), cmd.CommandPath(), processCommandArgs()); err != nil {
			return err
		}
	}
	if err := configureConfigDir(cmd); err != nil {
		return err
	}
	return validateLoggingFlags(cmd, args)
}

func verifyControlApproval(ctx context.Context, commandPath string, actualArgs []string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	capability := strings.TrimSpace(os.Getenv(controlplane.ControlApprovalEnv))
	if capability == "" || !controlplane.ApprovalEligibleArgs(actualArgs) {
		return fmt.Errorf("control-plane command denied from MCP tool execution context: %s", commandPath)
	}
	if err := requestRuntimeCLIApproval(ctx, capability, actualArgs); err != nil {
		return fmt.Errorf("control-plane command denied from MCP tool execution context: %s: approval verification failed: %w", commandPath, err)
	}
	return nil
}

func relativeCommandPath(cmd *cobra.Command) string {
	return strings.TrimSpace(strings.TrimPrefix(cmd.CommandPath(), cmd.Root().Name()))
}
