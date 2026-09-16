package cli

import (
	"errors"
	"io"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"go.mewis.me/chatgpt-mcp/internal/application"
)

func tuiCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "tui [path...]",
		Short: "Open the full-screen ChatGPT MCP command center",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			logCommandStep(cmd, "TUI", "tui.route.parsing", "Resolving command center route")
			if !tuiIsTerminal(cmd.InOrStdin(), cmd.OutOrStdout()) {
				return errors.New("cgm tui requires terminal stdin and stdout")
			}
			logCommandStep(cmd, "TUI", "tui.starting", "Starting command center")
			return application.RunTerminalUI(cmd.Context(), args, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}

var tuiIsTerminal = func(in io.Reader, out io.Writer) bool {
	input, inputOK := in.(*os.File)
	output, outputOK := out.(*os.File)
	return inputOK && outputOK && term.IsTerminal(int(input.Fd())) && term.IsTerminal(int(output.Fd()))
}
