package cli

import "github.com/spf13/cobra"

func mcpCommand() *cobra.Command {
	return &cobra.Command{Use: "mcp", Short: "Manage MCP configuration"}
}
