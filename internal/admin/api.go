package admin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/tools"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
)

const maxRequestBodyBytes int64 = 1 << 20

type API struct {
	Upstream *upstream.Manager
	Tools    *tools.Runtime
	Tunnel   *tunnel.Client
	Config   *config.Config
}

type authSettings struct {
	MCPEnabled   bool `json:"mcp_enabled"`
	AdminEnabled bool `json:"admin_enabled"`
}

type publicConfig struct {
	Server config.ServerConfig `json:"server"`
	Admin  config.AdminConfig  `json:"admin"`
	Auth   authSettings        `json:"auth"`
}

type configPatch struct {
	Server *config.ServerConfig `json:"server,omitempty"`
	Admin  *config.AdminConfig  `json:"admin,omitempty"`
	Auth   *authSettings        `json:"auth,omitempty"`
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, value any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must contain a single JSON value")
		}
		return err
	}
	return nil
}

func New(api API) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		if api.Config == nil {
			http.Error(w, "config unavailable", http.StatusServiceUnavailable)
			return
		}
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, publicConfig{Server: api.Config.Server, Admin: api.Config.Admin, Auth: authSettings{MCPEnabled: api.Config.Auth.MCPEnabled, AdminEnabled: api.Config.Auth.AdminEnabled}})
		case http.MethodPut:
			var patch configPatch
			if err := decodeJSONBody(w, r, &patch); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			next := *api.Config
			if patch.Server != nil {
				next.Server = *patch.Server
			}
			if patch.Admin != nil {
				next.Admin = *patch.Admin
			}
			if patch.Auth != nil {
				next.Auth.MCPEnabled = patch.Auth.MCPEnabled
				next.Auth.AdminEnabled = patch.Auth.AdminEnabled
			}
			if err := config.Validate(next); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if err := config.Save(next); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			*api.Config = next
			writeJSON(w, publicConfig{Server: next.Server, Admin: next.Admin, Auth: authSettings{MCPEnabled: next.Auth.MCPEnabled, AdminEnabled: next.Auth.AdminEnabled}})
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/workspaces", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		writeJSON(w, []any{})
	})
	mux.HandleFunc("/api/tools", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if api.Tools == nil {
			writeJSON(w, []any{})
			return
		}
		writeJSON(w, api.Tools.List())
	})
	mux.HandleFunc("/api/upstream", func(w http.ResponseWriter, r *http.Request) {
		if api.Upstream == nil {
			http.Error(w, "upstream unavailable", http.StatusServiceUnavailable)
			return
		}
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, api.Upstream.List())
		case http.MethodPost:
			var server upstream.Server
			if err := decodeJSONBody(w, r, &server); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if strings.TrimSpace(server.ID) == "" || strings.TrimSpace(server.Name) == "" || strings.TrimSpace(server.Transport) == "" {
				http.Error(w, "id, name and transport are required", http.StatusBadRequest)
				return
			}
			if err := api.Upstream.Add(server); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeJSON(w, server)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/upstream/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if api.Upstream == nil {
			http.Error(w, "upstream unavailable", http.StatusServiceUnavailable)
			return
		}
		id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/upstream/"))
		if id == "" {
			http.Error(w, "upstream id is required", http.StatusBadRequest)
			return
		}
		if err := api.Upstream.Remove(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
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
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	return mux
}

func Handler() http.Handler { return New(API{}) }
