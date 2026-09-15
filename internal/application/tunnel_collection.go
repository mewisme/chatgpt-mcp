package application

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/runtimecontrol"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

type LocalTunnel struct {
	ID                   string        `json:"id"`
	Enabled              bool          `json:"enabled"`
	RuntimeKeyConfigured bool          `json:"runtime_key_configured"`
	AdminProfileID       string        `json:"admin_profile_id,omitempty"`
	ControlPlaneBaseURL  string        `json:"control_plane_base_url,omitempty"`
	OrganizationID       string        `json:"organization_id,omitempty"`
	Status               tunnel.Status `json:"status"`
}

type TunnelAdminProfile struct {
	ID                  string `json:"id"`
	KeyConfigured       bool   `json:"key_configured"`
	OrganizationID      string `json:"organization_id,omitempty"`
	WorkspaceID         string `json:"workspace_id,omitempty"`
	TenantID            string `json:"tenant_id,omitempty"`
	ReadAccess          bool   `json:"read_access"`
	ManageAccess        bool   `json:"manage_access"`
	ControlPlaneBaseURL string `json:"control_plane_base_url,omitempty"`
}

type ManagedTunnelDiscovery struct {
	Metadata      tunnel.Metadata `json:"metadata"`
	AdminProfiles []string        `json:"admin_profiles"`
}

type AttachManagedTunnelOptions struct {
	AdminProfileID         string
	RuntimeAPIKey          string
	AutoGenerateRuntimeKey bool
	ProjectID              string
	Enabled                bool
}

func LocalTunnels() ([]LocalTunnel, error) {
	return LocalTunnelsContext(context.Background())
}

func LocalTunnelsContext(ctx context.Context) ([]LocalTunnel, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	instances := cfg.RuntimeTunnels().Instances
	items := make([]LocalTunnel, 0, len(instances))
	for _, instance := range instances {
		client := tunnel.NewConfigured(tunnel.Config{Enabled: instance.Enabled, ID: instance.ID, APIKey: instance.APIKey, ControlPlaneBaseURL: instance.ControlPlaneBaseURL, OrganizationID: instance.OrganizationID}, nil)
		if metadata, err := config.LoadTunnelMetadata(instance.ID); err == nil {
			_ = client.SeedMetadata(metadata)
		}
		items = append(items, localTunnelView(instance, client.Status()))
	}
	status, running, err := RuntimeStatus(ctx)
	if err != nil {
		return nil, err
	}
	if !running {
		return items, nil
	}
	byID := make(map[string]runtimecontrol.TunnelRuntimeStatus, len(status.Tunnels))
	for _, current := range status.Tunnels {
		byID[current.ID] = current
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

func StartLocalTunnel(ctx context.Context, id string) (LocalTunnel, error) {
	return controlLocalTunnel(ctx, id, "start")
}

func StopLocalTunnel(ctx context.Context, id string) (LocalTunnel, error) {
	return controlLocalTunnel(ctx, id, "stop")
}

func controlLocalTunnel(ctx context.Context, id, action string) (LocalTunnel, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return LocalTunnel{}, errors.New("tunnel id is required")
	}
	var status runtimecontrol.TunnelRuntimeStatus
	if _, err := runtimecontrol.Request(ctx, http.MethodPost, "/tunnels/"+action, map[string]string{"id": id}, &status); err != nil {
		return LocalTunnel{}, err
	}
	items, err := LocalTunnelsContext(ctx)
	if err != nil {
		return LocalTunnel{}, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return LocalTunnel{}, fmt.Errorf("tunnel %q is not attached", id)
}

func localTunnelView(instance tunnel.InstanceConfig, status tunnel.Status) LocalTunnel {
	status.AdminKeyConfigured = false
	status.AdminScope = nil
	return LocalTunnel{ID: instance.ID, Enabled: instance.Enabled, RuntimeKeyConfigured: instance.APIKey != "", AdminProfileID: instance.AdminProfileID, ControlPlaneBaseURL: instance.ControlPlaneBaseURL, OrganizationID: instance.OrganizationID, Status: status}
}

func saveTunnelCollection(ctx context.Context, previous config.Config, collection tunnel.CollectionConfig) error {
	instances := append([]tunnel.InstanceConfig{}, collection.Instances...)
	admins := append([]tunnel.AdminConfig{}, collection.Admins...)
	next := previous
	next.Tunnel = tunnel.Config{Instances: &instances, Admins: &admins}
	if err := config.Validate(next); err != nil {
		return err
	}
	_, _, err := saveConfigMutation(ctx, previous, next)
	return err
}

func AttachLocalTunnel(ctx context.Context, instance tunnel.InstanceConfig) (LocalTunnel, error) {
	instance.ID = strings.TrimSpace(instance.ID)
	if instance.ID == "" {
		return LocalTunnel{}, errors.New("tunnel id is required")
	}
	previous, err := config.Load()
	if err != nil {
		return LocalTunnel{}, err
	}
	collection := previous.RuntimeTunnels()
	for _, existing := range collection.Instances {
		if existing.ID == instance.ID {
			return LocalTunnel{}, fmt.Errorf("tunnel %q is already attached", instance.ID)
		}
	}
	collection.Instances = append(collection.Instances, instance)
	if err := saveTunnelCollection(ctx, previous, collection); err != nil {
		return LocalTunnel{}, err
	}
	return localTunnelView(instance, tunnel.Status{ID: instance.ID, Enabled: instance.Enabled}), nil
}

func UpdateLocalTunnel(ctx context.Context, instance tunnel.InstanceConfig) (LocalTunnel, error) {
	previous, err := config.Load()
	if err != nil {
		return LocalTunnel{}, err
	}
	collection := previous.RuntimeTunnels()
	for i, existing := range collection.Instances {
		if existing.ID != instance.ID {
			continue
		}
		if instance.APIKey == "" {
			instance.APIKey = existing.APIKey
		}
		collection.Instances[i] = instance
		if err := saveTunnelCollection(ctx, previous, collection); err != nil {
			return LocalTunnel{}, err
		}
		return localTunnelView(instance, tunnel.Status{ID: instance.ID, Enabled: instance.Enabled}), nil
	}
	return LocalTunnel{}, fmt.Errorf("tunnel %q is not attached", instance.ID)
}

func DetachLocalTunnel(ctx context.Context, id string) error {
	previous, err := config.Load()
	if err != nil {
		return err
	}
	collection := previous.RuntimeTunnels()
	for i, instance := range collection.Instances {
		if instance.ID != id {
			continue
		}
		collection.Instances = append(collection.Instances[:i], collection.Instances[i+1:]...)
		return saveTunnelCollection(ctx, previous, collection)
	}
	return fmt.Errorf("tunnel %q is not attached", id)
}

func SetLocalTunnelEnabled(ctx context.Context, id string, enabled bool) (LocalTunnel, error) {
	previous, err := config.Load()
	if err != nil {
		return LocalTunnel{}, err
	}
	collection := previous.RuntimeTunnels()
	for i, instance := range collection.Instances {
		if instance.ID != id {
			continue
		}
		collection.Instances[i].Enabled = enabled
		if err := saveTunnelCollection(ctx, previous, collection); err != nil {
			return LocalTunnel{}, err
		}
		return localTunnelView(collection.Instances[i], tunnel.Status{ID: id, Enabled: enabled}), nil
	}
	return LocalTunnel{}, fmt.Errorf("tunnel %q is not attached", id)
}

func TunnelAdminProfiles() ([]TunnelAdminProfile, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	admins := cfg.RuntimeTunnels().Admins
	items := make([]TunnelAdminProfile, 0, len(admins))
	for _, admin := range admins {
		items = append(items, adminProfileView(admin))
	}
	return items, nil
}

func adminProfileView(admin tunnel.AdminConfig) TunnelAdminProfile {
	return TunnelAdminProfile{ID: admin.ID, KeyConfigured: admin.AdminKey != "", OrganizationID: admin.OrganizationID, WorkspaceID: admin.WorkspaceID, TenantID: admin.TenantID, ReadAccess: admin.ReadAccess, ManageAccess: admin.ManageAccess, ControlPlaneBaseURL: admin.ControlPlaneBaseURL}
}

func AddTunnelAdminProfile(ctx context.Context, admin tunnel.AdminConfig) (TunnelAdminProfile, error) {
	admin.ID = strings.TrimSpace(admin.ID)
	if admin.ID == "" {
		return TunnelAdminProfile{}, errors.New("admin profile id is required")
	}
	previous, err := config.Load()
	if err != nil {
		return TunnelAdminProfile{}, err
	}
	collection := previous.RuntimeTunnels()
	for _, existing := range collection.Admins {
		if existing.ID == admin.ID {
			return TunnelAdminProfile{}, fmt.Errorf("admin profile %q already exists", admin.ID)
		}
	}
	collection.Admins = append(collection.Admins, admin)
	if err := saveTunnelCollection(ctx, previous, collection); err != nil {
		return TunnelAdminProfile{}, err
	}
	return adminProfileView(admin), nil
}

func UpdateTunnelAdminProfile(ctx context.Context, admin tunnel.AdminConfig) (TunnelAdminProfile, error) {
	previous, err := config.Load()
	if err != nil {
		return TunnelAdminProfile{}, err
	}
	collection := previous.RuntimeTunnels()
	for i, existing := range collection.Admins {
		if existing.ID != admin.ID {
			continue
		}
		if admin.AdminKey == "" {
			admin.AdminKey = existing.AdminKey
			admin.ReadAccess, admin.ManageAccess = existing.ReadAccess, existing.ManageAccess
		}
		collection.Admins[i] = admin
		if err := saveTunnelCollection(ctx, previous, collection); err != nil {
			return TunnelAdminProfile{}, err
		}
		return adminProfileView(admin), nil
	}
	return TunnelAdminProfile{}, fmt.Errorf("admin profile %q not found", admin.ID)
}

func RemoveTunnelAdminProfile(ctx context.Context, id string) error {
	previous, err := config.Load()
	if err != nil {
		return err
	}
	collection := previous.RuntimeTunnels()
	for _, instance := range collection.Instances {
		if instance.AdminProfileID == id {
			return fmt.Errorf("admin profile %q is used by tunnel %q", id, instance.ID)
		}
	}
	for i, admin := range collection.Admins {
		if admin.ID != id {
			continue
		}
		collection.Admins = append(collection.Admins[:i], collection.Admins[i+1:]...)
		return saveTunnelCollection(ctx, previous, collection)
	}
	return fmt.Errorf("admin profile %q not found", id)
}

func resolveProfile(collection tunnel.CollectionConfig, profileID string) (tunnel.AdminConfig, error) {
	if profileID == "" {
		if len(collection.Admins) == 1 {
			return collection.Admins[0], nil
		}
		return tunnel.AdminConfig{}, errors.New("admin profile must be specified")
	}
	for _, admin := range collection.Admins {
		if admin.ID == profileID {
			return admin, nil
		}
	}
	return tunnel.AdminConfig{}, fmt.Errorf("admin profile %q not found", profileID)
}

func VerifyTunnelAdminProfile(ctx context.Context, profileID string) (TunnelAdminProfile, int, error) {
	previous, err := config.Load()
	if err != nil {
		return TunnelAdminProfile{}, 0, err
	}
	collection := previous.RuntimeTunnels()
	admin, err := resolveProfile(collection, profileID)
	if err != nil {
		return TunnelAdminProfile{}, 0, err
	}
	access, count, err := tunnel.VerifyAdminProfile(ctx, admin)
	if err != nil {
		return TunnelAdminProfile{}, 0, err
	}
	admin.ReadAccess, admin.ManageAccess = access.Read, access.Manage
	for i := range collection.Admins {
		if collection.Admins[i].ID == admin.ID {
			collection.Admins[i] = admin
		}
	}
	if err := saveTunnelCollection(ctx, previous, collection); err != nil {
		return TunnelAdminProfile{}, 0, err
	}
	return adminProfileView(admin), count, nil
}

func DiscoverManagedTunnels(ctx context.Context, profileID string) ([]ManagedTunnelDiscovery, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	collection := cfg.RuntimeTunnels()
	if profileID != "" {
		if _, err := resolveProfile(collection, profileID); err != nil {
			return nil, err
		}
	}
	index := map[string]int{}
	items := []ManagedTunnelDiscovery{}
	for _, admin := range collection.Admins {
		if profileID != "" && admin.ID != profileID {
			continue
		}
		if !admin.ReadAccess && !admin.ManageAccess {
			continue
		}
		found, err := tunnel.ListManagedForAdmin(ctx, admin)
		if err != nil {
			return nil, fmt.Errorf("admin profile %s: %w", admin.ID, err)
		}
		for _, metadata := range found {
			if i, ok := index[metadata.ID]; ok {
				items[i].AdminProfiles = append(items[i].AdminProfiles, admin.ID)
				continue
			}
			index[metadata.ID] = len(items)
			items = append(items, ManagedTunnelDiscovery{Metadata: metadata, AdminProfiles: []string{admin.ID}})
		}
	}
	return items, nil
}

func GetManagedTunnelByProfile(ctx context.Context, id, profileID string) (ManagedTunnelDiscovery, error) {
	cfg, err := config.Load()
	if err != nil {
		return ManagedTunnelDiscovery{}, err
	}
	admin, err := resolveProfile(cfg.RuntimeTunnels(), profileID)
	if err != nil {
		return ManagedTunnelDiscovery{}, err
	}
	if !admin.ReadAccess && !admin.ManageAccess {
		return ManagedTunnelDiscovery{}, errors.New("admin profile lacks Read access")
	}
	metadata, err := tunnel.GetManagedForAdmin(ctx, admin, id)
	if err != nil {
		return ManagedTunnelDiscovery{}, err
	}
	return ManagedTunnelDiscovery{Metadata: metadata, AdminProfiles: []string{admin.ID}}, nil
}

func CreateManagedTunnelByProfile(ctx context.Context, profileID string, request tunnel.CreateRequest) (ManagedTunnelDiscovery, error) {
	cfg, err := config.Load()
	if err != nil {
		return ManagedTunnelDiscovery{}, err
	}
	admin, err := resolveProfile(cfg.RuntimeTunnels(), profileID)
	if err != nil {
		return ManagedTunnelDiscovery{}, err
	}
	if !admin.ManageAccess {
		return ManagedTunnelDiscovery{}, errors.New("admin profile lacks Manage access")
	}
	metadata, err := tunnel.CreateManagedForAdmin(ctx, admin, request)
	if err != nil {
		return ManagedTunnelDiscovery{}, err
	}
	_, _ = config.SaveTunnelMetadata(metadata)
	return ManagedTunnelDiscovery{Metadata: metadata, AdminProfiles: []string{admin.ID}}, nil
}

func UpdateManagedTunnelByProfile(ctx context.Context, id, profileID string, request tunnel.UpdateRequest) (ManagedTunnelDiscovery, error) {
	cfg, err := config.Load()
	if err != nil {
		return ManagedTunnelDiscovery{}, err
	}
	admin, err := resolveProfile(cfg.RuntimeTunnels(), profileID)
	if err != nil {
		return ManagedTunnelDiscovery{}, err
	}
	if !admin.ManageAccess {
		return ManagedTunnelDiscovery{}, errors.New("admin profile lacks Manage access")
	}
	metadata, err := tunnel.UpdateManagedForAdmin(ctx, admin, id, request)
	if err != nil {
		return ManagedTunnelDiscovery{}, err
	}
	_, _ = config.SaveTunnelMetadata(metadata)
	return ManagedTunnelDiscovery{Metadata: metadata, AdminProfiles: []string{admin.ID}}, nil
}

func AttachManagedTunnel(ctx context.Context, id, profileID, runtimeKey string, enabled bool) (LocalTunnel, error) {
	return AttachManagedTunnelWithOptions(ctx, id, AttachManagedTunnelOptions{AdminProfileID: profileID, RuntimeAPIKey: runtimeKey, Enabled: enabled})
}

func AttachManagedTunnelWithOptions(ctx context.Context, id string, options AttachManagedTunnelOptions) (LocalTunnel, error) {
	previous, err := config.Load()
	if err != nil {
		return LocalTunnel{}, err
	}
	collection := previous.RuntimeTunnels()
	admin, err := resolveProfile(collection, strings.TrimSpace(options.AdminProfileID))
	if err != nil {
		return LocalTunnel{}, err
	}
	if !admin.ReadAccess && !admin.ManageAccess {
		return LocalTunnel{}, errors.New("admin profile lacks Read access")
	}
	for _, instance := range collection.Instances {
		if instance.ID == id {
			return LocalTunnel{}, fmt.Errorf("tunnel %q is already attached", id)
		}
	}
	runtimeKey := strings.TrimSpace(options.RuntimeAPIKey)
	if runtimeKey == "" && options.AutoGenerateRuntimeKey {
		if !admin.ManageAccess {
			return LocalTunnel{}, errors.New("admin profile lacks Manage access required for runtime key generation")
		}
		generated, err := tunnel.GenerateRuntimeKeyForAdmin(ctx, admin, strings.TrimSpace(options.ProjectID))
		if err != nil {
			return LocalTunnel{}, err
		}
		runtimeKey = generated.Value
	}
	if runtimeKey == "" {
		return LocalTunnel{}, errors.New("runtime API key is required")
	}
	metadata, err := tunnel.GetManagedForAdmin(ctx, admin, id)
	if err != nil {
		return LocalTunnel{}, err
	}
	instance := tunnel.InstanceConfig{ID: id, Enabled: options.Enabled, APIKey: runtimeKey, AdminProfileID: admin.ID, ControlPlaneBaseURL: admin.ControlPlaneBaseURL}
	if len(metadata.OrganizationIDs) > 0 {
		instance.OrganizationID = metadata.OrganizationIDs[0]
	}
	collection.Instances = append(collection.Instances, instance)
	if err := saveTunnelCollection(ctx, previous, collection); err != nil {
		return LocalTunnel{}, err
	}
	_, _ = config.SaveTunnelMetadata(metadata)
	return localTunnelView(instance, tunnel.Status{ID: id, Enabled: options.Enabled, Metadata: &metadata}), nil
}

func DeleteManagedTunnelByProfile(ctx context.Context, id, profileID string) (tunnel.Metadata, error) {
	cfg, err := config.Load()
	if err != nil {
		return tunnel.Metadata{}, err
	}
	collection := cfg.RuntimeTunnels()
	admin, err := resolveProfile(collection, profileID)
	if err != nil {
		return tunnel.Metadata{}, err
	}
	if !admin.ManageAccess {
		return tunnel.Metadata{}, errors.New("admin profile lacks Manage access")
	}
	for _, instance := range collection.Instances {
		if instance.ID == id {
			return tunnel.Metadata{}, errors.New("detach local tunnel before remote deletion")
		}
	}
	metadata, err := tunnel.DeleteManagedForAdmin(ctx, admin, id)
	if err != nil {
		return tunnel.Metadata{}, err
	}
	_ = config.RemoveTunnelMetadata(metadata.ID)
	return metadata, nil
}
