package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/controlplane"
)

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
		return fmt.Errorf("control-plane command denied from MCP tool execution context: %s", cmd.CommandPath())
	}
	if err := configureConfigDir(cmd); err != nil {
		return err
	}
	return validateLoggingFlags(cmd, args)
}

func relativeCommandPath(cmd *cobra.Command) string {
	return strings.TrimSpace(strings.TrimPrefix(cmd.CommandPath(), cmd.Root().Name()))
}
