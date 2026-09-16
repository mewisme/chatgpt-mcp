package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/config"
	tracepkg "go.mewis.me/chatgpt-mcp/internal/trace"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

type TunnelDashboard struct {
	Config         tunnel.Config
	Status         tunnel.Status
	MCPHTTPEnabled bool
}

type TunnelRuntimeInput struct {
	Enabled             *bool
	ID                  *string
	APIKey              *string
	ControlPlaneBaseURL *string
	OrganizationID      *string
}

type TunnelAdminStatus struct {
	Configured bool
	Scope      tunnel.AdminScope
	Access     tunnel.AdminAccess
}

type TunnelAdminKeyInput struct {
	Key       string
	KeySource string
	Scope     *tunnel.AdminScope
}

type ManagedTunnelOptions struct {
	Configure     bool
	RuntimeAPIKey string
	Enable        bool
}

type ManagedTunnelUseOptions struct {
	RuntimeAPIKey          string
	AutoGenerateRuntimeKey bool
	ProjectID              string
}

type ManagedTunnelResult struct {
	Metadata   tunnel.Metadata
	Configured bool
	Cleared    bool
}

func TunnelStatus() (TunnelDashboard, error) {
	cfg, err := config.Load()
	if err != nil {
		return TunnelDashboard{}, err
	}
	return dashboardFromConfig(cfg), nil
}

func ConfigureTunnelRuntime(ctx context.Context, input TunnelRuntimeInput) (TunnelDashboard, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	span := tracepkg.Start(ctx, "TUNNEL", "tunnel.runtime.configure", "Configuring local tunnel runtime", tracepkg.Any("changed_fields", tunnelRuntimeInputFields(input)), tracepkg.Bool("runtime_key_replacement", input.APIKey != nil))
	load := config.Load
	if input.APIKey != nil {
		load = config.LoadForTunnelRuntimeKeyReplacement
	}
	cfg, _, err := loadConfigWithTracedLoader(ctx, "tunnel.config.load", "Loading tunnel runtime configuration", load)
	if err != nil {
		span.FailMessage("Tunnel runtime configuration load failed", err)
		return TunnelDashboard{}, err
	}
	collection := cfg.RuntimeTunnels()
	instance, index, found := primaryLocalInstance(collection)
	if input.ID != nil {
		id := strings.TrimSpace(*input.ID)
		if existing := localInstanceIndex(collection, id); existing >= 0 {
			instance, index, found = collection.Instances[existing], existing, true
		} else {
			instance.ID = id
		}
	}
	previous := instanceRuntimeConfig(instance)
	if found {
		previous = instanceRuntimeConfig(collection.Instances[index])
	} else {
		previous = tunnel.Config{}
	}
	if input.Enabled != nil {
		instance.Enabled = *input.Enabled
	}
	if input.APIKey != nil {
		instance.APIKey = strings.TrimSpace(*input.APIKey)
	}
	if input.ControlPlaneBaseURL != nil {
		instance.ControlPlaneBaseURL = strings.TrimSpace(*input.ControlPlaneBaseURL)
	}
	if input.OrganizationID != nil {
		instance.OrganizationID = strings.TrimSpace(*input.OrganizationID)
	}
	next := instanceRuntimeConfig(instance)
	metadataSync := tunnel.Configured(next) && (previous.ID != next.ID || previous.APIKey != next.APIKey || previous.ControlPlaneBaseURL != next.ControlPlaneBaseURL)
	tracepkg.Emit(ctx, "TUNNEL", "tunnel.runtime.metadata-sync-decision", "Resolved tunnel metadata synchronization decision", tracepkg.Bool("metadata_sync", metadataSync), tracepkg.String("tunnel_id", next.ID), tracepkg.URL("control_plane_base_url", next.ControlPlaneBaseURL), tracepkg.Bool("runtime_auth_present", strings.TrimSpace(next.APIKey) != ""))
	if metadataSync {
		if _, _, err := config.SyncTunnelMetadata(ctx, next); err != nil {
			span.FailMessage("Tunnel runtime metadata synchronization failed", err, tracepkg.Bool("metadata_sync", true))
			return TunnelDashboard{}, fmt.Errorf("persist tunnel metadata: %w", err)
		}
	}
	if found {
		collection.Instances[index] = instance
	} else if strings.TrimSpace(instance.ID) != "" {
		collection.Instances = append(collection.Instances, instance)
	}
	if err := saveTunnelCollection(ctx, cfg, collection); err != nil {
		span.FailMessage("Tunnel runtime configuration persistence failed", err, tracepkg.Bool("metadata_sync", metadataSync))
		return TunnelDashboard{}, err
	}
	dashboard, err := TunnelStatus()
	if err != nil {
		span.FailMessage("Tunnel runtime status reload failed", err)
		return TunnelDashboard{}, err
	}
	span.EndMessage("Local tunnel runtime configured", tracepkg.String("tunnel_id", dashboard.Config.ID), tracepkg.Bool("enabled", dashboard.Config.Enabled), tracepkg.Bool("metadata_sync", metadataSync), tracepkg.URL("control_plane_base_url", dashboard.Config.ControlPlaneBaseURL), tracepkg.Bool("runtime_auth_present", strings.TrimSpace(dashboard.Config.APIKey) != ""))
	return dashboard, nil
}

func SetTunnelEnabled(ctx context.Context, enabled bool) (TunnelDashboard, error) {
	previous, err := config.Load()
	if err != nil {
		return TunnelDashboard{}, err
	}
	instance, _, ok := primaryLocalInstance(previous.RuntimeTunnels())
	if !ok || strings.TrimSpace(instance.ID) == "" {
		return TunnelDashboard{}, errors.New("tunnel id is required")
	}
	if _, err := SetLocalTunnelEnabled(ctx, instance.ID, enabled); err != nil {
		return TunnelDashboard{}, err
	}
	return TunnelStatus()
}

func SyncConfiguredTunnel(ctx context.Context) (tunnel.Metadata, string, error) {
	cfg, err := config.Load()
	if err != nil {
		return tunnel.Metadata{}, "", err
	}
	instance, _, ok := primaryLocalInstance(cfg.RuntimeTunnels())
	runtime := instanceRuntimeConfig(instance)
	if !ok || !tunnel.Configured(runtime) {
		return tunnel.Metadata{}, "", errors.New("configured tunnel id and runtime API key are required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return config.SyncTunnelMetadata(ctx, runtime)
}

func TunnelAdminKeyStatus() (TunnelAdminStatus, error) {
	return TunnelAdminKeyStatusContext(context.Background())
}

func TunnelAdminKeyStatusContext(ctx context.Context) (TunnelAdminStatus, error) {
	span := tracepkg.Start(ctx, "TUNNEL", "tunnel.admin-key.status", "Loading stored tunnel admin key status")
	cfg, _, err := loadConfigTraced(ctx, "tunnel.admin.config.load", "Loading tunnel admin configuration")
	if err != nil {
		span.FailMessage("Stored tunnel admin key status load failed", err)
		return TunnelAdminStatus{}, err
	}
	runtime := dashboardFromConfig(cfg).Config
	scope := tunnel.AdminScopeFromConfig(runtime)
	status := TunnelAdminStatus{Configured: tunnel.AdminConfigured(runtime), Scope: scope, Access: tunnel.AdminAccessFromConfig(runtime)}
	fields := append(tunnelAdminScopeFields(scope), tracepkg.Bool("configured", status.Configured), tracepkg.Bool("read_access", status.Access.Read), tracepkg.Bool("manage_access", status.Access.Manage))
	span.EndMessage("Stored tunnel admin key status loaded", fields...)
	return status, nil
}

func SetTunnelAdminKey(ctx context.Context, input TunnelAdminKeyInput) (int, tunnel.AdminScope, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	keySource := strings.TrimSpace(input.KeySource)
	if keySource == "" {
		keySource = "provided"
	}
	fields := []tracepkg.Field{tracepkg.String("key_source", keySource), tracepkg.Bool("requested_scope_explicit", input.Scope != nil)}
	if input.Scope != nil {
		fields = append(fields, tunnelAdminScopeFields(*input.Scope)...)
	}
	span := tracepkg.Start(ctx, "TUNNEL", "tunnel.admin-key.set", "Verifying and storing tunnel admin access", fields...)
	previous, _, err := loadConfigWithTracedLoader(ctx, "tunnel.admin.config.load", "Loading tunnel admin configuration", config.LoadForTunnelAdminKeyReplacement)
	if err != nil {
		span.FailMessage("Tunnel admin configuration load failed", err)
		return 0, tunnel.AdminScope{}, err
	}
	key := strings.TrimSpace(input.Key)
	if key == "" {
		err := errors.New("OpenAI admin key is required")
		span.FailMessage("Tunnel admin key validation failed", err, tracepkg.String("key_source", keySource))
		return 0, tunnel.AdminScope{}, err
	}
	collection := previous.RuntimeTunnels()
	admin, exists := primaryAdminProfile(collection)
	if !exists {
		admin.ID = "default"
		if instance, _, ok := primaryLocalInstance(collection); ok {
			admin.ControlPlaneBaseURL = instance.ControlPlaneBaseURL
		}
	}
	admin.AdminKey = key
	if input.Scope != nil {
		admin.OrganizationID = strings.TrimSpace(input.Scope.OrganizationID)
		admin.WorkspaceID = strings.TrimSpace(input.Scope.WorkspaceID)
		admin.TenantID = strings.TrimSpace(input.Scope.TenantID)
	} else if !exists {
		candidate := instanceRuntimeConfig(tunnel.InstanceConfig{})
		if instance, _, ok := primaryLocalInstance(collection); ok {
			candidate = instanceRuntimeConfig(instance)
		}
		candidate.AdminKey = key
		scope, derivation, scopeErr := resolveTunnelAdminSetScope(ctx, candidate, nil)
		if scopeErr != nil {
			span.FailMessage("Tunnel admin verification scope resolution failed", scopeErr, tracepkg.String("scope_derivation", derivation))
			return 0, tunnel.AdminScope{}, fmt.Errorf("admin key verification scope: %w", scopeErr)
		}
		admin.OrganizationID, admin.WorkspaceID, admin.TenantID = scope.OrganizationID, scope.WorkspaceID, scope.TenantID
	}
	var count int
	if exists {
		_, count, err = UpdateTunnelAdminProfile(ctx, admin)
	} else {
		_, count, err = AddTunnelAdminProfile(ctx, admin)
	}
	if err != nil {
		span.FailMessage("Tunnel admin access persistence failed", err)
		return 0, tunnel.AdminScope{}, err
	}
	scope := tunnel.AdminScope{OrganizationID: admin.OrganizationID, WorkspaceID: admin.WorkspaceID, TenantID: admin.TenantID}
	span.EndMessage("Tunnel admin access verified and stored", append(tunnelAdminScopeFields(scope), tracepkg.String("key_source", keySource), tracepkg.Int("tunnel_count", count))...)
	return count, scope, nil
}

func VerifyTunnelAdminKey(ctx context.Context) (int, tunnel.AdminScope, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	span := tracepkg.Start(ctx, "TUNNEL", "tunnel.admin-key.verify-stored", "Verifying stored tunnel admin access", tracepkg.String("key_source", "stored"))
	cfg, _, err := loadConfigTraced(ctx, "tunnel.admin.config.load", "Loading stored tunnel admin configuration")
	if err != nil {
		span.FailMessage("Stored tunnel admin configuration load failed", err)
		return 0, tunnel.AdminScope{}, err
	}
	collection := cfg.RuntimeTunnels()
	admin, ok := primaryAdminProfile(collection)
	if !ok {
		err := errors.New("tunnel admin key is not configured; set it first")
		span.FailMessage("Stored tunnel admin access is not configured", err)
		return 0, tunnel.AdminScope{}, err
	}
	profile, count, err := VerifyTunnelAdminProfile(ctx, admin.ID)
	if err != nil {
		span.FailMessage("Stored tunnel admin access verification failed", errors.New("tunnel admin verification failed"))
		return 0, tunnel.AdminScope{OrganizationID: admin.OrganizationID, WorkspaceID: admin.WorkspaceID, TenantID: admin.TenantID}, err
	}
	scope := tunnel.AdminScope{OrganizationID: profile.OrganizationID, WorkspaceID: profile.WorkspaceID, TenantID: profile.TenantID}
	span.EndMessage("Stored tunnel admin access verified", append(tunnelAdminScopeFields(scope), tracepkg.Bool("read_access", profile.ReadAccess), tracepkg.Bool("manage_access", profile.ManageAccess), tracepkg.Int("tunnel_count", count))...)
	return count, scope, nil
}

func RemoveTunnelAdminKey(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	span := tracepkg.Start(ctx, "TUNNEL", "tunnel.admin-key.remove", "Removing stored tunnel admin key")
	previous, _, err := loadConfigWithTracedLoader(ctx, "tunnel.admin.config.load", "Loading tunnel admin configuration", config.LoadForTunnelAdminKeyReplacement)
	if err != nil {
		span.FailMessage("Tunnel admin key removal configuration load failed", err)
		return err
	}
	collection := previous.RuntimeTunnels()
	admin, ok := primaryAdminProfile(collection)
	if !ok {
		span.EndMessage("Stored tunnel admin key removed", tracepkg.Bool("runtime_reloaded", false))
		return nil
	}
	for i, instance := range collection.Instances {
		if instance.AdminProfileID == admin.ID {
			collection.Instances[i].AdminProfileID = ""
		}
	}
	filtered := collection.Admins[:0]
	for _, item := range collection.Admins {
		if item.ID != admin.ID {
			filtered = append(filtered, item)
		}
	}
	collection.Admins = append([]tunnel.AdminConfig{}, filtered...)
	if err := saveTunnelCollection(ctx, previous, collection); err != nil {
		span.FailMessage("Tunnel admin key removal failed", err)
		return err
	}
	span.EndMessage("Stored tunnel admin key removed", tracepkg.Bool("runtime_reloaded", true))
	return nil
}

func ListManagedTunnels(ctx context.Context) ([]tunnel.Metadata, error) {
	items, err := DiscoverManagedTunnels(ctx, "")
	if err != nil {
		return nil, err
	}
	result := make([]tunnel.Metadata, 0, len(items))
	for _, item := range items {
		result = append(result, item.Metadata)
	}
	return result, nil
}

func RefreshManagedTunnels(ctx context.Context) ([]tunnel.Metadata, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	items, err := ListManagedTunnels(ctx)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if _, err := config.SaveTunnelMetadataContext(ctx, item); err != nil {
			return nil, err
		}
	}
	return items, nil
}

func GetManagedTunnel(ctx context.Context, id string, options ManagedTunnelOptions) (ManagedTunnelResult, error) {
	discovery, err := GetManagedTunnelByProfile(ctx, strings.TrimSpace(id), "")
	if err != nil {
		return ManagedTunnelResult{}, err
	}
	if _, err := config.SaveTunnelMetadataContext(ctx, discovery.Metadata); err != nil {
		return ManagedTunnelResult{}, err
	}
	configured := false
	if options.Configure {
		if _, err := AttachManagedTunnelWithOptions(ctx, discovery.Metadata.ID, AttachManagedTunnelOptions{AdminProfileID: firstNonEmpty(discovery.AdminProfiles...), RuntimeAPIKey: options.RuntimeAPIKey, Enabled: options.Enable}); err != nil {
			return ManagedTunnelResult{}, err
		}
		configured = true
	}
	return ManagedTunnelResult{Metadata: discovery.Metadata, Configured: configured}, nil
}

func UseManagedTunnel(ctx context.Context, id string, options ManagedTunnelUseOptions) (ManagedTunnelResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	span := tracepkg.Start(ctx, "TUNNEL", "tunnel.managed.use", "Selecting managed tunnel for local runtime", tracepkg.String("tunnel_id", strings.TrimSpace(id)), tracepkg.Bool("runtime_key_supplied", strings.TrimSpace(options.RuntimeAPIKey) != ""), tracepkg.Bool("automatic_generation_requested", options.AutoGenerateRuntimeKey), tracepkg.Bool("project_explicit", strings.TrimSpace(options.ProjectID) != ""))
	id = strings.TrimSpace(id)
	if id == "" {
		err := errors.New("managed tunnel id is required")
		span.FailMessage("Managed tunnel selection validation failed", err)
		return ManagedTunnelResult{}, err
	}
	item, err := AttachManagedTunnelWithOptions(ctx, id, AttachManagedTunnelOptions{RuntimeAPIKey: options.RuntimeAPIKey, AutoGenerateRuntimeKey: options.AutoGenerateRuntimeKey, ProjectID: options.ProjectID, Enabled: true})
	if err != nil {
		span.FailMessage("Managed tunnel local configuration failed", err)
		return ManagedTunnelResult{}, err
	}
	result := ManagedTunnelResult{Configured: true}
	if item.Status.Metadata != nil {
		result.Metadata = *item.Status.Metadata
	}
	span.EndMessage("Managed tunnel selected for local runtime", tracepkg.String("tunnel_id", id), tracepkg.Bool("enabled", true))
	return result, nil
}

func CreateManagedTunnel(ctx context.Context, request tunnel.CreateRequest, options ManagedTunnelOptions) (ManagedTunnelResult, error) {
	cfg, err := config.Load()
	if err != nil {
		return ManagedTunnelResult{}, err
	}
	if !tunnel.AdminConfigured(cfg.Tunnel) {
		return ManagedTunnelResult{}, errors.New("verified tunnel admin key is required")
	}
	if !cfg.Tunnel.AdminManageAccess {
		return ManagedTunnelResult{}, errors.New("tunnel admin key does not have verified Manage access")
	}
	request.OrganizationIDs = NormalizeTunnelIDs(request.OrganizationIDs)
	request.WorkspaceIDs = NormalizeTunnelIDs(request.WorkspaceIDs)
	request.TenantIDs = NormalizeTunnelIDs(request.TenantIDs)
	if len(request.OrganizationIDs) == 0 && len(request.WorkspaceIDs) == 0 {
		scope := tunnel.AdminScopeFromConfig(cfg.Tunnel)
		if scope.OrganizationID != "" {
			request.OrganizationIDs = []string{scope.OrganizationID}
		} else if scope.WorkspaceID != "" {
			request.WorkspaceIDs = []string{scope.WorkspaceID}
		}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	tracepkg.Emit(ctx, "TUNNEL", "tunnel.managed.local-config-decision", "Resolved managed tunnel local configuration decision", tracepkg.Bool("configure", options.Configure), tracepkg.Bool("enable", options.Enable), tracepkg.Bool("runtime_key_supplied", strings.TrimSpace(options.RuntimeAPIKey) != ""), tracepkg.String("operation", "create"))
	metadata, err := tunnel.CreateManaged(ctx, cfg.Tunnel, request)
	if err != nil {
		return ManagedTunnelResult{}, err
	}
	if _, err := config.SaveTunnelMetadataContext(ctx, metadata); err != nil {
		return ManagedTunnelResult{}, err
	}
	configured := false
	if options.Configure {
		if err := configureManagedTunnel(&cfg, metadata, options.RuntimeAPIKey, options.Enable); err != nil {
			return ManagedTunnelResult{}, err
		}
		configured = true
	}
	return ManagedTunnelResult{Metadata: metadata, Configured: configured}, nil
}

func UpdateManagedTunnel(ctx context.Context, id string, request tunnel.UpdateRequest, options ManagedTunnelOptions) (ManagedTunnelResult, error) {
	cfg, err := config.Load()
	if err != nil {
		return ManagedTunnelResult{}, err
	}
	if !tunnel.AdminConfigured(cfg.Tunnel) {
		return ManagedTunnelResult{}, errors.New("verified tunnel admin key is required")
	}
	if !cfg.Tunnel.AdminManageAccess {
		return ManagedTunnelResult{}, errors.New("tunnel admin key does not have verified Manage access")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	tracepkg.Emit(ctx, "TUNNEL", "tunnel.managed.local-config-decision", "Resolved managed tunnel local configuration decision", tracepkg.String("tunnel_id", strings.TrimSpace(id)), tracepkg.Bool("configure", options.Configure), tracepkg.Bool("enable", options.Enable), tracepkg.Bool("runtime_key_supplied", strings.TrimSpace(options.RuntimeAPIKey) != ""), tracepkg.String("operation", "update"))
	metadata, err := tunnel.UpdateManaged(ctx, cfg.Tunnel, strings.TrimSpace(id), request)
	if err != nil {
		return ManagedTunnelResult{}, err
	}
	if _, err := config.SaveTunnelMetadataContext(ctx, metadata); err != nil {
		return ManagedTunnelResult{}, err
	}
	configured := false
	if options.Configure {
		if err := configureManagedTunnel(&cfg, metadata, options.RuntimeAPIKey, options.Enable); err != nil {
			return ManagedTunnelResult{}, err
		}
		configured = true
	}
	return ManagedTunnelResult{Metadata: metadata, Configured: configured}, nil
}

func DeleteManagedTunnel(ctx context.Context, id string, clearConfig bool) (ManagedTunnelResult, error) {
	id = strings.TrimSpace(id)
	cleared := false
	if clearConfig {
		if err := DetachLocalTunnel(ctx, id); err != nil && !strings.Contains(err.Error(), "not attached") {
			return ManagedTunnelResult{}, err
		} else if err == nil {
			cleared = true
		}
	}
	metadata, err := DeleteManagedTunnelByProfile(ctx, id, "")
	if err != nil {
		return ManagedTunnelResult{}, err
	}
	return ManagedTunnelResult{Metadata: metadata, Cleared: cleared}, nil
}

func NormalizeTunnelIDs(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func configureManagedTunnel(cfg *config.Config, metadata tunnel.Metadata, runtimeAPIKey string, enable bool) error {
	if cfg == nil {
		return errors.New("configuration is unavailable")
	}
	_, err := AttachManagedTunnelWithOptions(context.Background(), metadata.ID, AttachManagedTunnelOptions{RuntimeAPIKey: runtimeAPIKey, Enabled: enable})
	return err
}

func resolveTunnelAdminSetScope(ctx context.Context, cfg tunnel.Config, explicit *tunnel.AdminScope) (tunnel.AdminScope, string, error) {
	if explicit != nil {
		scope := tunnel.AdminScope{OrganizationID: strings.TrimSpace(explicit.OrganizationID), WorkspaceID: strings.TrimSpace(explicit.WorkspaceID), TenantID: strings.TrimSpace(explicit.TenantID)}
		return scope, "explicit", tunnel.ValidateAdminScope(scope)
	}
	if scope := tunnel.AdminScopeFromConfig(cfg); tunnel.ValidateAdminScope(scope) == nil {
		return scope, "stored", nil
	}
	if strings.TrimSpace(cfg.ID) == "" {
		return tunnel.AdminScope{}, "unresolved", errors.New("provide exactly one admin scope or configure a tunnel first")
	}
	metadata, err := tunnel.GetManaged(ctx, cfg, cfg.ID)
	if err != nil {
		return tunnel.AdminScope{}, "configured_tunnel", fmt.Errorf("derive admin scope from configured tunnel: %w", err)
	}
	for _, candidate := range []tunnel.AdminScope{
		{OrganizationID: singleID(metadata.OrganizationIDs)},
		{WorkspaceID: singleID(metadata.WorkspaceIDs)},
		{TenantID: singleID(metadata.TenantIDs)},
	} {
		if tunnel.ValidateAdminScope(candidate) == nil {
			return candidate, "configured_tunnel", nil
		}
	}
	return tunnel.AdminScope{}, "configured_tunnel", errors.New("configured tunnel does not expose one unambiguous admin scope")
}

func tunnelRuntimeInputFields(input TunnelRuntimeInput) []string {
	fields := []string{}
	if input.Enabled != nil {
		fields = append(fields, "enabled")
	}
	if input.ID != nil {
		fields = append(fields, "id")
	}
	if input.APIKey != nil {
		fields = append(fields, "runtime_key")
	}
	if input.ControlPlaneBaseURL != nil {
		fields = append(fields, "control_plane_base_url")
	}
	if input.OrganizationID != nil {
		fields = append(fields, "organization_id")
	}
	return fields
}

func tunnelAdminScopeFields(scope tunnel.AdminScope) []tracepkg.Field {
	if value := strings.TrimSpace(scope.OrganizationID); value != "" {
		return []tracepkg.Field{tracepkg.String("scope_type", "organization"), tracepkg.String("scope_id", value)}
	}
	if value := strings.TrimSpace(scope.WorkspaceID); value != "" {
		return []tracepkg.Field{tracepkg.String("scope_type", "workspace"), tracepkg.String("scope_id", value)}
	}
	if value := strings.TrimSpace(scope.TenantID); value != "" {
		return []tracepkg.Field{tracepkg.String("scope_type", "tenant"), tracepkg.String("scope_id", value)}
	}
	return []tracepkg.Field{tracepkg.String("scope_type", "unresolved")}
}

func singleID(values []string) string {
	if len(values) == 1 {
		return strings.TrimSpace(values[0])
	}
	return ""
}
