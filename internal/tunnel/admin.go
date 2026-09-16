package tunnel

import (
	"context"
	"errors"
	"strings"
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
	backend, err := requireAdmin()
	if err != nil {
		return AdminAccess{}, 0, err
	}
	return backend.VerifyAdminKey(ctx, cfg)
}

func ListManaged(ctx context.Context, cfg Config, scope AdminScope) ([]Metadata, error) {
	backend, err := requireAdmin()
	if err != nil {
		return nil, err
	}
	return backend.ListManaged(ctx, cfg, scope)
}

func GetManaged(ctx context.Context, cfg Config, id string) (Metadata, error) {
	backend, err := requireAdmin()
	if err != nil {
		return Metadata{}, err
	}
	return backend.GetManaged(ctx, cfg, id)
}

func CreateManaged(ctx context.Context, cfg Config, req CreateRequest) (Metadata, error) {
	backend, err := requireAdmin()
	if err != nil {
		return Metadata{}, err
	}
	return backend.CreateManaged(ctx, cfg, req)
}

func UpdateManaged(ctx context.Context, cfg Config, id string, req UpdateRequest) (Metadata, error) {
	backend, err := requireAdmin()
	if err != nil {
		return Metadata{}, err
	}
	return backend.UpdateManaged(ctx, cfg, id, req)
}

func DeleteManaged(ctx context.Context, cfg Config, id string) (Metadata, error) {
	backend, err := requireAdmin()
	if err != nil {
		return Metadata{}, err
	}
	return backend.DeleteManaged(ctx, cfg, id)
}
