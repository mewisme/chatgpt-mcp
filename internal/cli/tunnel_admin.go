package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/cli/interactive"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

const tunnelAdminTimeout = 30 * time.Second

type tunnelAdminScopeFlags struct {
	organizationID string
	workspaceID    string
	tenantID       string
}

func (flags *tunnelAdminScopeFlags) add(cmd *cobra.Command) {
	cmd.Flags().StringVar(&flags.organizationID, "organization-id", "", "OpenAI organization scope")
	cmd.Flags().StringVar(&flags.workspaceID, "workspace-id", "", "OpenAI workspace scope")
	cmd.Flags().StringVar(&flags.tenantID, "tenant-id", "", "OpenAI tenant scope")
}

func (flags tunnelAdminScopeFlags) changed(cmd *cobra.Command) bool {
	return cmd.Flags().Changed("organization-id") || cmd.Flags().Changed("workspace-id") || cmd.Flags().Changed("tenant-id")
}

func (flags tunnelAdminScopeFlags) scope() tunnel.AdminScope {
	return tunnel.AdminScope{OrganizationID: strings.TrimSpace(flags.organizationID), WorkspaceID: strings.TrimSpace(flags.workspaceID), TenantID: strings.TrimSpace(flags.tenantID)}
}

func resolveTunnelAdminScope(cmd *cobra.Command, cfg tunnel.Config, flags tunnelAdminScopeFlags) (tunnel.AdminScope, error) {
	scope := tunnel.AdminScopeFromConfig(cfg)
	if flags.changed(cmd) {
		scope = flags.scope()
	}
	if err := tunnel.ValidateAdminScope(scope); err != nil {
		return tunnel.AdminScope{}, err
	}
	return scope, nil
}

func resolveTunnelAdminSetScope(ctx context.Context, cmd *cobra.Command, cfg tunnel.Config, flags tunnelAdminScopeFlags) (tunnel.AdminScope, error) {
	if flags.changed(cmd) {
		scope := flags.scope()
		return scope, tunnel.ValidateAdminScope(scope)
	}
	if scope := tunnel.AdminScopeFromConfig(cfg); tunnel.ValidateAdminScope(scope) == nil {
		return scope, nil
	}
	if strings.TrimSpace(cfg.ID) == "" {
		return tunnel.AdminScope{}, errors.New("provide exactly one admin scope or configure a tunnel first")
	}
	metadata, err := tunnel.GetManaged(ctx, cfg, cfg.ID)
	if err != nil {
		return tunnel.AdminScope{}, fmt.Errorf("derive admin scope from configured tunnel: %w", err)
	}
	for _, candidate := range []tunnel.AdminScope{
		{OrganizationID: singleTunnelID(metadata.OrganizationIDs)},
		{WorkspaceID: singleTunnelID(metadata.WorkspaceIDs)},
		{TenantID: singleTunnelID(metadata.TenantIDs)},
	} {
		if tunnel.ValidateAdminScope(candidate) == nil {
			return candidate, nil
		}
	}
	return tunnel.AdminScope{}, errors.New("configured tunnel does not expose one unambiguous admin scope; provide --organization-id, --workspace-id, or --tenant-id")
}

func singleTunnelID(values []string) string {
	if len(values) == 1 {
		return strings.TrimSpace(values[0])
	}
	return ""
}

func tunnelAdminCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "admin", Short: "Manage OpenAI tunnel administration"}
	cmd.AddCommand(tunnelAdminKeyCommand())
	return cmd
}

func tunnelAdminKeyCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "key", Short: "Manage the stored OpenAI tunnel admin key"}
	cmd.AddCommand(tunnelAdminKeySetCommand(), tunnelAdminKeyStatusCommand(), tunnelAdminKeyVerifyCommand(), tunnelAdminKeyRemoveCommand())
	return cmd
}

func tunnelAdminKeySetCommand() *cobra.Command {
	var adminKey string
	var scopeFlags tunnelAdminScopeFlags
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Verify and store an OpenAI tunnel admin key",
		Long:  "Verify Tunnels Manage access by listing an organization, workspace, or tenant scope, then store the admin key in the secret file store and verification scope in tunnel.<ext>. If no scope flag is provided, cgm first reuses a stored scope or derives one from the currently configured tunnel metadata.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadForTunnelAdminKeyReplacement()
			if err != nil {
				return err
			}
			key := strings.TrimSpace(adminKey)
			if key == "" {
				key = strings.TrimSpace(os.Getenv("OPENAI_ADMIN_KEY"))
			}
			if key == "" {
				return errors.New("OpenAI admin key is required; use --admin-key or OPENAI_ADMIN_KEY")
			}
			candidate := cfg.Tunnel
			candidate.AdminKey = key
			ctx, cancel := context.WithTimeout(cmd.Context(), tunnelAdminTimeout)
			defer cancel()
			scope, err := resolveTunnelAdminSetScope(ctx, cmd, candidate, scopeFlags)
			if err != nil {
				return fmt.Errorf("admin key verification scope: %w", err)
			}
			tunnel.ApplyAdminScope(&candidate, scope)
			count, err := tunnel.VerifyAdminKey(ctx, candidate)
			if err != nil {
				return fmt.Errorf("admin key verification failed: %w", err)
			}
			cfg.Tunnel = candidate
			if err := config.Save(cfg); err != nil {
				return err
			}
			log := commandLogger(cmd)
			log.Success("TUNNEL", "Admin key verified and saved")
			log.Detail("scope", formatTunnelAdminScope(scope))
			log.Detail("tunnels", count)
			log.Detail("secret store", "secret file store")
			return nil
		},
	}
	cmd.Flags().StringVar(&adminKey, "admin-key", "", "OpenAI admin API key with Tunnels Manage; defaults to OPENAI_ADMIN_KEY")
	scopeFlags.add(cmd)
	return cmd
}

func tunnelAdminKeyStatusCommand() *cobra.Command {
	return &cobra.Command{Use: "status", Aliases: []string{"st"}, Short: "Show stored tunnel admin key state without revealing the key", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		log := commandLogger(cmd)
		configured := tunnel.AdminConfigured(cfg.Tunnel)
		log.Detail("configured", configured)
		if strings.TrimSpace(cfg.Tunnel.AdminKey) != "" {
			log.Detail("key", "<redacted>")
		}
		if scope := tunnel.AdminScopeFromConfig(cfg.Tunnel); tunnel.ValidateAdminScope(scope) == nil {
			log.Detail("scope", formatTunnelAdminScope(scope))
		}
		log.Detail("secret store", "secret file store")
		return nil
	}}
}

func tunnelAdminKeyVerifyCommand() *cobra.Command {
	return &cobra.Command{Use: "verify", Short: "Re-verify the stored admin key has Tunnels Manage access", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if !tunnel.AdminConfigured(cfg.Tunnel) {
			return errors.New("tunnel admin key is not configured; run tunnel admin key set first")
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), tunnelAdminTimeout)
		defer cancel()
		count, err := tunnel.VerifyAdminKey(ctx, cfg.Tunnel)
		if err != nil {
			return fmt.Errorf("admin key verification failed: %w", err)
		}
		log := commandLogger(cmd)
		log.Success("TUNNEL", "Admin key verified")
		log.Detail("scope", formatTunnelAdminScope(tunnel.AdminScopeFromConfig(cfg.Tunnel)))
		log.Detail("tunnels", count)
		return nil
	}}
}

func tunnelAdminKeyRemoveCommand() *cobra.Command {
	return &cobra.Command{Use: "remove", Aliases: []string{"rm"}, Short: "Remove the stored tunnel admin key and verification scope", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadForTunnelAdminKeyReplacement()
		if err != nil {
			return err
		}
		cfg.Tunnel.AdminKey = ""
		tunnel.ApplyAdminScope(&cfg.Tunnel, tunnel.AdminScope{})
		if err := config.Save(cfg); err != nil {
			return err
		}
		commandLogger(cmd).Success("TUNNEL", "Admin key removed")
		return nil
	}}
}

func tunnelListCommand() *cobra.Command {
	var scopeFlags tunnelAdminScopeFlags
	var asJSON, forceInteractive, noInteractive bool
	cmd := &cobra.Command{Use: "list", Aliases: []string{"ls"}, Short: "List tunnels manageable by the stored admin key", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if strings.TrimSpace(cfg.Tunnel.AdminKey) == "" {
			return errors.New("tunnel admin key is not configured; run tunnel admin key set first")
		}
		scope, err := resolveTunnelAdminScope(cmd, cfg.Tunnel, scopeFlags)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), tunnelAdminTimeout)
		defer cancel()
		items, err := tunnel.ListManaged(ctx, cfg.Tunnel, scope)
		if err != nil {
			return err
		}
		interactiveMode, err := interactive.ResolveMode(cmd.InOrStdin(), cmd.OutOrStdout(), forceInteractive, noInteractive, asJSON)
		if err != nil {
			return err
		}
		if interactiveMode {
			refreshRows := func(parent context.Context) ([]interactive.Row, error) {
				ctx, cancel := context.WithTimeout(parent, tunnelAdminTimeout)
				defer cancel()
				items, err := tunnel.ListManaged(ctx, cfg.Tunnel, scope)
				if err != nil {
					return nil, err
				}
				return tunnelInteractiveRows(items), nil
			}
			return runInteractiveBrowser(cmd, "Managed tunnels", tunnelInteractiveRows(items), refreshRows)
		}
		return printJSON(cmd, items)
	}}
	scopeFlags.add(cmd)
	cmd.Flags().BoolVar(&asJSON, "json", false, "print JSON")
	cmd.Flags().BoolVar(&forceInteractive, "interactive", false, "force interactive tunnel list")
	cmd.Flags().BoolVar(&noInteractive, "no-interactive", false, "disable interactive tunnel list")
	return cmd
}

func tunnelGetCommand() *cobra.Command {
	var configure, enable bool
	var runtimeAPIKey string
	cmd := &cobra.Command{Use: "get <tunnel_id>", Short: "Fetch a managed tunnel by id", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if !tunnel.AdminConfigured(cfg.Tunnel) {
			return errors.New("verified tunnel admin key is required; run tunnel admin key set first")
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), tunnelAdminTimeout)
		defer cancel()
		metadata, err := tunnel.GetManaged(ctx, cfg.Tunnel, args[0])
		if err != nil {
			return err
		}
		if _, err := config.SaveTunnelMetadata(metadata); err != nil {
			return err
		}
		if configure {
			if err := configureManagedTunnel(&cfg, metadata, runtimeAPIKey, enable); err != nil {
				return err
			}
			if err := config.Save(cfg); err != nil {
				return err
			}
		}
		return printJSON(cmd, metadata)
	}}
	addManagedConfigureFlags(cmd, &configure, &runtimeAPIKey, &enable)
	return cmd
}

func tunnelCreateCommand() *cobra.Command {
	var name, description, runtimeAPIKey string
	var organizationIDs, workspaceIDs, tenantIDs []string
	var configure, enable bool
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a tunnel with the stored verified admin key",
		Long:  "Create tunnel metadata through the OpenAI Tunnel Management API. The stored admin key must already pass tunnel admin key verify. Use --configure to select the new tunnel for cgm with a separate runtime API key.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if !tunnel.AdminConfigured(cfg.Tunnel) {
				return errors.New("verified tunnel admin key is required; run tunnel admin key set first")
			}
			orgs, workspaces, tenants := normalizeTunnelIDs(organizationIDs), normalizeTunnelIDs(workspaceIDs), normalizeTunnelIDs(tenantIDs)
			if len(orgs) == 0 && len(workspaces) == 0 {
				scope := tunnel.AdminScopeFromConfig(cfg.Tunnel)
				if scope.OrganizationID != "" {
					orgs = []string{scope.OrganizationID}
				} else if scope.WorkspaceID != "" {
					workspaces = []string{scope.WorkspaceID}
				}
			}
			request := tunnel.CreateRequest{Name: strings.TrimSpace(name), Description: strings.TrimSpace(description), OrganizationIDs: orgs, WorkspaceIDs: workspaces, TenantIDs: tenants}
			ctx, cancel := context.WithTimeout(cmd.Context(), tunnelAdminTimeout)
			defer cancel()
			metadata, err := tunnel.CreateManaged(ctx, cfg.Tunnel, request)
			if err != nil {
				return err
			}
			if _, err := config.SaveTunnelMetadata(metadata); err != nil {
				return err
			}
			if configure {
				if err := configureManagedTunnel(&cfg, metadata, runtimeAPIKey, enable); err != nil {
					return err
				}
				if err := config.Save(cfg); err != nil {
					return err
				}
			}
			logManagedTunnel(cmd, "Tunnel created", metadata, configure)
			commandLogger(cmd).Detail("ready", "allow 25-30 seconds before expecting the new tunnel to be active")
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "tunnel name (required)")
	cmd.Flags().StringVar(&description, "description", "", "tunnel description (required)")
	cmd.Flags().StringSliceVar(&organizationIDs, "organization-id", nil, "OpenAI organization identifier; repeatable")
	cmd.Flags().StringSliceVar(&workspaceIDs, "workspace-id", nil, "OpenAI workspace identifier; repeatable")
	cmd.Flags().StringSliceVar(&tenantIDs, "tenant-id", nil, "OpenAI tenant identifier; repeatable")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("description")
	addManagedConfigureFlags(cmd, &configure, &runtimeAPIKey, &enable)
	return cmd
}

func tunnelUpdateCommand() *cobra.Command {
	var name, description, runtimeAPIKey string
	var organizationIDs, workspaceIDs, tenantIDs []string
	var configure, enable bool
	cmd := &cobra.Command{Use: "update <tunnel_id>", Short: "Update a tunnel with the stored verified admin key", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if !tunnel.AdminConfigured(cfg.Tunnel) {
			return errors.New("verified tunnel admin key is required; run tunnel admin key set first")
		}
		request := tunnel.UpdateRequest{}
		if cmd.Flags().Changed("name") {
			value := strings.TrimSpace(name)
			request.Name = &value
		}
		if cmd.Flags().Changed("description") {
			value := description
			request.Description = &value
		}
		if cmd.Flags().Changed("organization-id") {
			value := normalizeTunnelIDs(organizationIDs)
			request.OrganizationIDs = &value
		}
		if cmd.Flags().Changed("workspace-id") {
			value := normalizeTunnelIDs(workspaceIDs)
			request.WorkspaceIDs = &value
		}
		if cmd.Flags().Changed("tenant-id") {
			value := normalizeTunnelIDs(tenantIDs)
			request.TenantIDs = &value
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), tunnelAdminTimeout)
		defer cancel()
		metadata, err := tunnel.UpdateManaged(ctx, cfg.Tunnel, args[0], request)
		if err != nil {
			return err
		}
		if _, err := config.SaveTunnelMetadata(metadata); err != nil {
			return err
		}
		if configure {
			if err := configureManagedTunnel(&cfg, metadata, runtimeAPIKey, enable); err != nil {
				return err
			}
			if err := config.Save(cfg); err != nil {
				return err
			}
		}
		logManagedTunnel(cmd, "Tunnel updated", metadata, configure)
		return nil
	}}
	cmd.Flags().StringVar(&name, "name", "", "new tunnel name")
	cmd.Flags().StringVar(&description, "description", "", "new tunnel description")
	cmd.Flags().StringSliceVar(&organizationIDs, "organization-id", nil, "replace organization identifiers; repeatable")
	cmd.Flags().StringSliceVar(&workspaceIDs, "workspace-id", nil, "replace workspace identifiers; repeatable")
	cmd.Flags().StringSliceVar(&tenantIDs, "tenant-id", nil, "replace tenant identifiers; repeatable")
	addManagedConfigureFlags(cmd, &configure, &runtimeAPIKey, &enable)
	return cmd
}

func tunnelDeleteCommand() *cobra.Command {
	var confirm, clearConfig bool
	cmd := &cobra.Command{Use: "delete <tunnel_id>", Short: "Delete a tunnel with the stored verified admin key", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if !confirm {
			return errors.New("refusing to delete tunnel without --confirm")
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if !tunnel.AdminConfigured(cfg.Tunnel) {
			return errors.New("verified tunnel admin key is required; run tunnel admin key set first")
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), tunnelAdminTimeout)
		defer cancel()
		metadata, err := tunnel.DeleteManaged(ctx, cfg.Tunnel, args[0])
		if err != nil {
			return err
		}
		cleared := clearConfig && cfg.Tunnel.ID == metadata.ID
		if cleared {
			cfg.Tunnel.Enabled = false
			cfg.Tunnel.ID = ""
			cfg.Tunnel.APIKey = ""
			cfg.Tunnel.OrganizationID = ""
			if err := config.Save(cfg); err != nil {
				return err
			}
		}
		if err := config.RemoveTunnelMetadata(metadata.ID); err != nil {
			return err
		}
		logManagedTunnel(cmd, "Tunnel deleted", metadata, cleared)
		return nil
	}}
	cmd.Flags().BoolVar(&confirm, "confirm", false, "confirm permanent tunnel deletion")
	cmd.Flags().BoolVar(&clearConfig, "clear-config", false, "clear cgm runtime tunnel config when deleting the selected tunnel")
	return cmd
}

func addManagedConfigureFlags(cmd *cobra.Command, configure *bool, runtimeAPIKey *string, enable *bool) {
	cmd.Flags().BoolVar(configure, "configure", false, "configure cgm to use this tunnel")
	cmd.Flags().StringVar(runtimeAPIKey, "runtime-api-key", "", "runtime API key for cgm; defaults to the currently configured runtime key")
	cmd.Flags().BoolVar(enable, "enable", false, "enable the tunnel in cgm when used with --configure")
}

func configureManagedTunnel(cfg *config.Config, metadata tunnel.Metadata, runtimeAPIKey string, enable bool) error {
	if cfg == nil {
		return errors.New("configuration is unavailable")
	}
	key := strings.TrimSpace(runtimeAPIKey)
	if key == "" {
		key = strings.TrimSpace(cfg.Tunnel.APIKey)
	}
	if key == "" {
		return errors.New("runtime API key is required to configure cgm; use --runtime-api-key or configure one first")
	}
	cfg.Tunnel.ID = metadata.ID
	cfg.Tunnel.APIKey = key
	if len(metadata.OrganizationIDs) == 1 {
		cfg.Tunnel.OrganizationID = metadata.OrganizationIDs[0]
	}
	if enable {
		cfg.Tunnel.Enabled = true
	}
	return config.Validate(*cfg)
}

func logManagedTunnel(cmd *cobra.Command, message string, metadata tunnel.Metadata, configured bool) {
	log := commandLogger(cmd)
	log.Success("TUNNEL", message)
	log.Detail("id", metadata.ID)
	if metadata.Name != "" {
		log.Detail("name", metadata.Name)
	}
	if metadata.Description != "" {
		log.Detail("description", metadata.Description)
	}
	if configured {
		log.Detail("cgm", "configured")
	}
}

func formatTunnelAdminScope(scope tunnel.AdminScope) string {
	if scope.OrganizationID != "" {
		return "organization:" + scope.OrganizationID
	}
	if scope.WorkspaceID != "" {
		return "workspace:" + scope.WorkspaceID
	}
	if scope.TenantID != "" {
		return "tenant:" + scope.TenantID
	}
	return "none"
}
