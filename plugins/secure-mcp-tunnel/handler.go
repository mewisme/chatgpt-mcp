package securemcptunnel

import (
	"context"
	"encoding/json"

	"go.mewis.me/chatgpt-mcp/internal/runtimeplugin"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
	"go.mewis.me/chatgpt-mcp/internal/tunnelprovider"
)

type Handler struct {
	Manager *tunnel.Manager
}

func NewHandler() *Handler {
	tunnel.SetAdminBackend(ControlPlane())
	manager := tunnel.NewManager(nil, nil)
	manager.SetBackendFactory(OpenAIFactory(nil))
	return &Handler{Manager: manager}
}

func (h *Handler) Describe(context.Context, json.RawMessage) (any, error) {
	return runtimeplugin.DescribeResult{
		Provider: tunnelprovider.ProviderSecureMCP,
		Name:     "Secure MCP Tunnel",
		Targets:  []runtimeplugin.Target{{ID: "mcp", OriginKind: tunnelprovider.OriginPrivate}},
	}, nil
}

func (h *Handler) Status(context.Context, json.RawMessage) (any, error) {
	return h.statusResult(), nil
}

func (h *Handler) Start(ctx context.Context, _ json.RawMessage) (any, error) {
	if err := h.manager().StartContext(ctx); err != nil {
		return nil, err
	}
	return h.statusResult(), nil
}

func (h *Handler) Stop(ctx context.Context, _ json.RawMessage) (any, error) {
	if err := h.manager().StopContext(ctx); err != nil {
		return nil, err
	}
	return h.statusResult(), nil
}

func (h *Handler) Shutdown(ctx context.Context, _ json.RawMessage) (any, error) {
	_ = h.manager().StopContext(ctx)
	return map[string]bool{"ok": true}, nil
}

func (h *Handler) Reconcile(ctx context.Context, params json.RawMessage) (any, error) {
	var reconcile runtimeplugin.ReconcileParams
	if err := json.Unmarshal(params, &reconcile); err != nil {
		return nil, err
	}
	collection, err := decodeCollection(reconcile.Collection)
	if err != nil {
		return nil, err
	}
	h.manager().SetBridge(reconcile.BridgeURL, reconcile.BridgeToken)
	if err := h.manager().Reconcile(ctx, collection); err != nil {
		return nil, err
	}
	if err := h.manager().StartContext(ctx); err != nil {
		return nil, err
	}
	return h.statusResult(), nil
}

func (h *Handler) Invoke(ctx context.Context, params json.RawMessage) (any, error) {
	var invoke runtimeplugin.InvokeParams
	if err := json.Unmarshal(params, &invoke); err != nil {
		return nil, err
	}
	switch invoke.Op {
	case "fetch_metadata":
		cfg, err := decodeValue[tunnel.Config](invoke.Payload)
		if err != nil {
			return nil, err
		}
		return FetchMetadata(ctx, cfg)
	case "list_managed":
		in, err := decodeValue[adminListIn](invoke.Payload)
		if err != nil {
			return nil, err
		}
		return ListManaged(ctx, in.Config, in.Scope)
	case "get_managed":
		in, err := decodeValue[adminIDIn](invoke.Payload)
		if err != nil {
			return nil, err
		}
		return GetManaged(ctx, in.Config, in.ID)
	case "create_managed":
		in, err := decodeValue[adminCreateIn](invoke.Payload)
		if err != nil {
			return nil, err
		}
		return CreateManaged(ctx, in.Config, in.Create)
	case "update_managed":
		in, err := decodeValue[adminUpdateIn](invoke.Payload)
		if err != nil {
			return nil, err
		}
		return UpdateManaged(ctx, in.Config, in.ID, in.Update)
	case "delete_managed":
		in, err := decodeValue[adminIDIn](invoke.Payload)
		if err != nil {
			return nil, err
		}
		return DeleteManaged(ctx, in.Config, in.ID)
	case "verify_admin_key":
		cfg, err := decodeValue[tunnel.Config](invoke.Payload)
		if err != nil {
			return nil, err
		}
		access, count, err := VerifyAdminKey(ctx, cfg)
		if err != nil {
			return nil, err
		}
		return adminVerifyOut{Access: access, Count: count}, nil
	case "runtime_status":
		return h.manager().Statuses(), nil
	default:
		return nil, runtimeplugin.ErrUnknownMethod
	}
}

type adminListIn struct {
	Config tunnel.Config     `json:"config"`
	Scope  tunnel.AdminScope `json:"scope"`
}

type adminIDIn struct {
	Config tunnel.Config `json:"config"`
	ID     string        `json:"id"`
}

type adminCreateIn struct {
	Config tunnel.Config        `json:"config"`
	Create tunnel.CreateRequest `json:"create"`
}

type adminUpdateIn struct {
	Config tunnel.Config        `json:"config"`
	ID     string               `json:"id"`
	Update tunnel.UpdateRequest `json:"update"`
}

type adminVerifyOut struct {
	Access tunnel.AdminAccess `json:"access"`
	Count  int                `json:"count"`
}

func (h *Handler) manager() *tunnel.Manager {
	if h != nil && h.Manager != nil {
		return h.Manager
	}
	return NewHandler().Manager
}

func (h *Handler) statusResult() runtimeplugin.StatusResult {
	statuses := h.manager().Statuses()
	targets := make([]runtimeplugin.TargetStatus, 0, len(statuses))
	state := "stopped"
	for _, item := range statuses {
		targets = append(targets, runtimeplugin.TargetStatus{
			Target: item.ID, Desired: item.Enabled, Running: item.Running, Ready: item.Ready, Restarting: item.Restarting, LastError: item.LastError,
		})
		if item.Enabled && item.Running {
			state = "running"
		}
	}
	return runtimeplugin.StatusResult{State: state, Targets: targets}
}

func decodeCollection(value any) (tunnel.CollectionConfig, error) {
	in, err := decodeValue[reconcileCollection](value)
	if err != nil {
		return tunnel.CollectionConfig{}, err
	}
	out := tunnel.CollectionConfig{Instances: make([]tunnel.InstanceConfig, 0, len(in.Instances))}
	for _, item := range in.Instances {
		out.Instances = append(out.Instances, tunnel.InstanceConfig{
			Enabled: item.Enabled, ID: item.ID, APIKey: item.APIKey, ControlPlaneBaseURL: item.ControlPlaneBaseURL, OrganizationID: item.OrganizationID,
		})
	}
	return out, nil
}

type reconcileCollection struct {
	Instances []reconcileInstance `json:"instances"`
}

type reconcileInstance struct {
	Enabled             bool   `json:"enabled"`
	ID                  string `json:"id"`
	APIKey              string `json:"api_key"`
	ControlPlaneBaseURL string `json:"control_plane_base_url"`
	OrganizationID      string `json:"organization_id"`
}

func decodeValue[T any](value any) (T, error) {
	var out T
	if value == nil {
		return out, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, err
	}
	return out, nil
}
