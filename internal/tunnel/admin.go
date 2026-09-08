package tunnel

import (
	"context"
	"errors"
	"net/http"
	"strings"

	tcadmin "github.com/openai/tunnel-client/pkg/controlplane/admin"
)

type UpdateRequest struct {
	Name            *string
	Description     *string
	TenantIDs       *[]string
	WorkspaceIDs    *[]string
	OrganizationIDs *[]string
}

func AdminScopeFromConfig(cfg Config) AdminScope {
	return AdminScope{OrganizationID: strings.TrimSpace(cfg.AdminOrganizationID), WorkspaceID: strings.TrimSpace(cfg.AdminWorkspaceID), TenantID: strings.TrimSpace(cfg.AdminTenantID)}
}

func ApplyAdminScope(cfg *Config, scope AdminScope) {
	if cfg == nil {
		return
	}
	cfg.AdminOrganizationID = strings.TrimSpace(scope.OrganizationID)
	cfg.AdminWorkspaceID = strings.TrimSpace(scope.WorkspaceID)
	cfg.AdminTenantID = strings.TrimSpace(scope.TenantID)
}

func ValidateAdminScope(scope AdminScope) error {
	count := 0
	if strings.TrimSpace(scope.OrganizationID) != "" {
		count++
	}
	if strings.TrimSpace(scope.WorkspaceID) != "" {
		count++
	}
	if strings.TrimSpace(scope.TenantID) != "" {
		count++
	}
	if count != 1 {
		return errors.New("provide exactly one admin scope: organization, workspace, or tenant")
	}
	return nil
}

func AdminConfigured(cfg Config) bool {
	return strings.TrimSpace(cfg.AdminKey) != "" && ValidateAdminScope(AdminScopeFromConfig(cfg)) == nil
}

func AdminAccessFromConfig(cfg Config) AdminAccess {
	return AdminAccess{Read: cfg.AdminReadAccess, Manage: cfg.AdminManageAccess}
}

func ApplyAdminAccess(cfg *Config, access AdminAccess) {
	if cfg == nil {
		return
	}
	cfg.AdminReadAccess = access.Read
	cfg.AdminManageAccess = access.Manage
}

func VerifyAdminKey(ctx context.Context, cfg Config) (AdminAccess, int, error) {
	items, err := ListManaged(ctx, cfg, AdminScopeFromConfig(cfg))
	if err == nil {
		return AdminAccess{Read: true, Manage: true}, len(items), nil
	}
	var requestErr *tcadmin.RequestError
	if !errors.As(err, &requestErr) || requestErr.StatusCode != http.StatusForbidden {
		return AdminAccess{}, 0, err
	}
	client, clientErr := adminTunnelClient(cfg, cfg.AdminKey)
	if clientErr != nil {
		return AdminAccess{}, 0, clientErr
	}
	_, readErr := client.GetTunnel(ctx, "tunnel_00000000000000000000000000000000")
	if readErr == nil {
		return AdminAccess{Read: true}, 0, nil
	}
	var readRequestErr *tcadmin.RequestError
	if errors.As(readErr, &readRequestErr) && (readRequestErr.StatusCode == http.StatusNotFound || readRequestErr.StatusCode == http.StatusBadRequest) {
		return AdminAccess{Read: true}, 0, nil
	}
	return AdminAccess{}, 0, err
}

func ListManaged(ctx context.Context, cfg Config, scope AdminScope) ([]Metadata, error) {
	if strings.TrimSpace(cfg.AdminKey) == "" {
		return nil, errors.New("OpenAI tunnel admin key is not configured")
	}
	if err := ValidateAdminScope(scope); err != nil {
		return nil, err
	}
	client, err := adminTunnelClient(cfg, cfg.AdminKey)
	if err != nil {
		return nil, err
	}
	response, err := client.ListTunnels(ctx, strings.TrimSpace(scope.OrganizationID), strings.TrimSpace(scope.WorkspaceID), strings.TrimSpace(scope.TenantID))
	if err != nil {
		return nil, err
	}
	items := make([]Metadata, 0, len(response.Tunnels))
	for index := range response.Tunnels {
		items = append(items, metadataFromTunnel(&response.Tunnels[index]))
	}
	return items, nil
}

func GetManaged(ctx context.Context, cfg Config, id string) (Metadata, error) {
	if strings.TrimSpace(cfg.AdminKey) == "" {
		return Metadata{}, errors.New("OpenAI tunnel admin key is not configured")
	}
	client, err := adminTunnelClient(cfg, cfg.AdminKey)
	if err != nil {
		return Metadata{}, err
	}
	value, err := client.GetTunnel(ctx, strings.TrimSpace(id))
	if err != nil {
		return Metadata{}, err
	}
	return metadataFromTunnel(value), nil
}

func CreateManaged(ctx context.Context, cfg Config, req CreateRequest) (Metadata, error) {
	return createWithAdminKey(ctx, cfg, cfg.AdminKey, req)
}

func UpdateManaged(ctx context.Context, cfg Config, id string, req UpdateRequest) (Metadata, error) {
	if strings.TrimSpace(cfg.AdminKey) == "" {
		return Metadata{}, errors.New("OpenAI tunnel admin key is not configured")
	}
	if req.Name == nil && req.Description == nil && req.TenantIDs == nil && req.WorkspaceIDs == nil && req.OrganizationIDs == nil {
		return Metadata{}, errors.New("provide at least one field to update")
	}
	client, err := adminTunnelClient(cfg, cfg.AdminKey)
	if err != nil {
		return Metadata{}, err
	}
	value, err := client.UpdateTunnel(ctx, strings.TrimSpace(id), tcadmin.TunnelUpdateRequest{
		Name: req.Name, Description: req.Description, TenantIDs: req.TenantIDs, WorkspaceIDs: req.WorkspaceIDs, OrganizationIDs: req.OrganizationIDs,
	})
	if err != nil {
		return Metadata{}, err
	}
	return metadataFromTunnel(value), nil
}

func DeleteManaged(ctx context.Context, cfg Config, id string) (Metadata, error) {
	if strings.TrimSpace(cfg.AdminKey) == "" {
		return Metadata{}, errors.New("OpenAI tunnel admin key is not configured")
	}
	client, err := adminTunnelClient(cfg, cfg.AdminKey)
	if err != nil {
		return Metadata{}, err
	}
	value, err := client.DeleteTunnel(ctx, strings.TrimSpace(id))
	if err != nil {
		return Metadata{}, err
	}
	return metadataFromTunnel(value), nil
}
