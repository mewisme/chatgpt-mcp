package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	mcpoauth "go.mewis.me/chatgpt-mcp/internal/oauth"
)

func mcpServerAuthCommand() *cobra.Command {
	command := &cobra.Command{Use: "auth", Short: "Manage OAuth authorization for an upstream MCP server"}
	command.AddCommand(mcpServerAuthLoginCommand(), mcpServerAuthStatusCommand(), mcpServerAuthLogoutCommand())
	return command
}

func mcpServerAuthLoginCommand() *cobra.Command {
	var issuer, clientID, clientSecretEnv, clientMetadataURL, extraScope string
	var noOpen bool
	var timeout time.Duration
	command := &cobra.Command{
		Use:   "login <id>",
		Short: "Authorize an HTTP MCP server with OAuth",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := loadUpstreamManager()
			if err != nil {
				return err
			}
			server, ok := manager.Get(args[0])
			if !ok {
				return fmt.Errorf("unknown upstream server: %s", args[0])
			}
			if server.Transport != "http" {
				return fmt.Errorf("OAuth login requires an HTTP upstream server")
			}
			if server.Auth.Type == "none" {
				return fmt.Errorf("OAuth is disabled for %s; configure --auth oauth or --auth auto first", server.ID)
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()
			store := mcpoauth.NewStore(mcpoauth.Path())
			log := logger.NewCLIWithWriter(cmd.OutOrStdout())
			credential, err := store.Login(ctx, mcpoauth.LoginConfig{
				ServerID: server.ID, ServerURL: server.URL, Scope: server.Auth.Scope, Issuer: issuer,
				ClientID: clientID, ClientSecretEnvVar: clientSecretEnv, ClientMetadataURL: clientMetadataURL,
			}, mcpoauth.LoginOptions{ExtraScope: extraScope, OnURL: func(raw string) error {
				log.Info("OAUTH", "authorization required")
				log.Detail("url", raw)
				if !noOpen {
					if err := openBrowser(raw); err != nil {
						log.Warn("OAUTH", "could not open browser; use the URL above", "error", err)
					}
				}
				return nil
			}})
			if err != nil {
				return err
			}
			log.Success("OAUTH", "authorization stored", "id", server.ID)
			log.Detail("issuer", credential.Issuer)
			log.Detail("registration", credential.Registration)
			log.Detail("scopes", strings.Join(credential.Scopes, " "))
			if !credential.ExpiresAt.IsZero() {
				log.Detail("expires", credential.ExpiresAt.Format(time.RFC3339))
			}
			status := manager.CheckHealth(ctx, server.ID, true)
			if status.Health != "connected" {
				log.Warn("MCP", "OAuth completed but upstream health check did not connect", "error", status.LastError)
			}
			return nil
		},
	}
	command.Flags().StringVar(&issuer, "issuer", "", "authorization server issuer when the resource advertises multiple issuers")
	command.Flags().StringVar(&clientID, "client-id", "", "pre-registered OAuth client ID")
	command.Flags().StringVar(&clientSecretEnv, "client-secret-env", "", "environment variable containing a pre-registered client secret")
	command.Flags().StringVar(&clientMetadataURL, "client-metadata-url", "", "Client ID Metadata Document URL")
	command.Flags().StringVar(&extraScope, "scope", "", "additional OAuth scopes for this login")
	command.Flags().BoolVar(&noOpen, "no-open", false, "print the authorization URL without opening a browser")
	command.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "OAuth login timeout")
	return command
}

func mcpServerAuthStatusCommand() *cobra.Command {
	var asJSON bool
	command := &cobra.Command{
		Use:   "status <id>",
		Short: "Show OAuth authorization status without secrets",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := loadUpstreamManager()
			if err != nil {
				return err
			}
			if _, ok := manager.Get(args[0]); !ok {
				return fmt.Errorf("unknown upstream server: %s", args[0])
			}
			status, err := mcpoauth.NewStore(mcpoauth.Path()).Status(args[0])
			if err != nil {
				return err
			}
			if asJSON {
				return printJSON(cmd, status)
			}
			log := logger.NewCLIWithWriter(cmd.OutOrStdout())
			if !status.Configured {
				log.Info("OAUTH", "not authorized", "id", args[0])
				return nil
			}
			log.Success("OAUTH", "authorization configured", "id", args[0])
			log.Detail("issuer", status.Issuer)
			log.Detail("registration", status.Registration)
			log.Detail("scopes", strings.Join(status.Scopes, " "))
			log.Detail("refresh", status.HasRefreshToken)
			if status.ExpiresAt != nil {
				log.Detail("expires", status.ExpiresAt.Format(time.RFC3339))
			}
			return nil
		},
	}
	command.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	return command
}

func mcpServerAuthLogoutCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "logout <id>",
		Short: "Delete stored OAuth credentials for an upstream MCP server",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := loadUpstreamManager()
			if err != nil {
				return err
			}
			if _, ok := manager.Get(args[0]); !ok {
				return fmt.Errorf("unknown upstream server: %s", args[0])
			}
			if err := mcpoauth.NewStore(mcpoauth.Path()).Delete(args[0]); err != nil {
				return err
			}
			logger.NewCLIWithWriter(cmd.OutOrStdout()).Success("OAUTH", "authorization removed", "id", args[0])
			return nil
		},
	}
}

func openBrowser(raw string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		command = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", raw)
	case "darwin":
		command = exec.Command("open", raw)
	default:
		if os.Getenv("WSL_DISTRO_NAME") != "" {
			if _, err := exec.LookPath("explorer.exe"); err == nil {
				command = exec.Command("explorer.exe", raw)
				break
			}
		}
		command = exec.Command("xdg-open", raw)
	}
	if command == nil {
		return fmt.Errorf("no browser opener available")
	}
	if err := command.Start(); err != nil {
		return err
	}
	return nil
}
