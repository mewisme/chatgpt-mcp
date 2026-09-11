package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"go.mewis.me/chatgpt-mcp/internal/app"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/mcp"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func mcpCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "mcp", Short: "Serve ChatGPT MCP transports"}
	cmd.AddCommand(mcpStdioCommand(), legacyMCPServerCommand())
	return cmd
}

func mcpStdioCommand() *cobra.Command {
	var workspace string
	cmd := &cobra.Command{
		Use:   "stdio",
		Short: "Serve MCP over stdin/stdout for local MCP clients",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMCPStdio(cmd, workspace)
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "bind this MCP session to a registered workspace ID or path")
	markMachineOutput(cmd, "always")
	return cmd
}

func runMCPStdio(cmd *cobra.Command, workspace string) (runErr error) {
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
	workspaceID, err := resolveMCPWorkspace(runtime.Tools.Workspaces, workspace)
	if err != nil {
		return err
	}
	stdio, err := mcp.NewStdioRuntimeWithWorkspace(runtime.Tools, readCloser{cmd.InOrStdin()}, writeCloser{cmd.OutOrStdout()}, workspaceID)
	if err != nil {
		return err
	}
	err = stdio.Run(cmd.Context())
	if errors.Is(err, context.Canceled) || errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

func resolveMCPWorkspace(manager interface {
	Get(string) (workspace.Workspace, error)
	List() ([]workspace.Workspace, error)
}, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if strings.HasPrefix(value, "ws_") {
		item, err := manager.Get(value)
		if err != nil {
			return "", fmt.Errorf("resolve MCP workspace %q: %w", value, err)
		}
		return item.ID, nil
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("resolve MCP workspace path: %w", err)
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve MCP workspace path %q: %w", value, err)
	}
	canonical = filepath.Clean(canonical)
	items, err := manager.List()
	if err != nil {
		return "", err
	}
	for _, item := range items {
		if filepath.Clean(item.Path) == canonical {
			return item.ID, nil
		}
	}
	return "", fmt.Errorf("workspace path is not registered: %s", canonical)
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
