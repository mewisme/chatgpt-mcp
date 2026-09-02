package admin

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

func (api API) handleTunnelConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if api.Tunnel == nil {
		http.Error(w, "tunnel unavailable", http.StatusServiceUnavailable)
		return
	}
	if api.Config == nil {
		http.Error(w, "config unavailable", http.StatusServiceUnavailable)
		return
	}
	value := api.Config.Snapshot().Tunnel
	view := tunnelConfigView{Config: value, RuntimeKeyConfigured: strings.TrimSpace(value.APIKey) != "", AdminKeyConfigured: tunnel.AdminConfigured(value)}
	view.Config.APIKey = ""
	view.Config.AdminKey = ""
	writeJSON(w, view)
}

func (api API) handleTunnel(w http.ResponseWriter, r *http.Request) {
	if api.Tunnel == nil {
		http.Error(w, "tunnel unavailable", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, api.tunnelStatus(r.Context()))
	case http.MethodPost:
		if err := api.Tunnel.Start(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, api.tunnelStatus(r.Context()))
	case http.MethodDelete:
		if err := api.Tunnel.Stop(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, api.tunnelStatus(r.Context()))
	case http.MethodPut:
		api.configureTunnel(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api API) tunnelStatus(_ context.Context) tunnel.Status {
	if api.Tunnel == nil {
		return tunnel.Status{Provider: tunnel.ProviderOpenAI}
	}
	return api.Tunnel.Status()
}

func (api API) configureTunnel(w http.ResponseWriter, r *http.Request) {
	if api.Config == nil || api.Tunnel == nil {
		http.Error(w, "config unavailable", http.StatusServiceUnavailable)
		return
	}
	var next tunnel.Config
	if err := decodeJSONBody(w, r, &next); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	current := api.Config.Snapshot()
	effective := next
	if effective.APIKey == "" {
		effective.APIKey = current.Tunnel.APIKey
	}
	effective.AdminKey = current.Tunnel.AdminKey
	effective.AdminOrganizationID = current.Tunnel.AdminOrganizationID
	effective.AdminWorkspaceID = current.Tunnel.AdminWorkspaceID
	effective.AdminTenantID = current.Tunnel.AdminTenantID
	candidate := current
	candidate.Tunnel = effective
	if err := config.Validate(candidate); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if tunnel.Configured(effective) && !tunnel.RuntimeConfigEqual(current.Tunnel, effective) {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		_, _, err := config.SyncTunnelMetadata(ctx, effective)
		cancel()
		if err != nil {
			http.Error(w, "persist tunnel metadata: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	_, err := api.Config.Update(func(config.Config) (config.Config, error) {
		if err := api.Tunnel.Reconfigure(effective, func() error { return api.persistConfig(candidate) }); err != nil {
			return current, err
		}
		return candidate, nil
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if metadata, err := config.LoadTunnelMetadata(effective.ID); err == nil {
		_ = api.Tunnel.SeedMetadata(metadata)
	}
	writeJSON(w, api.tunnelStatus(r.Context()))
}

type tunnelConfigView struct {
	tunnel.Config
	RuntimeKeyConfigured bool `json:"runtime_key_configured"`
	AdminKeyConfigured   bool `json:"admin_key_configured"`
}

type tunnelAdminKeyRequest struct {
	AdminKey       string `json:"admin_key,omitempty"`
	OrganizationID string `json:"organization_id,omitempty"`
	WorkspaceID    string `json:"workspace_id,omitempty"`
	TenantID       string `json:"tenant_id,omitempty"`
}

type tunnelAdminKeyStatus struct {
	Configured bool              `json:"configured"`
	Scope      tunnel.AdminScope `json:"scope"`
	Tunnels    int               `json:"tunnels,omitempty"`
}

func (api API) handleTunnelAdminKey(w http.ResponseWriter, r *http.Request) {
	if api.Config == nil || api.Tunnel == nil {
		http.Error(w, "tunnel configuration unavailable", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, tunnelAdminStatus(api.Config.Snapshot().Tunnel, 0))
	case http.MethodPut:
		var request tunnelAdminKeyRequest
		if err := decodeJSONBody(w, r, &request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		count, err := api.saveTunnelAdminKey(r.Context(), request)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, tunnelAdminStatus(api.Config.Snapshot().Tunnel, count))
	case http.MethodPost:
		cfg := api.Config.Snapshot().Tunnel
		if !tunnel.AdminConfigured(cfg) {
			http.Error(w, "tunnel admin key is not configured", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		count, err := tunnel.VerifyAdminKey(ctx, cfg)
		cancel()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, tunnelAdminStatus(cfg, count))
	case http.MethodDelete:
		if err := api.removeTunnelAdminKey(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, tunnelAdminStatus(api.Config.Snapshot().Tunnel, 0))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api API) saveTunnelAdminKey(parent context.Context, request tunnelAdminKeyRequest) (int, error) {
	count := 0
	_, err := api.Config.Update(func(candidate config.Config) (config.Config, error) {
		previous := candidate
		key := strings.TrimSpace(request.AdminKey)
		if key == "" {
			key = strings.TrimSpace(candidate.Tunnel.AdminKey)
		}
		candidate.Tunnel.AdminKey = key
		tunnel.ApplyAdminScope(&candidate.Tunnel, tunnel.AdminScope{OrganizationID: request.OrganizationID, WorkspaceID: request.WorkspaceID, TenantID: request.TenantID})
		if !tunnel.AdminConfigured(candidate.Tunnel) {
			return previous, errors.New("admin key and exactly one organization, workspace, or tenant scope are required")
		}
		ctx, cancel := context.WithTimeout(parent, 30*time.Second)
		var verifyErr error
		count, verifyErr = tunnel.VerifyAdminKey(ctx, candidate.Tunnel)
		cancel()
		if verifyErr != nil {
			return previous, verifyErr
		}
		if err := api.persistConfig(candidate); err != nil {
			return previous, err
		}
		if err := api.Tunnel.SyncManagementConfig(candidate.Tunnel); err != nil {
			return previous, errors.Join(err, api.persistConfig(previous))
		}
		return candidate, nil
	})
	return count, err
}

func (api API) removeTunnelAdminKey() error {
	_, err := api.Config.Update(func(candidate config.Config) (config.Config, error) {
		previous := candidate
		candidate.Tunnel.AdminKey = ""
		tunnel.ApplyAdminScope(&candidate.Tunnel, tunnel.AdminScope{})
		if err := api.persistConfig(candidate); err != nil {
			return previous, err
		}
		if err := api.Tunnel.SyncManagementConfig(candidate.Tunnel); err != nil {
			return previous, errors.Join(err, api.persistConfig(previous))
		}
		return candidate, nil
	})
	return err
}

func tunnelAdminStatus(cfg tunnel.Config, count int) tunnelAdminKeyStatus {
	return tunnelAdminKeyStatus{Configured: tunnel.AdminConfigured(cfg), Scope: tunnel.AdminScopeFromConfig(cfg), Tunnels: count}
}
