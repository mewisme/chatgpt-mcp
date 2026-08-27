package cli

import (
	"fmt"
	"github.com/spf13/cobra"
)

func serveCommand() *cobra.Command {
	return &cobra.Command{Use: "serve", Run: func(cmd *cobra.Command, args []string) { fmt.Println("server start") }}
}
