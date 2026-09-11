package cli

import (
	"github.com/spf13/cobra"
)

func mcpCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "mcp", Short: "Serve ChatGPT MCP transports"}
	cmd.AddCommand(legacyMCPServerCommand())
	return cmd
}

func legacyMCPServerCommand() *cobra.Command {
	server := upstreamServerCommand()
	server.Deprecated = "use 'cgm upstream server' instead"
	server.Short = "Deprecated: manage upstream MCP servers"
	return server
}
