package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/mcp"
	"go.mewis.me/chatgpt-mcp/internal/plugindev"
	"go.mewis.me/chatgpt-mcp/internal/runtimeplugin"
	"go.mewis.me/chatgpt-mcp/internal/tools"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
	"go.mewis.me/chatgpt-mcp/internal/tunnelprovider"
)

type pluginAdmin struct {
	session *runtimeplugin.Session
}

func BindSecureMCPAdmin(host *runtimeplugin.Host) {
	tunnel.SetAdminResolver(func() (tunnel.AdminBackend, error) {
		return EnsureSecureMCPAdmin(context.Background(), host)
	})
}

func EnsureSecureMCPAdmin(ctx context.Context, host *runtimeplugin.Host) (tunnel.AdminBackend, error) {
	session, err := ensureSecureMCPSession(ctx, host)
	if err != nil {
		return nil, err
	}
	return pluginAdmin{session: session}, nil
}

func ReconcileSecureMCP(ctx context.Context, cfg config.Config, host *runtimeplugin.Host, bridge *mcp.PrivateBridge) error {
	if host == nil || bridge == nil {
		return nil
	}
	session, err := ensureSecureMCPSession(ctx, host)
	if err != nil {
		if errors.Is(err, tunnel.ErrPluginMissing) {
			return nil
		}
		return err
	}
	tunnel.SetAdminBackend(pluginAdmin{session: session})
	_, err = session.Call(ctx, runtimeplugin.MethodReconcile, runtimeplugin.ReconcileParams{
		BridgeURL: bridge.URL, BridgeToken: bridge.Token, Collection: collectionPayload(cfg.RuntimeTunnels()),
	})
	return err
}

func StartPrivateSecureMCP(ctx context.Context, cfg config.Config, host *runtimeplugin.Host, runtime *tools.Runtime) (*mcp.PrivateBridge, error) {
	if runtime == nil || len(cfg.RuntimeTunnels().Instances) == 0 {
		return nil, nil
	}
	if _, err := ensureSecureMCPSession(ctx, host); err != nil {
		if errors.Is(err, tunnel.ErrPluginMissing) {
			return nil, nil
		}
		return nil, err
	}
	bridge, err := mcp.StartPrivateBridge(runtime)
	if err != nil {
		return nil, err
	}
	if err := ReconcileSecureMCP(ctx, cfg, host, bridge); err != nil {
		_ = bridge.Close()
		return nil, err
	}
	return bridge, nil
}

func SecureMCPRuntimeStatuses(ctx context.Context, host *runtimeplugin.Host) ([]tunnel.Status, error) {
	session, ok := host.Get(tunnel.PluginIDSecureMCP)
	if !ok {
		return nil, nil
	}
	raw, err := session.Call(ctx, runtimeplugin.MethodInvoke, runtimeplugin.InvokeParams{Op: "runtime_status"})
	if err != nil {
		return nil, err
	}
	var statuses []tunnel.Status
	if err := json.Unmarshal(raw, &statuses); err != nil {
		return nil, err
	}
	return statuses, nil
}

func StartSecureMCPInstance(ctx context.Context, host *runtimeplugin.Host, id string) (tunnel.Status, error) {
	return mutateSecureMCPInstance(ctx, host, "start_instance", id)
}

func StopSecureMCPInstance(ctx context.Context, host *runtimeplugin.Host, serverEnabled bool, id string) (tunnel.Status, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return tunnel.Status{}, errors.New("tunnel id is required")
	}
	if _, ok := host.Get(tunnel.PluginIDSecureMCP); !ok {
		return tunnel.Status{}, tunnel.PluginMissingError()
	}
	statuses, err := SecureMCPRuntimeStatuses(ctx, host)
	if err != nil {
		return tunnel.Status{}, err
	}
	if err := ErrIfLastUsableMCPTransport(serverEnabled, statuses, id); err != nil {
		return tunnel.Status{}, err
	}
	return mutateSecureMCPInstance(ctx, host, "stop_instance", id)
}

func ErrIfLastUsableMCPTransport(serverEnabled bool, statuses []tunnel.Status, id string) error {
	if serverEnabled {
		return nil
	}
	for _, item := range statuses {
		if item.ID != id && item.Enabled && item.Ready {
			return nil
		}
	}
	return errors.New("cannot stop the last usable MCP transport")
}

func mutateSecureMCPInstance(ctx context.Context, host *runtimeplugin.Host, op, id string) (tunnel.Status, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return tunnel.Status{}, errors.New("tunnel id is required")
	}
	session, ok := host.Get(tunnel.PluginIDSecureMCP)
	if !ok {
		return tunnel.Status{}, tunnel.PluginMissingError()
	}
	raw, err := session.Call(ctx, runtimeplugin.MethodInvoke, runtimeplugin.InvokeParams{Op: op, Payload: map[string]string{"id": id}})
	if err != nil {
		return tunnel.Status{}, err
	}
	var status tunnel.Status
	if err := json.Unmarshal(raw, &status); err != nil {
		return tunnel.Status{}, err
	}
	return status, nil
}

func WaitSecureMCPReady(ctx context.Context, host *runtimeplugin.Host) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := host.Get(tunnel.PluginIDSecureMCP); !ok {
		return tunnel.PluginMissingError()
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		statuses, err := SecureMCPRuntimeStatuses(ctx, host)
		if err != nil {
			return err
		}
		for _, item := range statuses {
			if item.Enabled && item.Ready {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func ensureSecureMCPSession(ctx context.Context, host *runtimeplugin.Host) (*runtimeplugin.Session, error) {
	if host == nil {
		return nil, tunnel.PluginMissingError()
	}
	ref, err := LookupTunnelProvider(tunnelprovider.ProviderSecureMCP)
	if err != nil || strings.TrimSpace(ref.Path) == "" {
		return nil, tunnel.PluginMissingError()
	}
	return host.Ensure(ctx, runtimeplugin.Spec{
		ID: string(ref.PluginID), Version: string(ref.Version), Entrypoint: ref.Path, WorkDir: ref.WorkDir,
		DataDir: plugindev.RuntimeLayout().PluginRuntimeDataDir(ref.PluginID),
	})
}

func collectionPayload(cfg tunnel.CollectionConfig) map[string]any {
	instances := make([]map[string]any, 0, len(cfg.Instances))
	for _, item := range cfg.Instances {
		instances = append(instances, map[string]any{
			"enabled": item.Enabled, "id": item.ID, "api_key": item.APIKey,
			"control_plane_base_url": item.ControlPlaneBaseURL, "organization_id": item.OrganizationID,
		})
	}
	return map[string]any{"instances": instances}
}

func (a pluginAdmin) invoke(ctx context.Context, op string, payload any, out any) error {
	raw, err := a.session.Call(ctx, runtimeplugin.MethodInvoke, runtimeplugin.InvokeParams{Op: op, Payload: payload})
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

func (a pluginAdmin) FetchMetadata(ctx context.Context, cfg tunnel.Config) (tunnel.Metadata, error) {
	var metadata tunnel.Metadata
	return metadata, a.invoke(ctx, "fetch_metadata", cfg, &metadata)
}

func (a pluginAdmin) ListManaged(ctx context.Context, cfg tunnel.Config, scope tunnel.AdminScope) ([]tunnel.Metadata, error) {
	var items []tunnel.Metadata
	return items, a.invoke(ctx, "list_managed", map[string]any{"config": cfg, "scope": scope}, &items)
}

func (a pluginAdmin) GetManaged(ctx context.Context, cfg tunnel.Config, id string) (tunnel.Metadata, error) {
	var metadata tunnel.Metadata
	return metadata, a.invoke(ctx, "get_managed", map[string]any{"config": cfg, "id": id}, &metadata)
}

func (a pluginAdmin) CreateManaged(ctx context.Context, cfg tunnel.Config, req tunnel.CreateRequest) (tunnel.Metadata, error) {
	var metadata tunnel.Metadata
	return metadata, a.invoke(ctx, "create_managed", map[string]any{"config": cfg, "create": req}, &metadata)
}

func (a pluginAdmin) UpdateManaged(ctx context.Context, cfg tunnel.Config, id string, req tunnel.UpdateRequest) (tunnel.Metadata, error) {
	var metadata tunnel.Metadata
	return metadata, a.invoke(ctx, "update_managed", map[string]any{"config": cfg, "id": id, "update": req}, &metadata)
}

func (a pluginAdmin) DeleteManaged(ctx context.Context, cfg tunnel.Config, id string) (tunnel.Metadata, error) {
	var metadata tunnel.Metadata
	return metadata, a.invoke(ctx, "delete_managed", map[string]any{"config": cfg, "id": id}, &metadata)
}

func (a pluginAdmin) VerifyAdminKey(ctx context.Context, cfg tunnel.Config) (tunnel.AdminAccess, int, error) {
	var out struct {
		Access tunnel.AdminAccess `json:"access"`
		Count  int                `json:"count"`
	}
	if err := a.invoke(ctx, "verify_admin_key", cfg, &out); err != nil {
		return tunnel.AdminAccess{}, 0, err
	}
	return out.Access, out.Count, nil
}
