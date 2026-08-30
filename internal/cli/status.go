package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func statusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show local configuration, endpoints, workspaces, and upstream count",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			workspaces, err := workspace.NewManager(workspace.DefaultStorePath()).List()
			if err != nil {
				return err
			}
			upstreams := upstream.NewManager(upstream.NewStore(upstream.Path()))
			if err := upstreams.Load(); err != nil {
				return err
			}
			source, err := config.Source()
			if err != nil {
				return err
			}
			log := commandLogger(cmd)
			log.Info("STATUS", "local runtime configuration")
			log.Detail("initialized", source.Exists)
			log.Detail("config", source.Path)
			log.Detail("format", source.Format)
			logEndpointDetails(log, cfg)
			log.Detail("auth", fmt.Sprintf("mcp=%t admin=%t", cfg.Auth.MCPEnabled, cfg.Auth.AdminEnabled))
			log.Detail("workspaces", len(workspaces))
			log.Detail("upstreams", len(upstreams.List()))
			return nil
		},
	}
}
