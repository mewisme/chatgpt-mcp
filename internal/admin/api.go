package admin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/tools"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

const maxRequestBodyBytes int64 = 1 << 20

type API struct {
	Upstream   *upstream.Manager
	Tools      *tools.Runtime
	Workspaces *workspace.Manager
	Tunnel     *tunnel.Client
	Config     *config.Config
}

type authSettings struct {
	MCPEnabled           bool `json:"mcp_enabled"`
	AdminEnabled         bool `json:"admin_enabled"`
	MCPTokenConfigured   bool `json:"mcp_token_configured"`
	AdminTokenConfigured bool `json:"admin_token_configured"`
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

func New(api API) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", method(http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("/api/config", api.handleConfig)
	mux.HandleFunc("/api/workspaces", api.handleWorkspaces)
	mux.HandleFunc("/api/workspaces/", api.handleWorkspace)
	mux.HandleFunc("/api/tools", api.handleTools)
	mux.HandleFunc("/api/upstream", api.handleUpstreams)
	mux.HandleFunc("/api/upstream/", api.handleUpstream)
	mux.HandleFunc("/api/tunnel/config", api.handleTunnelConfig)
	mux.HandleFunc("/api/tunnel", api.handleTunnel)
	return mux
}

func (api API) handleConfig(w http.ResponseWriter, r *http.Request) {
	if api.Config == nil {
		http.Error(w, "config unavailable", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, publicConfigView(*api.Config))
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
		writeJSON(w, publicConfigView(next))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api API) handleTools(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if api.Tools == nil {
		writeJSON(w, []any{})
		return
	}
	writeJSON(w, api.Tools.List())
}

func (api API) workspaceManager() *workspace.Manager {
	if api.Workspaces != nil {
		return api.Workspaces
	}
	if api.Tools != nil {
		return api.Tools.Workspaces
	}
	return nil
}

func (api API) upstreamManager() *upstream.Manager {
	if api.Tools != nil && api.Tools.Upstream != nil {
		return api.Tools.Upstream
	}
	return api.Upstream
}

func publicConfigView(cfg config.Config) publicConfig {
	return publicConfig{
		Server: cfg.Server,
		Admin:  cfg.Admin,
		Auth: authSettings{
			MCPEnabled: cfg.Auth.MCPEnabled, AdminEnabled: cfg.Auth.AdminEnabled,
			MCPTokenConfigured: cfg.Auth.MCPTokenHash != "", AdminTokenConfigured: cfg.Auth.AdminTokenHash != "",
		},
	}
}

func method(method string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		next(w, r)
	}
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

func Handler() http.Handler { return New(API{}) }
