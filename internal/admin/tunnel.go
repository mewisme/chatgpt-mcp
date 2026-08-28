package admin

import (
	"net/http"

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
	value := api.Tunnel.Config()
	value.APIKey = ""
	writeJSON(w, value)
}

func (api API) handleTunnel(w http.ResponseWriter, r *http.Request) {
	if api.Tunnel == nil {
		http.Error(w, "tunnel unavailable", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, api.Tunnel.Status())
	case http.MethodPost:
		if err := api.Tunnel.Start(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, api.Tunnel.Status())
	case http.MethodDelete:
		if err := api.Tunnel.Stop(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, api.Tunnel.Status())
	case http.MethodPut:
		api.configureTunnel(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api API) configureTunnel(w http.ResponseWriter, r *http.Request) {
	if api.Config == nil {
		http.Error(w, "config unavailable", http.StatusServiceUnavailable)
		return
	}
	var next tunnel.Config
	if err := decodeJSONBody(w, r, &next); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	current := api.Tunnel.Config()
	if next.APIKey == "" {
		next.APIKey = current.APIKey
	}
	if next.Origin == "" {
		next.Origin = config.TunnelOrigin(*api.Config)
	}
	candidate := *api.Config
	candidate.Tunnel = next
	if err := config.Validate(candidate); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := api.Tunnel.Stop(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := api.Tunnel.Configure(next); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if next.Enabled {
		if err := api.Tunnel.Start(); err != nil {
			_ = api.Tunnel.Configure(current)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	if err := config.Save(candidate); err != nil {
		_ = api.Tunnel.Stop()
		_ = api.Tunnel.Configure(current)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	*api.Config = candidate
	writeJSON(w, api.Tunnel.Status())
}
