package securemcptunnel

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	tcconfig "github.com/openai/tunnel-client/pkg/config"
	tcadmin "github.com/openai/tunnel-client/pkg/controlplane/admin"

	tracepkg "go.mewis.me/chatgpt-mcp/internal/trace"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

type controlPlane struct{}

func ControlPlane() tunnel.AdminBackend { return controlPlane{} }

func (controlPlane) FetchMetadata(ctx context.Context, cfg tunnel.Config) (tunnel.Metadata, error) {
	return FetchMetadata(ctx, cfg)
}

func (controlPlane) ListManaged(ctx context.Context, cfg tunnel.Config, scope tunnel.AdminScope) ([]tunnel.Metadata, error) {
	return ListManaged(ctx, cfg, scope)
}

func (controlPlane) GetManaged(ctx context.Context, cfg tunnel.Config, id string) (tunnel.Metadata, error) {
	return GetManaged(ctx, cfg, id)
}

func (controlPlane) CreateManaged(ctx context.Context, cfg tunnel.Config, req tunnel.CreateRequest) (tunnel.Metadata, error) {
	return CreateManaged(ctx, cfg, req)
}

func (controlPlane) UpdateManaged(ctx context.Context, cfg tunnel.Config, id string, req tunnel.UpdateRequest) (tunnel.Metadata, error) {
	return UpdateManaged(ctx, cfg, id, req)
}

func (controlPlane) DeleteManaged(ctx context.Context, cfg tunnel.Config, id string) (tunnel.Metadata, error) {
	return DeleteManaged(ctx, cfg, id)
}

func (controlPlane) VerifyAdminKey(ctx context.Context, cfg tunnel.Config) (tunnel.AdminAccess, int, error) {
	return VerifyAdminKey(ctx, cfg)
}

func FetchMetadata(ctx context.Context, cfg tunnel.Config) (tunnel.Metadata, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	id := strings.TrimSpace(cfg.ID)
	span := tracepkg.Start(ctx, "TUNNEL", "tunnel.metadata.fetch", "Fetching tunnel metadata", tracepkg.String("tunnel_id", id), tracepkg.String("method", "GET"), tracepkg.URL("url", adminAPIURL(cfg, "/v1/tunnels/"+url.PathEscape(id), nil)), tracepkg.Bool("runtime_key_configured", strings.TrimSpace(cfg.APIKey) != ""))
	if strings.TrimSpace(cfg.ID) == "" {
		err := errors.New("OpenAI tunnel id is empty")
		span.FailMessage("Tunnel metadata fetch validation failed", err)
		return tunnel.Metadata{}, err
	}
	if strings.TrimSpace(cfg.APIKey) == "" {
		err := errors.New("OpenAI tunnel API key is empty")
		span.FailMessage("Tunnel metadata fetch validation failed", err)
		return tunnel.Metadata{}, err
	}
	client, err := adminTunnelClient(cfg, cfg.APIKey)
	if err != nil {
		span.FailMessage("Tunnel metadata client setup failed", err)
		return tunnel.Metadata{}, err
	}
	value, err := client.GetTunnel(ctx, cfg.ID)
	if err != nil {
		span.FailMessage("Tunnel metadata fetch failed", safeAdminTraceError(err), adminRequestErrorTraceFields(err)...)
		return tunnel.Metadata{}, err
	}
	metadata := metadataFromTunnel(value)
	span.EndMessage("Tunnel metadata fetched", tracepkg.String("tunnel_id", metadata.ID), tracepkg.Int("organization_count", len(metadata.OrganizationIDs)), tracepkg.Int("workspace_count", len(metadata.WorkspaceIDs)), tracepkg.Int("tenant_count", len(metadata.TenantIDs)))
	return metadata, nil
}

func VerifyAdminKey(ctx context.Context, cfg tunnel.Config) (tunnel.AdminAccess, int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	scope := tunnel.AdminScopeFromConfig(cfg)
	span := tracepkg.Start(ctx, "TUNNEL", "tunnel.admin.verify", "Verifying tunnel admin access", append(adminScopeTraceFields(scope), tracepkg.URL("control_plane_url", adminAPIURL(cfg, "/v1/tunnels", adminScopeQuery(scope))))...)
	items, err := ListManaged(ctx, cfg, scope)
	if err == nil {
		span.EndMessage("Tunnel admin access verified", tracepkg.Bool("read_access", true), tracepkg.Bool("manage_access", true), tracepkg.Int("tunnel_count", len(items)), tracepkg.String("verification_method", "list"))
		return tunnel.AdminAccess{Read: true, Manage: true}, len(items), nil
	}
	var requestErr *tcadmin.RequestError
	if !errors.As(err, &requestErr) || requestErr.StatusCode != http.StatusForbidden {
		span.FailMessage("Tunnel admin access verification failed", safeAdminTraceError(err), adminRequestErrorTraceFields(err)...)
		return tunnel.AdminAccess{}, 0, err
	}
	client, clientErr := adminTunnelClient(cfg, cfg.AdminKey)
	if clientErr != nil {
		span.FailMessage("Tunnel admin Read access probe setup failed", clientErr)
		return tunnel.AdminAccess{}, 0, clientErr
	}
	const probeID = "tunnel_00000000000000000000000000000000"
	_, readErr := client.GetTunnel(ctx, probeID)
	if readErr == nil {
		span.EndMessage("Tunnel admin access verified", tracepkg.Bool("read_access", true), tracepkg.Bool("manage_access", false), tracepkg.Int("tunnel_count", 0), tracepkg.String("verification_method", "read_probe"))
		return tunnel.AdminAccess{Read: true}, 0, nil
	}
	var readRequestErr *tcadmin.RequestError
	if errors.As(readErr, &readRequestErr) && (readRequestErr.StatusCode == http.StatusNotFound || readRequestErr.StatusCode == http.StatusBadRequest) {
		span.EndMessage("Tunnel admin access verified", tracepkg.Bool("read_access", true), tracepkg.Bool("manage_access", false), tracepkg.Int("tunnel_count", 0), tracepkg.String("verification_method", "read_probe"))
		return tunnel.AdminAccess{Read: true}, 0, nil
	}
	span.FailMessage("Tunnel admin access verification failed", safeAdminTraceError(err), tracepkg.Bool("read_access", false), tracepkg.Bool("manage_access", false))
	return tunnel.AdminAccess{}, 0, err
}

func ListManaged(ctx context.Context, cfg tunnel.Config, scope tunnel.AdminScope) ([]tunnel.Metadata, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	span := tracepkg.Start(ctx, "TUNNEL", "tunnel.admin.list", "Listing managed tunnels", append(adminScopeTraceFields(scope), tracepkg.String("method", http.MethodGet), tracepkg.URL("url", adminAPIURL(cfg, "/v1/tunnels", adminScopeQuery(scope))))...)
	if strings.TrimSpace(cfg.AdminKey) == "" {
		err := errors.New("OpenAI tunnel admin key is not configured")
		span.FailMessage("Managed tunnel listing failed", err)
		return nil, err
	}
	if err := tunnel.ValidateAdminScope(scope); err != nil {
		span.FailMessage("Managed tunnel listing scope invalid", err)
		return nil, err
	}
	client, err := adminTunnelClient(cfg, cfg.AdminKey)
	if err != nil {
		span.FailMessage("Managed tunnel client setup failed", err)
		return nil, err
	}
	response, err := client.ListTunnels(ctx, strings.TrimSpace(scope.OrganizationID), strings.TrimSpace(scope.WorkspaceID), strings.TrimSpace(scope.TenantID))
	if err != nil {
		span.FailMessage("Managed tunnel listing failed", safeAdminTraceError(err), adminRequestErrorTraceFields(err)...)
		return nil, err
	}
	items := make([]tunnel.Metadata, 0, len(response.Tunnels))
	for index := range response.Tunnels {
		items = append(items, metadataFromTunnel(&response.Tunnels[index]))
	}
	span.EndMessage("Managed tunnels listed", tracepkg.Int("tunnel_count", len(items)))
	return items, nil
}

func GetManaged(ctx context.Context, cfg tunnel.Config, id string) (tunnel.Metadata, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	id = strings.TrimSpace(id)
	if strings.TrimSpace(cfg.AdminKey) == "" {
		return tunnel.Metadata{}, errors.New("OpenAI tunnel admin key is not configured")
	}
	client, err := adminTunnelClient(cfg, cfg.AdminKey)
	if err != nil {
		return tunnel.Metadata{}, err
	}
	value, err := client.GetTunnel(ctx, id)
	if err != nil {
		return tunnel.Metadata{}, err
	}
	return metadataFromTunnel(value), nil
}

func CreateManaged(ctx context.Context, cfg tunnel.Config, req tunnel.CreateRequest) (tunnel.Metadata, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return createWithAdminKey(ctx, cfg, cfg.AdminKey, req)
}

func UpdateManaged(ctx context.Context, cfg tunnel.Config, id string, req tunnel.UpdateRequest) (tunnel.Metadata, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	id = strings.TrimSpace(id)
	if strings.TrimSpace(cfg.AdminKey) == "" {
		return tunnel.Metadata{}, errors.New("OpenAI tunnel admin key is not configured")
	}
	if req.Name == nil && req.Description == nil && req.TenantIDs == nil && req.WorkspaceIDs == nil && req.OrganizationIDs == nil {
		return tunnel.Metadata{}, errors.New("provide at least one field to update")
	}
	client, err := adminTunnelClient(cfg, cfg.AdminKey)
	if err != nil {
		return tunnel.Metadata{}, err
	}
	value, err := client.UpdateTunnel(ctx, id, tcadmin.TunnelUpdateRequest{
		Name: req.Name, Description: req.Description, TenantIDs: req.TenantIDs, WorkspaceIDs: req.WorkspaceIDs, OrganizationIDs: req.OrganizationIDs,
	})
	if err != nil {
		return tunnel.Metadata{}, err
	}
	return metadataFromTunnel(value), nil
}

func DeleteManaged(ctx context.Context, cfg tunnel.Config, id string) (tunnel.Metadata, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	id = strings.TrimSpace(id)
	if strings.TrimSpace(cfg.AdminKey) == "" {
		return tunnel.Metadata{}, errors.New("OpenAI tunnel admin key is not configured")
	}
	client, err := adminTunnelClient(cfg, cfg.AdminKey)
	if err != nil {
		return tunnel.Metadata{}, err
	}
	value, err := client.DeleteTunnel(ctx, id)
	if err != nil {
		return tunnel.Metadata{}, err
	}
	return metadataFromTunnel(value), nil
}

func createWithAdminKey(ctx context.Context, cfg tunnel.Config, apiKey string, req tunnel.CreateRequest) (tunnel.Metadata, error) {
	if strings.TrimSpace(apiKey) == "" {
		return tunnel.Metadata{}, errors.New("OpenAI tunnel admin API key is empty")
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	if req.Name == "" {
		return tunnel.Metadata{}, errors.New("tunnel name is required")
	}
	if req.Description == "" {
		return tunnel.Metadata{}, errors.New("tunnel description is required")
	}
	if len(req.OrganizationIDs) == 0 && len(req.WorkspaceIDs) == 0 {
		return tunnel.Metadata{}, errors.New("at least one organization or workspace id is required")
	}
	client, err := adminTunnelClient(cfg, apiKey)
	if err != nil {
		return tunnel.Metadata{}, err
	}
	value, err := client.CreateTunnel(ctx, tcadmin.TunnelCreateRequest{
		Name: req.Name, Description: req.Description,
		TenantIDs: append([]string(nil), req.TenantIDs...), WorkspaceIDs: append([]string(nil), req.WorkspaceIDs...), OrganizationIDs: append([]string(nil), req.OrganizationIDs...),
	})
	if err != nil {
		return tunnel.Metadata{}, err
	}
	return metadataFromTunnel(value), nil
}

func metadataFromTunnel(value *tcadmin.Tunnel) tunnel.Metadata {
	if value == nil {
		return tunnel.Metadata{}
	}
	return tunnel.Metadata{
		ID: value.ID, Name: value.Name, Description: value.Description, Creator: value.Creator,
		TenantIDs: append([]string(nil), value.TenantIDs...), WorkspaceIDs: append([]string(nil), value.WorkspaceIDs...), OrganizationIDs: append([]string(nil), value.OrganizationIDs...),
		RequestID: value.RequestID, FetchedAt: time.Now().UTC(),
	}
}

func adminScopeQuery(scope tunnel.AdminScope) url.Values {
	query := url.Values{}
	if value := strings.TrimSpace(scope.OrganizationID); value != "" {
		query.Set("organization_id", value)
	}
	if value := strings.TrimSpace(scope.WorkspaceID); value != "" {
		query.Set("workspace_id", value)
	}
	if value := strings.TrimSpace(scope.TenantID); value != "" {
		query.Set("tenant_id", value)
	}
	return query
}

func adminScopeTraceFields(scope tunnel.AdminScope) []tracepkg.Field {
	fields := []tracepkg.Field{}
	if value := strings.TrimSpace(scope.OrganizationID); value != "" {
		fields = append(fields, tracepkg.String("scope_type", "organization"), tracepkg.String("scope_id", value))
	}
	if value := strings.TrimSpace(scope.WorkspaceID); value != "" {
		fields = append(fields, tracepkg.String("scope_type", "workspace"), tracepkg.String("scope_id", value))
	}
	if value := strings.TrimSpace(scope.TenantID); value != "" {
		fields = append(fields, tracepkg.String("scope_type", "tenant"), tracepkg.String("scope_id", value))
	}
	if len(fields) == 0 {
		fields = append(fields, tracepkg.String("scope_type", "unresolved"))
	}
	return fields
}

func adminAPIURL(cfg tunnel.Config, path string, query url.Values) string {
	baseURL := strings.TrimSpace(cfg.ControlPlaneBaseURL)
	if baseURL == "" {
		baseURL = tunnel.DefaultControlPlaneBaseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return baseURL
	}
	target := tcconfig.ResolveControlPlanePath(parsed, "", path)
	if len(query) > 0 {
		target.RawQuery = query.Encode()
	}
	return target.String()
}

func adminRequestErrorTraceFields(err error) []tracepkg.Field {
	var requestErr *tcadmin.RequestError
	if !errors.As(err, &requestErr) {
		return nil
	}
	fields := []tracepkg.Field{tracepkg.String("method", requestErr.Method), tracepkg.Int("status", requestErr.StatusCode), tracepkg.Int64("response_bytes", int64(len(requestErr.ResponseBody)))}
	if requestErr.RequestID != "" {
		fields = append(fields, tracepkg.String("request_id", requestErr.RequestID))
	}
	if requestErr.Code != "" {
		fields = append(fields, tracepkg.String("error_code", requestErr.Code))
	}
	return fields
}

func safeAdminTraceError(err error) error {
	if err == nil {
		return nil
	}
	var requestErr *tcadmin.RequestError
	if errors.As(err, &requestErr) {
		return errors.New("OpenAI tunnel management API request failed")
	}
	return err
}
