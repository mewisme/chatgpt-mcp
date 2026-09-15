package cli

import (
	"strings"
	"time"

	"github.com/spf13/cobra"

	"go.mewis.me/chatgpt-mcp/internal/logger"
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

func logManagedTunnelMetadata(log *logger.Logger, metadata tunnel.Metadata) {
	log.Detail("id", metadata.ID)
	if metadata.Name != "" {
		log.Detail("name", metadata.Name)
	}
	if metadata.Description != "" {
		log.Detail("description", metadata.Description)
	}
	if metadata.Creator != "" {
		log.Detail("creator", metadata.Creator)
	}
	if len(metadata.OrganizationIDs) > 0 {
		log.Detail("organizations", metadata.OrganizationIDs)
	}
	if len(metadata.WorkspaceIDs) > 0 {
		log.Detail("workspaces", metadata.WorkspaceIDs)
	}
	if len(metadata.TenantIDs) > 0 {
		log.Detail("tenants", metadata.TenantIDs)
	}
	if metadata.RequestID != "" {
		log.Detail("request", metadata.RequestID)
	}
	if !metadata.FetchedAt.IsZero() {
		log.Detail("fetched", metadata.FetchedAt.Local().Format(time.RFC3339))
	}
}

func managedTunnelSummary(metadata tunnel.Metadata) string {
	name := metadata.Name
	if name == "" {
		name = "unnamed"
	}
	scope := append(append(append([]string{}, metadata.OrganizationIDs...), metadata.WorkspaceIDs...), metadata.TenantIDs...)
	if len(scope) == 0 {
		return name
	}
	return name + " scope=" + strings.Join(scope, ",")
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
