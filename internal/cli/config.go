package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/config"
)

func configCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "config"}
	cmd.AddCommand(&cobra.Command{Use: "path", Run: func(cmd *cobra.Command, args []string) { fmt.Println(config.DefaultPath()) }})
	cmd.AddCommand(&cobra.Command{Use: "get", Run: func(cmd *cobra.Command, args []string) {
		value, _ := config.DefaultStore().Load()
		data, _ := json.MarshalIndent(value, "", "  ")
		fmt.Println(string(data))
	}})
	return cmd
}
