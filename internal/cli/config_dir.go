package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
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
	if err := configureConfigDir(cmd); err != nil {
		return err
	}
	return validateLoggingFlags(cmd, args)
}
