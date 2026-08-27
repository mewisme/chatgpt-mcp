package cli

import (
	"encoding/json"
	"strings"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/logger"
)

func configCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "config"}
	cmd.AddCommand(&cobra.Command{Use: "path", Run: func(cmd *cobra.Command, args []string) {
		logger.NewCLIWithWriter(cmd.OutOrStdout()).Detail("config", config.DefaultPath())
	}})
	cmd.AddCommand(&cobra.Command{Use: "get", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := config.DefaultStore().Load()
		if err != nil {
			return err
		}
		data, _ := json.MarshalIndent(value, "", "  ")
		cmd.Println(string(data))
		return nil
	}})
	cmd.AddCommand(&cobra.Command{Use: "set key=value", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		parts := strings.SplitN(args[0], "=", 2)
		value, err := config.DefaultStore().Load()
		if err != nil {
			return err
		}
		value[parts[0]] = parts[1]
		if err := config.DefaultStore().Save(value); err != nil {
			return err
		}
		logger.NewCLIWithWriter(cmd.OutOrStdout()).Success("CONFIG", "value saved", "key", parts[0])
		return nil
	}})
	return cmd
}
