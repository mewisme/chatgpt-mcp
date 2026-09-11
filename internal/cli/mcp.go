package cli

import (
	"context"
	"errors"
	"io"

	"github.com/spf13/cobra"

	"go.mewis.me/chatgpt-mcp/internal/app"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/mcp"
)

func mcpCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "mcp", Short: "Serve ChatGPT MCP transports"}
	cmd.AddCommand(mcpStdioCommand(), legacyMCPServerCommand())
	return cmd
}

func mcpStdioCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stdio",
		Short: "Serve MCP over stdin/stdout for local MCP clients",
		Args:  cobra.NoArgs,
		RunE:  runMCPStdio,
	}
	markMachineOutput(cmd, "always")
	return cmd
}

func runMCPStdio(cmd *cobra.Command, _ []string) (runErr error) {
	if cmd == nil {
		return errors.New("stdio command is unavailable")
	}
	source, err := config.Source()
	if err != nil {
		return err
	}
	if !source.Exists {
		return errors.New("chatgpt-mcp is not initialized; run chatgpt-mcp init")
	}
	cfg, err := config.LoadRuntime()
	if err != nil {
		return err
	}
	if err := config.Validate(cfg); err != nil {
		return err
	}
	cfg.Server.Enabled = false
	cfg.Admin.Enabled = false
	cfg.Tunnel.Enabled = false
	runtime, err := app.NewWithLoggerContext(cmd.Context(), cfg, commandLogger(cmd))
	if err != nil {
		return err
	}
	if err := runtime.Start(cmd.Context()); err != nil {
		return err
	}
	defer func() {
		if err := runtime.Stop(); err != nil && runErr == nil {
			runErr = err
		}
	}()
	stdio, err := mcp.NewStdioRuntime(runtime.Tools, readCloser{cmd.InOrStdin()}, writeCloser{cmd.OutOrStdout()})
	if err != nil {
		return err
	}
	err = stdio.Run(cmd.Context())
	if errors.Is(err, context.Canceled) || errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

type readCloser struct{ io.Reader }

func (readCloser) Close() error { return nil }

type writeCloser struct{ io.Writer }

func (writeCloser) Close() error { return nil }

func legacyMCPServerCommand() *cobra.Command {
	server := upstreamServerCommand()
	server.Deprecated = "use 'cgm upstream server' instead"
	server.Short = "Deprecated: manage upstream MCP servers"
	return server
}
