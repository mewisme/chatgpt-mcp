package cli

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/auth"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/version"
)

var root = &cobra.Command{Use: "chatgpt-mcp", RunE: runServer, Version: version.Short(), SilenceErrors: true, SilenceUsage: true}

func init() {
	root.AddCommand(&cobra.Command{Use: "uninit", Short: "Remove local configuration", RunE: func(cmd *cobra.Command, args []string) error {
		if err := os.Remove(config.Path()); err != nil && !os.IsNotExist(err) {
			return err
		}
		log := logger.NewCLIWithWriter(cmd.OutOrStdout())
		log.Success("UNINIT", "configuration removed")
		log.Detail("config", config.Path())
		return nil
	}})
	root.AddCommand(&cobra.Command{Use: "init", Short: "Initialize local configuration and tokens", RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.Default()
		mcpToken := auth.GenerateToken("mcp")
		adminToken := auth.GenerateToken("admin")
		cfg.Auth.MCPTokenHash = auth.HashToken(mcpToken)
		cfg.Auth.AdminTokenHash = auth.HashToken(adminToken)
		if err := config.Save(cfg); err != nil {
			return err
		}
		log := logger.NewCLIWithWriter(cmd.OutOrStdout())
		log.Success("INIT", "configuration created")
		log.Detail("config", config.Path())
		log.Detail("mcp", mcpToken)
		log.Detail("admin", adminToken)
		return nil
	}})
	root.AddCommand(&cobra.Command{Use: "config-path", Run: func(cmd *cobra.Command, args []string) {
		logger.NewCLIWithWriter(cmd.OutOrStdout()).Detail("config", config.Path())
	}})
	root.AddCommand(&cobra.Command{Use: "version", Run: func(cmd *cobra.Command, args []string) {
		logger.NewCLIWithWriter(cmd.OutOrStdout()).Info("VERSION", version.String())
	}})
	authCmd := &cobra.Command{Use: "auth"}
	authCmd.AddCommand(authCreateCommand("mcp"), authCreateCommand("admin"))
	root.AddCommand(authCmd, serveCommand(), configCommand(), mcpCommand(), tunnelCommand())
}

func authCreateCommand(kind string) *cobra.Command {
	return &cobra.Command{Use: kind + "-create", RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		token := auth.GenerateToken(kind)
		hash := auth.HashToken(token)
		if kind == "mcp" {
			cfg.Auth.MCPTokenHash = hash
			cfg.Auth.MCPEnabled = true
		} else {
			cfg.Auth.AdminTokenHash = hash
			cfg.Auth.AdminEnabled = true
		}
		if err := config.Save(cfg); err != nil {
			return err
		}
		log := logger.NewCLIWithWriter(cmd.OutOrStdout())
		log.Success("AUTH", "token rotated", "type", kind)
		log.Detail(strings.ToUpper(kind), token)
		return nil
	}}
}

func Execute() error { return root.Execute() }
