package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func mcpCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "mcp"}
	server := &cobra.Command{Use: "server"}
	server.AddCommand(&cobra.Command{Use: "list", Run: func(cmd *cobra.Command, args []string) { fmt.Println("[]") }})
	server.AddCommand(&cobra.Command{Use: "add", Run: func(cmd *cobra.Command, args []string) { fmt.Println("added") }})
	server.AddCommand(&cobra.Command{Use: "remove", Run: func(cmd *cobra.Command, args []string) { fmt.Println("removed") }})
	cmd.AddCommand(server)
	return cmd
}
