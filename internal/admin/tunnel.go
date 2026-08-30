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
	if api.Config == nil {
		http.Error(w, "config unavailable", http.StatusServiceUnavailable)
		return
	}
	value := api.Config.Snapshot().Tunnel
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
	status := http.StatusInternalServerError
	_, err := api.Config.Update(func(candidate config.Config) (config.Config, error) {
		effective := next
		if effective.APIKey == "" {
			effective.APIKey = candidate.Tunnel.APIKey
		}
		candidate.Tunnel = effective
		if err := config.Validate(candidate); err != nil {
			status = http.StatusBadRequest
			return candidate, err
		}
		if err := api.Tunnel.Reconfigure(effective, func() error { return config.Save(candidate) }); err != nil {
			return candidate, err
		}
		return candidate, nil
	})
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, api.Tunnel.Status())
}
