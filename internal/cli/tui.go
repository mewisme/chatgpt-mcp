package cli

import (
	"github.com/spf13/cobra"
	commandtui "go.mewis.me/chatgpt-mcp/internal/tui"
)

func tuiCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "tui [path...]",
		Short: "Open the full-screen ChatGPT MCP command center",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			route, err := commandtui.ParseRoute(args)
			if err != nil {
				return err
			}
			return commandtui.Run(cmd.Context(), route, cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
}
