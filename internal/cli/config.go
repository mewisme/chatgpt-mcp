package cli

import (
	"fmt"
	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/config"
)

func configCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "config"}
	cmd.AddCommand(&cobra.Command{Use: "path", RunE: func(cmd *cobra.Command, args []string) error { fmt.Println(config.DefaultPath()); return nil }})
	return cmd
}
