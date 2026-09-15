package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/runtimecontrol"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

func tunnelLocalListCommand() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{Use: "list", Aliases: []string{"ls"}, Short: "List local tunnel instances", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		items, err := localTunnelRuntimeViews(cmd.Context())
		if err != nil {
			return err
		}
		if asJSON {
			return printJSON(cmd, items)
		}
		log := commandLogger(cmd)
		if len(items) == 0 {
			log.Info("TUNNEL", "No local tunnels attached")
			return nil
		}
		for _, item := range items {
			log.Detail(item.ID, localTunnelSummary(item))
		}
		return nil
	}}
	addJSONOutputFlag(cmd, &asJSON)
	return cmd
}

func tunnelCollectionStatusCommand() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{Use: "status [tunnel_id]", Aliases: []string{"st"}, Short: "Show local tunnel status", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		items, err := localTunnelRuntimeViews(cmd.Context())
		if err != nil {
			return err
		}
		if len(args) == 1 {
			id := strings.TrimSpace(args[0])
			for _, item := range items {
				if item.ID != id {
					continue
				}
				if asJSON {
					return printJSON(cmd, item)
				}
				renderLocalTunnelStatus(cmd, item)
				return nil
			}
			return fmt.Errorf("tunnel %q is not attached", id)
		}
		if asJSON {
			return printJSON(cmd, items)
		}
		log := commandLogger(cmd)
		for _, item := range items {
			log.Detail(item.ID, localTunnelSummary(item))
		}
		return nil
	}}
	addJSONOutputFlag(cmd, &asJSON)
	return cmd
}

func tunnelAttachCommand() *cobra.Command {
	var profileID, runtimeKey, projectID string
	var autoRuntimeKey, disabled bool
	cmd := &cobra.Command{Use: "attach <tunnel_id>", Short: "Attach a managed tunnel to this runtime", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), tunnelAdminTimeout)
		defer cancel()
		item, err := application.AttachManagedTunnelWithOptions(ctx, args[0], application.AttachManagedTunnelOptions{AdminProfileID: profileID, RuntimeAPIKey: runtimeKey, AutoGenerateRuntimeKey: autoRuntimeKey, ProjectID: projectID, Enabled: !disabled})
		if err != nil {
			return err
		}
		commandLogger(cmd).Success("TUNNEL", "Managed tunnel attached", "id", item.ID)
		return nil
	}}
	cmd.Flags().StringVar(&profileID, "admin", "", "admin profile; required when multiple profiles exist")
	cmd.Flags().StringVar(&runtimeKey, "runtime-api-key", "", "OpenAI runtime API key with Tunnels Read + Use")
	cmd.Flags().BoolVar(&autoRuntimeKey, "auto-runtime-key", false, "generate a runtime key through the selected admin profile")
	cmd.Flags().StringVar(&projectID, "project-id", "", "OpenAI project for automatic runtime key generation")
	cmd.Flags().BoolVar(&disabled, "disabled", false, "attach without enabling the tunnel")
	return cmd
}

func tunnelDetachCommand() *cobra.Command {
	return &cobra.Command{Use: "detach <tunnel_id>", Short: "Detach a local tunnel instance", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := application.DetachLocalTunnel(cmd.Context(), strings.TrimSpace(args[0])); err != nil {
			return err
		}
		commandLogger(cmd).Success("TUNNEL", "Tunnel detached", "id", strings.TrimSpace(args[0]))
		return nil
	}}
}

func tunnelLocalToggleCommand(enabled bool) *cobra.Command {
	name := "disable"
	short := "Disable one local tunnel instance"
	if enabled {
		name = "enable"
		short = "Enable one local tunnel instance"
	}
	return &cobra.Command{Use: name + " <tunnel_id>", Short: short, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		item, err := application.SetLocalTunnelEnabled(cmd.Context(), strings.TrimSpace(args[0]), enabled)
		if err != nil {
			return err
		}
		commandLogger(cmd).Success("TUNNEL", "Tunnel "+name+"d", "id", item.ID)
		return nil
	}}
}

func tunnelStartCommand() *cobra.Command { return tunnelRuntimeActionCommand("start") }
func tunnelStopCommand() *cobra.Command  { return tunnelRuntimeActionCommand("stop") }

func tunnelRuntimeActionCommand(action string) *cobra.Command {
	short := "Stop one tunnel connection in the running runtime"
	if action == "start" {
		short = "Start one tunnel connection in the running runtime"
	}
	return &cobra.Command{Use: action + " <tunnel_id>", Short: short, Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		id := strings.TrimSpace(args[0])
		ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Second)
		defer cancel()
		var result runtimecontrol.TunnelRuntimeStatus
		if _, err := runtimeControlJSONRequest(ctx, http.MethodPost, "/tunnels/"+action, map[string]string{"id": id}, &result); err != nil {
			return err
		}
		commandLogger(cmd).Success("TUNNEL", "Tunnel "+action+" requested", "id", id)
		return nil
	}}
}

func tunnelManagedCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "managed", Short: "Manage remote OpenAI tunnels"}
	cmd.AddCommand(tunnelManagedListCommand(), tunnelManagedGetCommand(), tunnelManagedCreateCommand(), tunnelManagedUpdateCommand(), tunnelManagedDeleteCommand())
	return cmd
}

func tunnelManagedListCommand() *cobra.Command {
	var profileID string
	var asJSON bool
	cmd := &cobra.Command{Use: "list", Aliases: []string{"ls"}, Short: "List managed tunnels", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), tunnelAdminTimeout)
		defer cancel()
		items, err := application.DiscoverManagedTunnels(ctx, strings.TrimSpace(profileID))
		if err != nil {
			return err
		}
		if asJSON {
			return printJSON(cmd, items)
		}
		log := commandLogger(cmd)
		for _, item := range items {
			log.Detail(item.Metadata.ID, managedTunnelSummary(item.Metadata)+" admin="+strings.Join(item.AdminProfiles, ","))
		}
		return nil
	}}
	cmd.Flags().StringVar(&profileID, "admin", "", "limit discovery to one admin profile")
	addJSONOutputFlag(cmd, &asJSON)
	return cmd
}

func tunnelManagedGetCommand() *cobra.Command {
	var profileID string
	var asJSON bool
	cmd := &cobra.Command{Use: "get <tunnel_id>", Short: "Get one managed tunnel", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), tunnelAdminTimeout)
		defer cancel()
		item, err := application.GetManagedTunnelByProfile(ctx, strings.TrimSpace(args[0]), strings.TrimSpace(profileID))
		if err != nil {
			return err
		}
		if asJSON {
			return printJSON(cmd, item)
		}
		logManagedTunnelMetadata(commandLogger(cmd), item.Metadata)
		return nil
	}}
	cmd.Flags().StringVar(&profileID, "admin", "", "admin profile; required when multiple profiles exist")
	addJSONOutputFlag(cmd, &asJSON)
	return cmd
}

func tunnelManagedCreateCommand() *cobra.Command {
	var profileID, name, description string
	var organizationIDs, workspaceIDs, tenantIDs []string
	cmd := &cobra.Command{Use: "create", Short: "Create a managed tunnel", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), tunnelAdminTimeout)
		defer cancel()
		item, err := application.CreateManagedTunnelByProfile(ctx, strings.TrimSpace(profileID), tunnel.CreateRequest{Name: strings.TrimSpace(name), Description: strings.TrimSpace(description), OrganizationIDs: normalizeTunnelIDs(organizationIDs), WorkspaceIDs: normalizeTunnelIDs(workspaceIDs), TenantIDs: normalizeTunnelIDs(tenantIDs)})
		if err != nil {
			return err
		}
		commandLogger(cmd).Success("TUNNEL", "Managed tunnel created", "id", item.Metadata.ID)
		return nil
	}}
	cmd.Flags().StringVar(&profileID, "admin", "", "admin profile; required when multiple profiles exist")
	cmd.Flags().StringVar(&name, "name", "", "tunnel name")
	cmd.Flags().StringVar(&description, "description", "", "tunnel description")
	cmd.Flags().StringSliceVar(&organizationIDs, "organization-id", nil, "OpenAI organization identifier; repeatable")
	cmd.Flags().StringSliceVar(&workspaceIDs, "workspace-id", nil, "OpenAI workspace identifier; repeatable")
	cmd.Flags().StringSliceVar(&tenantIDs, "tenant-id", nil, "OpenAI tenant identifier; repeatable")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("description")
	return cmd
}

func tunnelManagedUpdateCommand() *cobra.Command {
	var profileID, name, description string
	var organizationIDs, workspaceIDs, tenantIDs []string
	cmd := &cobra.Command{Use: "update <tunnel_id>", Short: "Update a managed tunnel", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
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
		item, err := application.UpdateManagedTunnelByProfile(ctx, strings.TrimSpace(args[0]), strings.TrimSpace(profileID), request)
		if err != nil {
			return err
		}
		commandLogger(cmd).Success("TUNNEL", "Managed tunnel updated", "id", item.Metadata.ID)
		return nil
	}}
	cmd.Flags().StringVar(&profileID, "admin", "", "admin profile; required when multiple profiles exist")
	cmd.Flags().StringVar(&name, "name", "", "new tunnel name")
	cmd.Flags().StringVar(&description, "description", "", "new tunnel description")
	cmd.Flags().StringSliceVar(&organizationIDs, "organization-id", nil, "replace organization identifiers; repeatable")
	cmd.Flags().StringSliceVar(&workspaceIDs, "workspace-id", nil, "replace workspace identifiers; repeatable")
	cmd.Flags().StringSliceVar(&tenantIDs, "tenant-id", nil, "replace tenant identifiers; repeatable")
	return cmd
}

func tunnelManagedDeleteCommand() *cobra.Command {
	var profileID string
	var confirm bool
	cmd := &cobra.Command{Use: "delete <tunnel_id>", Aliases: []string{"rm"}, Short: "Delete a remote managed tunnel", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if !confirm {
			return errors.New("refusing to delete tunnel without --confirm")
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), tunnelAdminTimeout)
		defer cancel()
		metadata, err := application.DeleteManagedTunnelByProfile(ctx, strings.TrimSpace(args[0]), strings.TrimSpace(profileID))
		if err != nil {
			return err
		}
		commandLogger(cmd).Success("TUNNEL", "Managed tunnel deleted", "id", metadata.ID)
		return nil
	}}
	cmd.Flags().StringVar(&profileID, "admin", "", "admin profile; required when multiple profiles exist")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "confirm permanent remote deletion")
	return cmd
}

func tunnelAdminProfilesCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "admin", Short: "Manage tunnel admin profiles"}
	cmd.AddCommand(tunnelAdminListProfilesCommand(), tunnelAdminAddProfileCommand(), tunnelAdminVerifyProfileCommand(), tunnelAdminRemoveProfileCommand())
	return cmd
}

func tunnelAdminListProfilesCommand() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{Use: "list", Aliases: []string{"ls"}, Short: "List tunnel admin profiles", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		items, err := application.TunnelAdminProfiles()
		if err != nil {
			return err
		}
		if asJSON {
			return printJSON(cmd, items)
		}
		log := commandLogger(cmd)
		for _, item := range items {
			log.Detail(item.ID, adminProfileSummary(item))
		}
		return nil
	}}
	addJSONOutputFlag(cmd, &asJSON)
	return cmd
}

func tunnelAdminAddProfileCommand() *cobra.Command {
	var adminKey, controlPlane string
	var scope tunnelAdminScopeFlags
	cmd := &cobra.Command{Use: "add <profile>", Short: "Add a tunnel admin profile", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		admin := tunnel.AdminConfig{ID: strings.TrimSpace(args[0]), AdminKey: strings.TrimSpace(adminKey), ControlPlaneBaseURL: strings.TrimSpace(controlPlane), OrganizationID: strings.TrimSpace(scope.organizationID), WorkspaceID: strings.TrimSpace(scope.workspaceID), TenantID: strings.TrimSpace(scope.tenantID)}
		if admin.AdminKey == "" {
			return errors.New("admin key is required")
		}
		if err := tunnel.ValidateAdminScope(tunnel.AdminScope{OrganizationID: admin.OrganizationID, WorkspaceID: admin.WorkspaceID, TenantID: admin.TenantID}); err != nil {
			return err
		}
		item, count, err := application.AddTunnelAdminProfile(cmd.Context(), admin)
		if err != nil {
			return err
		}
		commandLogger(cmd).Success("TUNNEL", "Admin profile added", "profile", item.ID)
		commandLogger(cmd).Detail("tunnels", count)
		return nil
	}}
	cmd.Flags().StringVar(&adminKey, "admin-key", "", "OpenAI admin API key")
	cmd.Flags().StringVar(&controlPlane, "control-plane-base-url", "", "OpenAI control plane base URL")
	scope.add(cmd)
	return cmd
}

func tunnelAdminVerifyProfileCommand() *cobra.Command {
	return &cobra.Command{Use: "verify <profile>", Short: "Verify one tunnel admin profile", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := context.WithTimeout(cmd.Context(), tunnelAdminTimeout)
		defer cancel()
		item, count, err := application.VerifyTunnelAdminProfile(ctx, strings.TrimSpace(args[0]))
		if err != nil {
			return err
		}
		log := commandLogger(cmd)
		log.Success("TUNNEL", "Admin profile verified", "profile", item.ID)
		log.Detail("tunnels", count)
		return nil
	}}
}

func tunnelAdminRemoveProfileCommand() *cobra.Command {
	return &cobra.Command{Use: "remove <profile>", Aliases: []string{"rm"}, Short: "Remove one tunnel admin profile", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := application.RemoveTunnelAdminProfile(cmd.Context(), strings.TrimSpace(args[0])); err != nil {
			return err
		}
		commandLogger(cmd).Success("TUNNEL", "Admin profile removed", "profile", strings.TrimSpace(args[0]))
		return nil
	}}
}

func localTunnelRuntimeViews(ctx context.Context) ([]application.LocalTunnel, error) {
	items, err := application.LocalTunnels()
	if err != nil {
		return nil, err
	}
	runtimeCtx, cancel := context.WithTimeout(ctx, time.Second)
	status, running, err := managedRuntimeStatus(runtimeCtx)
	cancel()
	if err != nil {
		return nil, err
	}
	if !running {
		return items, nil
	}
	byID := make(map[string]runtimecontrol.TunnelRuntimeStatus, len(status.Tunnels))
	for _, item := range status.Tunnels {
		byID[item.ID] = item
	}
	for i := range items {
		if current, ok := byID[items[i].ID]; ok {
			items[i].Status.Running = current.Running
			items[i].Status.Ready = current.Ready
			items[i].Status.Restarting = current.Restarting
			items[i].Status.LastError = current.LastError
		}
	}
	return items, nil
}

func localTunnelSummary(item application.LocalTunnel) string {
	state := "disabled"
	if item.Enabled {
		state = "offline"
		if item.Status.Ready {
			state = "ready"
		} else if item.Status.Restarting {
			state = "reconnecting"
		} else if item.Status.Running {
			state = "connecting"
		} else if item.Status.LastError != "" {
			state = "failed"
		}
	}
	parts := []string{state}
	if item.AdminProfileID != "" {
		parts = append(parts, "admin="+item.AdminProfileID)
	}
	return strings.Join(parts, " · ")
}

func renderLocalTunnelStatus(cmd *cobra.Command, item application.LocalTunnel) {
	log := commandLogger(cmd)
	log.Info("TUNNEL", item.ID)
	log.Detail("enabled", item.Enabled)
	log.Detail("runtime key", item.RuntimeKeyConfigured)
	log.Detail("state", localTunnelSummary(item))
	if item.AdminProfileID != "" {
		log.Detail("admin profile", item.AdminProfileID)
	}
	if item.Status.LastError != "" {
		log.Detail("error", item.Status.LastError)
	}
}

func adminProfileSummary(item application.TunnelAdminProfile) string {
	access := "not verified"
	if item.ManageAccess {
		access = "manage"
	} else if item.ReadAccess {
		access = "read"
	}
	scope := tunnel.AdminScope{OrganizationID: item.OrganizationID, WorkspaceID: item.WorkspaceID, TenantID: item.TenantID}
	return fmt.Sprintf("%s · %s", formatTunnelAdminScope(scope), access)
}
