package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/config"
)

func configCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "config"}
	cmd.AddCommand(&cobra.Command{Use: "path", Run: func(cmd *cobra.Command, args []string) { fmt.Println(config.DefaultPath()) }})
	cmd.AddCommand(&cobra.Command{Use: "get", RunE: func(cmd *cobra.Command, args []string) error {
		value, err := config.DefaultStore().Load()
		if err != nil {
			return err
		}
		data, _ := json.MarshalIndent(value, "", "  ")
		fmt.Println(string(data))
		return nil
	}})
	cmd.AddCommand(&cobra.Command{Use: "set key=value", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		parts := strings.SplitN(args[0], "=", 2)
		value, err := config.DefaultStore().Load()
		if err != nil {
			return err
		}
		value[parts[0]] = parts[1]
		return config.DefaultStore().Save(value)
	}})
	return cmd
}
