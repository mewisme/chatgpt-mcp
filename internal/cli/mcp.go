package cli

import (
	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/logger"
)

func mcpCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "mcp"}
	server := &cobra.Command{Use: "server"}
	server.AddCommand(&cobra.Command{Use: "list", Run: func(cmd *cobra.Command, args []string) {
		logger.NewCLIWithWriter(cmd.OutOrStdout()).Warn("MCP", "server list is not implemented yet")
	}})
	server.AddCommand(&cobra.Command{Use: "add", Run: func(cmd *cobra.Command, args []string) {
		logger.NewCLIWithWriter(cmd.OutOrStdout()).Warn("MCP", "server add is not implemented yet")
	}})
	server.AddCommand(&cobra.Command{Use: "remove", Run: func(cmd *cobra.Command, args []string) {
		logger.NewCLIWithWriter(cmd.OutOrStdout()).Warn("MCP", "server remove is not implemented yet")
	}})
	cmd.AddCommand(server)
	return cmd
}
