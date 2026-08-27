package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/auth"
	"go.mewis.me/chatgpt-mcp/internal/config"
)

var root = &cobra.Command{Use: "chatgpt-mcp", RunE: runServer}

func init() {
	root.AddCommand(&cobra.Command{Use: "init", RunE: func(cmd *cobra.Command, args []string) error {
		cfg := config.Default()
		mcpToken := auth.GenerateToken("mcp")
		adminToken := auth.GenerateToken("admin")
		cfg.Auth.MCPTokenHash = auth.HashToken(mcpToken)
		cfg.Auth.AdminTokenHash = auth.HashToken(adminToken)
		if err := config.Save(cfg); err != nil {
			return err
		}
		fmt.Printf("config: %s\nmcp token: %s\nadmin token: %s\n", config.Path(), mcpToken, adminToken)
		return nil
	}})
	root.AddCommand(&cobra.Command{Use: "config-path", Run: func(cmd *cobra.Command, args []string) { fmt.Println(config.Path()) }})

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
		fmt.Println(token)
		return nil
	}}
}

func Execute() {
	if err := root.Execute(); err != nil {
		panic(err)
	}
}
