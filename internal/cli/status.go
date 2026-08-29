package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/logger"
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
			_, configErr := os.Stat(config.Path())
			initialized := configErr == nil
			log := logger.NewCLIWithWriter(cmd.OutOrStdout())
			log.Info("STATUS", "local runtime configuration")
			log.Detail("initialized", initialized)
			log.Detail("config", config.Path())
			logEndpointDetails(log, cfg)
			log.Detail("auth", fmt.Sprintf("mcp=%t admin=%t", cfg.Auth.MCPEnabled, cfg.Auth.AdminEnabled))
			log.Detail("workspaces", len(workspaces))
			log.Detail("upstreams", len(upstreams.List()))
			return nil
		},
	}
}
