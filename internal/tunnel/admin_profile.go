package tunnel

import "context"

// Admin profiles own management credentials independently of runtime clients.
func (cfg AdminConfig) legacyAdminConfig() Config {
	return Config{AdminKey: cfg.AdminKey, AdminOrganizationID: cfg.OrganizationID, AdminWorkspaceID: cfg.WorkspaceID, AdminTenantID: cfg.TenantID, AdminReadAccess: cfg.ReadAccess, AdminManageAccess: cfg.ManageAccess, ControlPlaneBaseURL: cfg.ControlPlaneBaseURL}
}

func VerifyAdminProfile(ctx context.Context, cfg AdminConfig) (AdminAccess, int, error) {
	return VerifyAdminKey(ctx, cfg.legacyAdminConfig())
}

func ListManagedForAdmin(ctx context.Context, cfg AdminConfig) ([]Metadata, error) {
	return ListManaged(ctx, cfg.legacyAdminConfig(), AdminScope{OrganizationID: cfg.OrganizationID, WorkspaceID: cfg.WorkspaceID, TenantID: cfg.TenantID})
}

func GetManagedForAdmin(ctx context.Context, cfg AdminConfig, id string) (Metadata, error) {
	return GetManaged(ctx, cfg.legacyAdminConfig(), id)
}

func CreateManagedForAdmin(ctx context.Context, cfg AdminConfig, request CreateRequest) (Metadata, error) {
	return CreateManaged(ctx, cfg.legacyAdminConfig(), request)
}

func UpdateManagedForAdmin(ctx context.Context, cfg AdminConfig, id string, request UpdateRequest) (Metadata, error) {
	return UpdateManaged(ctx, cfg.legacyAdminConfig(), id, request)
}

func DeleteManagedForAdmin(ctx context.Context, cfg AdminConfig, id string) (Metadata, error) {
	return DeleteManaged(ctx, cfg.legacyAdminConfig(), id)
}

func GenerateRuntimeKeyForAdmin(ctx context.Context, cfg AdminConfig, projectID string) (GeneratedRuntimeKey, error) {
	return GenerateRuntimeKey(ctx, cfg.legacyAdminConfig(), projectID)
}
