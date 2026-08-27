package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func mcpCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "mcp"}
	cmd.AddCommand(&cobra.Command{Use: "server-list", Run: func(cmd *cobra.Command, args []string) { fmt.Println("[]") }})
	return cmd
}
