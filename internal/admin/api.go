package admin

import (
	"encoding/json"
	"net/http"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/tools"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
)

type API struct {
	Upstream *upstream.Manager
	Tools    *tools.Registry
	Tunnel   *tunnel.Client
	Config   *config.Config
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func New(api API) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, map[string]bool{"ok": true}) })
	mux.HandleFunc("/api/tools", func(w http.ResponseWriter, r *http.Request) {
		if api.Tools == nil {
			writeJSON(w, []any{})
			return
		}
		writeJSON(w, api.Tools.ListSchemas())
	})
	mux.HandleFunc("/api/upstream", func(w http.ResponseWriter, r *http.Request) {
		if api.Upstream == nil {
			writeJSON(w, []any{})
			return
		}
		writeJSON(w, api.Upstream.List())
	})
	mux.HandleFunc("/api/tunnel/config", func(w http.ResponseWriter, r *http.Request) {
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
	})
	mux.HandleFunc("/api/tunnel", func(w http.ResponseWriter, r *http.Request) {
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
			if api.Config == nil {
				http.Error(w, "config unavailable", http.StatusServiceUnavailable)
				return
			}
			var next tunnel.Config
			if err := json.NewDecoder(r.Body).Decode(&next); err != nil {
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
			if err := api.Tunnel.Stop(); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if err := api.Tunnel.Configure(next); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			api.Config.Tunnel = next
			if err := config.Save(*api.Config); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			if next.Enabled {
				if err := api.Tunnel.Start(); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
			}
			writeJSON(w, api.Tunnel.Status())
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	return mux
}

func Handler() http.Handler { return New(API{}) }
