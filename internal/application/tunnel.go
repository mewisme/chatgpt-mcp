package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/config"
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
}

type TunnelAdminKeyInput struct {
	Key   string
	Scope *tunnel.AdminScope
}

type ManagedTunnelOptions struct {
	Configure     bool
	RuntimeAPIKey string
	Enable        bool
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
	client := tunnel.NewConfigured(cfg.Tunnel, nil)
	if metadata, err := config.LoadTunnelMetadata(cfg.Tunnel.ID); err == nil {
		_ = client.SeedMetadata(metadata)
	}
	return TunnelDashboard{Config: cfg.Tunnel, Status: client.Status(), MCPHTTPEnabled: cfg.Server.Enabled}, nil
}

func ConfigureTunnelRuntime(ctx context.Context, input TunnelRuntimeInput) (TunnelDashboard, error) {
	load := config.Load
	if input.APIKey != nil {
		load = config.LoadForTunnelRuntimeKeyReplacement
	}
	cfg, err := load()
	if err != nil {
		return TunnelDashboard{}, err
	}
	previousConfig := cfg
	previous := cfg.Tunnel
	next := previous
	if input.Enabled != nil {
		next.Enabled = *input.Enabled
	}
	if input.ID != nil {
		next.ID = strings.TrimSpace(*input.ID)
	}
	if input.APIKey != nil {
		next.APIKey = strings.TrimSpace(*input.APIKey)
	}
	if input.ControlPlaneBaseURL != nil {
		next.ControlPlaneBaseURL = strings.TrimSpace(*input.ControlPlaneBaseURL)
	}
	if input.OrganizationID != nil {
		next.OrganizationID = strings.TrimSpace(*input.OrganizationID)
	}
	cfg.Tunnel = next
	if err := config.Validate(cfg); err != nil {
		return TunnelDashboard{}, err
	}
	metadataSync := tunnel.Configured(next) && (previous.ID != next.ID || previous.APIKey != next.APIKey || previous.ControlPlaneBaseURL != next.ControlPlaneBaseURL)
	if metadataSync {
		if ctx == nil {
			ctx = context.Background()
		}
		if _, _, err := config.SyncTunnelMetadata(ctx, next); err != nil {
			return TunnelDashboard{}, fmt.Errorf("persist tunnel metadata: %w", err)
		}
	}
	if _, _, err := saveConfigMutation(ctx, previousConfig, cfg); err != nil {
		return TunnelDashboard{}, err
	}
	return TunnelStatus()
}

func SetTunnelEnabled(ctx context.Context, enabled bool) (TunnelDashboard, error) {
	previous, err := config.Load()
	if err != nil {
		return TunnelDashboard{}, err
	}
	cfg := previous
	cfg.Tunnel.Enabled = enabled
	if err := config.Validate(cfg); err != nil {
		return TunnelDashboard{}, err
	}
	if _, _, err := saveConfigMutation(ctx, previous, cfg); err != nil {
		return TunnelDashboard{}, err
	}
	return TunnelStatus()
}

func SyncConfiguredTunnel(ctx context.Context) (tunnel.Metadata, string, error) {
	cfg, err := config.Load()
	if err != nil {
		return tunnel.Metadata{}, "", err
	}
	if !tunnel.Configured(cfg.Tunnel) {
		return tunnel.Metadata{}, "", errors.New("configured tunnel id and runtime API key are required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return config.SyncTunnelMetadata(ctx, cfg.Tunnel)
}

func TunnelAdminKeyStatus() (TunnelAdminStatus, error) {
	cfg, err := config.Load()
	if err != nil {
		return TunnelAdminStatus{}, err
	}
	scope := tunnel.AdminScopeFromConfig(cfg.Tunnel)
	return TunnelAdminStatus{Configured: tunnel.AdminConfigured(cfg.Tunnel), Scope: scope}, nil
}

func SetTunnelAdminKey(ctx context.Context, input TunnelAdminKeyInput) (int, tunnel.AdminScope, error) {
	previous, err := config.LoadForTunnelAdminKeyReplacement()
	if err != nil {
		return 0, tunnel.AdminScope{}, err
	}
	cfg := previous
	key := strings.TrimSpace(input.Key)
	if key == "" {
		return 0, tunnel.AdminScope{}, errors.New("OpenAI admin key is required")
	}
	candidate := cfg.Tunnel
	candidate.AdminKey = key
	if ctx == nil {
		ctx = context.Background()
	}
	scope, err := resolveTunnelAdminSetScope(ctx, candidate, input.Scope)
	if err != nil {
		return 0, tunnel.AdminScope{}, fmt.Errorf("admin key verification scope: %w", err)
	}
	tunnel.ApplyAdminScope(&candidate, scope)
	count, err := tunnel.VerifyAdminKey(ctx, candidate)
	if err != nil {
		return 0, tunnel.AdminScope{}, fmt.Errorf("admin key verification failed: %w", err)
	}
	cfg.Tunnel = candidate
	if _, _, err := saveConfigMutation(ctx, previous, cfg); err != nil {
		return 0, tunnel.AdminScope{}, err
	}
	return count, scope, nil
}

func VerifyTunnelAdminKey(ctx context.Context) (int, tunnel.AdminScope, error) {
	cfg, err := config.Load()
	if err != nil {
		return 0, tunnel.AdminScope{}, err
	}
	if !tunnel.AdminConfigured(cfg.Tunnel) {
		return 0, tunnel.AdminScope{}, errors.New("tunnel admin key is not configured; set it first")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	count, err := tunnel.VerifyAdminKey(ctx, cfg.Tunnel)
	return count, tunnel.AdminScopeFromConfig(cfg.Tunnel), err
}

func RemoveTunnelAdminKey(ctx context.Context) error {
	previous, err := config.LoadForTunnelAdminKeyReplacement()
	if err != nil {
		return err
	}
	cfg := previous
	cfg.Tunnel.AdminKey = ""
	tunnel.ApplyAdminScope(&cfg.Tunnel, tunnel.AdminScope{})
	_, _, err = saveConfigMutation(ctx, previous, cfg)
	return err
}

func ListManagedTunnels(ctx context.Context) ([]tunnel.Metadata, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	if !tunnel.AdminConfigured(cfg.Tunnel) {
		return nil, errors.New("verified tunnel admin key is required")
	}
	scope := tunnel.AdminScopeFromConfig(cfg.Tunnel)
	if ctx == nil {
		ctx = context.Background()
	}
	return tunnel.ListManaged(ctx, cfg.Tunnel, scope)
}

func RefreshManagedTunnels(ctx context.Context) ([]tunnel.Metadata, error) {
	items, err := ListManagedTunnels(ctx)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if _, err := config.SaveTunnelMetadata(item); err != nil {
			return nil, err
		}
	}
	return items, nil
}

func GetManagedTunnel(ctx context.Context, id string, options ManagedTunnelOptions) (ManagedTunnelResult, error) {
	cfg, err := config.Load()
	if err != nil {
		return ManagedTunnelResult{}, err
	}
	if !tunnel.AdminConfigured(cfg.Tunnel) {
		return ManagedTunnelResult{}, errors.New("verified tunnel admin key is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	metadata, err := tunnel.GetManaged(ctx, cfg.Tunnel, strings.TrimSpace(id))
	if err != nil {
		return ManagedTunnelResult{}, err
	}
	if _, err := config.SaveTunnelMetadata(metadata); err != nil {
		return ManagedTunnelResult{}, err
	}
	configured := false
	if options.Configure {
		previous := cfg
		if err := configureManagedTunnel(&cfg, metadata, options.RuntimeAPIKey, options.Enable); err != nil {
			return ManagedTunnelResult{}, err
		}
		if _, _, err := saveConfigMutation(ctx, previous, cfg); err != nil {
			return ManagedTunnelResult{}, err
		}
		configured = true
	}
	return ManagedTunnelResult{Metadata: metadata, Configured: configured}, nil
}

func CreateManagedTunnel(ctx context.Context, request tunnel.CreateRequest, options ManagedTunnelOptions) (ManagedTunnelResult, error) {
	cfg, err := config.Load()
	if err != nil {
		return ManagedTunnelResult{}, err
	}
	if !tunnel.AdminConfigured(cfg.Tunnel) {
		return ManagedTunnelResult{}, errors.New("verified tunnel admin key is required")
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
	metadata, err := tunnel.CreateManaged(ctx, cfg.Tunnel, request)
	if err != nil {
		return ManagedTunnelResult{}, err
	}
	if _, err := config.SaveTunnelMetadata(metadata); err != nil {
		return ManagedTunnelResult{}, err
	}
	configured := false
	if options.Configure {
		previous := cfg
		if err := configureManagedTunnel(&cfg, metadata, options.RuntimeAPIKey, options.Enable); err != nil {
			return ManagedTunnelResult{}, err
		}
		if _, _, err := saveConfigMutation(ctx, previous, cfg); err != nil {
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
	if ctx == nil {
		ctx = context.Background()
	}
	metadata, err := tunnel.UpdateManaged(ctx, cfg.Tunnel, strings.TrimSpace(id), request)
	if err != nil {
		return ManagedTunnelResult{}, err
	}
	if _, err := config.SaveTunnelMetadata(metadata); err != nil {
		return ManagedTunnelResult{}, err
	}
	configured := false
	if options.Configure {
		previous := cfg
		if err := configureManagedTunnel(&cfg, metadata, options.RuntimeAPIKey, options.Enable); err != nil {
			return ManagedTunnelResult{}, err
		}
		if _, _, err := saveConfigMutation(ctx, previous, cfg); err != nil {
			return ManagedTunnelResult{}, err
		}
		configured = true
	}
	return ManagedTunnelResult{Metadata: metadata, Configured: configured}, nil
}

func DeleteManagedTunnel(ctx context.Context, id string, clearConfig bool) (ManagedTunnelResult, error) {
	cfg, err := config.Load()
	if err != nil {
		return ManagedTunnelResult{}, err
	}
	if !tunnel.AdminConfigured(cfg.Tunnel) {
		return ManagedTunnelResult{}, errors.New("verified tunnel admin key is required")
	}
	id = strings.TrimSpace(id)
	configuredTunnel := id != "" && id == strings.TrimSpace(cfg.Tunnel.ID)
	if configuredTunnel && !cfg.Server.Enabled {
		return ManagedTunnelResult{}, errors.New("cannot delete the configured OpenAI tunnel while MCP HTTP is disabled")
	}
	clearConfigured := clearConfig && configuredTunnel
	var clearedConfig config.Config
	if clearConfigured {
		clearedConfig = cfg
		clearedConfig.Tunnel.Enabled = false
		clearedConfig.Tunnel.ID = ""
		clearedConfig.Tunnel.APIKey = ""
		clearedConfig.Tunnel.OrganizationID = ""
		if err := config.Validate(clearedConfig); err != nil {
			return ManagedTunnelResult{}, err
		}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	metadata, err := tunnel.DeleteManaged(ctx, cfg.Tunnel, id)
	if err != nil {
		return ManagedTunnelResult{}, err
	}
	cleared := clearConfigured && cfg.Tunnel.ID == metadata.ID
	if cleared {
		if _, _, err := saveConfigMutationWithoutRollback(ctx, clearedConfig); err != nil {
			return ManagedTunnelResult{}, err
		}
	}
	if err := config.RemoveTunnelMetadata(metadata.ID); err != nil {
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
	key := strings.TrimSpace(runtimeAPIKey)
	if key == "" {
		key = strings.TrimSpace(cfg.Tunnel.APIKey)
	}
	if key == "" {
		return errors.New("runtime API key is required to configure cgm")
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

func resolveTunnelAdminSetScope(ctx context.Context, cfg tunnel.Config, explicit *tunnel.AdminScope) (tunnel.AdminScope, error) {
	if explicit != nil {
		scope := tunnel.AdminScope{OrganizationID: strings.TrimSpace(explicit.OrganizationID), WorkspaceID: strings.TrimSpace(explicit.WorkspaceID), TenantID: strings.TrimSpace(explicit.TenantID)}
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
		{OrganizationID: singleID(metadata.OrganizationIDs)},
		{WorkspaceID: singleID(metadata.WorkspaceIDs)},
		{TenantID: singleID(metadata.TenantIDs)},
	} {
		if tunnel.ValidateAdminScope(candidate) == nil {
			return candidate, nil
		}
	}
	return tunnel.AdminScope{}, errors.New("configured tunnel does not expose one unambiguous admin scope")
}

func singleID(values []string) string {
	if len(values) == 1 {
		return strings.TrimSpace(values[0])
	}
	return ""
}
