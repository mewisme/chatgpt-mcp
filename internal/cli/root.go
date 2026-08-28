package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/auth"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/version"
)

var root = newRootCommand()

func newRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "chatgpt-mcp",
		Short:         "Workspace-bound local MCP server for ChatGPT",
		RunE:          runServer,
		Version:       version.Short(),
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	cmd.AddCommand(
		initCommand(),
		uninitCommand(),
		configCommand(),
		authCommand(),
		workspaceCommand(),
		mcpCommand(),
		tunnelCommand(),
		serveCommand(),
		statusCommand(),
		&cobra.Command{Use: "config-path", Hidden: true, Run: func(cmd *cobra.Command, args []string) {
			logger.NewCLIWithWriter(cmd.OutOrStdout()).Detail("config", config.Path())
		}},
		&cobra.Command{Use: "version", Run: func(cmd *cobra.Command, args []string) {
			logger.NewCLIWithWriter(cmd.OutOrStdout()).Info("VERSION", version.String())
		}},
	)
	return cmd
}

func initCommand() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize configuration and authentication tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, err := os.Stat(config.Path()); err == nil && !force {
				return errors.New("configuration already exists; use --force to rotate tokens and rewrite config")
			} else if err != nil && !os.IsNotExist(err) {
				return err
			}
			cfg := config.Default()
			mcpToken := auth.GenerateToken("mcp")
			adminToken := auth.GenerateToken("admin")
			cfg.Auth.MCPTokenHash = auth.HashToken(mcpToken)
			cfg.Auth.AdminTokenHash = auth.HashToken(adminToken)
			if err := config.Validate(cfg); err != nil {
				return err
			}
			if err := config.Save(cfg); err != nil {
				return err
			}
			log := logger.NewCLIWithWriter(cmd.OutOrStdout())
			log.Success("INIT", "configuration created")
			log.Detail("config", config.Path())
			log.Detail("mcp url", fmt.Sprintf("http://%s:%d/mcp", cfg.Server.Host, cfg.Server.Port))
			log.Detail("admin url", fmt.Sprintf("http://%s:%d/", cfg.Server.Host, cfg.Admin.Port))
			log.Detail("mcp token", mcpToken)
			log.Detail("admin token", adminToken)
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "rewrite config and rotate both tokens if already initialized")
	return cmd
}

func uninitCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "uninit",
		Short: "Remove all local chatgpt-mcp configuration and state",
		RunE: func(cmd *cobra.Command, args []string) error {
			root := config.RootPath()
			if err := removeConfigRoot(root); err != nil {
				return err
			}
			log := logger.NewCLIWithWriter(cmd.OutOrStdout())
			log.Success("UNINIT", "local configuration and state removed")
			log.Detail("root", root)
			return nil
		},
	}
}

func removeConfigRoot(root string) error {
	clean := filepath.Clean(root)
	if clean == "." || clean == string(filepath.Separator) || filepath.Base(clean) != "chatgpt-mcp" {
		return fmt.Errorf("refusing to remove unsafe config root: %s", clean)
	}
	volume := filepath.VolumeName(clean)
	if clean == volume+string(filepath.Separator) {
		return fmt.Errorf("refusing to remove volume root: %s", clean)
	}
	return os.RemoveAll(clean)
}

func authCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "auth", Short: "Manage MCP and admin authentication"}
	cmd.AddCommand(
		authCreateCommand("mcp"),
		authCreateCommand("admin"),
		authToggleCommand("mcp", true),
		authToggleCommand("mcp", false),
		authToggleCommand("admin", true),
		authToggleCommand("admin", false),
		authStatusCommand(),
	)
	return cmd
}

func authCreateCommand(kind string) *cobra.Command {
	return &cobra.Command{
		Use:   kind + "-create",
		Short: "Create or rotate the " + kind + " token",
		RunE: func(cmd *cobra.Command, args []string) error {
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
				cfg.Admin.Enabled = true
			}
			if err := config.Validate(cfg); err != nil {
				return err
			}
			if err := config.Save(cfg); err != nil {
				return err
			}
			log := logger.NewCLIWithWriter(cmd.OutOrStdout())
			log.Success("AUTH", "token rotated", "type", kind)
			log.Detail(strings.ToUpper(kind), token)
			return nil
		},
	}
}

func authToggleCommand(kind string, enabled bool) *cobra.Command {
	action := "disable"
	if enabled {
		action = "enable"
	}
	return &cobra.Command{
		Use:   kind + "-" + action,
		Short: action + " " + kind + " authentication",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if kind == "mcp" {
				if enabled && cfg.Auth.MCPTokenHash == "" {
					return errors.New("MCP token is not configured; run chatgpt-mcp auth mcp-create")
				}
				cfg.Auth.MCPEnabled = enabled
			} else {
				if enabled && cfg.Auth.AdminTokenHash == "" {
					return errors.New("admin token is not configured; run chatgpt-mcp auth admin-create")
				}
				cfg.Auth.AdminEnabled = enabled
			}
			if err := config.Validate(cfg); err != nil {
				return err
			}
			if err := config.Save(cfg); err != nil {
				return err
			}
			state := "disabled"
			if enabled {
				state = "enabled"
			}
			logger.NewCLIWithWriter(cmd.OutOrStdout()).Success("AUTH", state, "type", kind)
			return nil
		},
	}
}

func authStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show authentication state without revealing token hashes",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			log := logger.NewCLIWithWriter(cmd.OutOrStdout())
			log.Info("AUTH", "authentication status")
			log.Detail("mcp", fmt.Sprintf("enabled=%t configured=%t", cfg.Auth.MCPEnabled, cfg.Auth.MCPTokenHash != ""))
			log.Detail("admin", fmt.Sprintf("enabled=%t configured=%t", cfg.Auth.AdminEnabled, cfg.Auth.AdminTokenHash != ""))
			return nil
		},
	}
}

func Execute() error { return root.Execute() }
