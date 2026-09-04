package admin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"go.mewis.me/chatgpt-mcp/internal/approval"
	"go.mewis.me/chatgpt-mcp/internal/config"
	mcpnetwork "go.mewis.me/chatgpt-mcp/internal/network"
	mcpoauth "go.mewis.me/chatgpt-mcp/internal/oauth"
	"go.mewis.me/chatgpt-mcp/internal/tools"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

const maxRequestBodyBytes int64 = 1 << 20

type API struct {
	Approvals    *approval.Manager
	Upstream     *upstream.Manager
	Tools        *tools.Runtime
	Workspaces   *workspace.Manager
	Tunnel       *tunnel.Client
	Config       *config.RuntimeStore
	OAuth        *mcpoauth.Store
	OAuthFlows   *mcpoauth.FlowManager
	ReloadConfig func(config.Config) error
	saveConfig   func(config.Config) error
}

type authSettings struct {
	MCPEnabled           bool `json:"mcp_enabled"`
	AdminEnabled         bool `json:"admin_enabled"`
	MCPTokenConfigured   bool `json:"mcp_token_configured"`
	AdminTokenConfigured bool `json:"admin_token_configured"`
}

type publicConfig struct {
	Server      config.ServerConfig      `json:"server"`
	Admin       config.AdminConfig       `json:"admin"`
	Auth        authSettings             `json:"auth"`
	Permissions config.PermissionsConfig `json:"permissions"`
	Shell       config.ShellConfig       `json:"shell"`
	Features    config.FeaturesConfig    `json:"features"`
}

type configPatch struct {
	Server      *config.ServerConfig      `json:"server,omitempty"`
	Admin       *config.AdminConfig       `json:"admin,omitempty"`
	Auth        *authSettings             `json:"auth,omitempty"`
	Permissions *config.PermissionsConfig `json:"permissions,omitempty"`
	Shell       *config.ShellConfig       `json:"shell,omitempty"`
	Features    *featurePatch             `json:"features,omitempty"`
}

type featurePatch struct {
	Ponytail *featureEnabledPatch `json:"ponytail,omitempty"`
	Caveman  *featureEnabledPatch `json:"caveman,omitempty"`
}

type featureEnabledPatch struct {
	Enabled *bool `json:"enabled,omitempty"`
}

func New(api API) http.Handler {
	api = api.withOAuth()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", method(http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		authEnabled := api.Config != nil && api.Config.Snapshot().Auth.AdminEnabled
		writeJSON(w, map[string]bool{"ok": true, "auth_enabled": authEnabled})
	}))
	mux.HandleFunc("/api/network/interfaces", api.handleNetworkInterfaces)

	mux.HandleFunc("/api/config", api.handleConfig)
	mux.HandleFunc("/api/config/presets", api.handleConfigPresets)
	mux.HandleFunc("/api/config/presets/", api.handleConfigPreset)
	mux.HandleFunc("/api/instructions/global", api.handleGlobalInstructions)
	mux.HandleFunc("/api/workspaces", api.handleWorkspaces)
	mux.HandleFunc("/api/workspaces/", api.handleWorkspace)
	mux.HandleFunc("/api/tools", api.handleTools)
	mux.HandleFunc("/api/requests", api.handleRequests)
	mux.HandleFunc("/api/requests/", api.handleRequest)
	mux.HandleFunc("/api/upstream", api.handleUpstreams)
	mux.HandleFunc("/api/upstream/", api.handleUpstream)
	mux.HandleFunc("/api/tunnel/config", api.handleTunnelConfig)
	mux.HandleFunc("/api/tunnel/admin/key", api.handleTunnelAdminKey)
	mux.HandleFunc("/api/tunnel", api.handleTunnel)
	return mux
}

func (api API) handleNetworkInterfaces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	interfaces, err := mcpnetwork.Discover()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, interfaces)
}

func (api API) handleConfig(w http.ResponseWriter, r *http.Request) {
	if api.Config == nil {
		http.Error(w, "config unavailable", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, publicConfigView(api.Config.Snapshot()))
	case http.MethodPut:
		var patch configPatch
		if err := decodeJSONBody(w, r, &patch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		status := http.StatusInternalServerError
		previous := api.Config.Snapshot()
		next := previous
		var err error
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

		if patch.Permissions != nil {
			var allowDirs []string
			allowDirs, err = config.NormalizeAllowDirs(patch.Permissions.AllowDirs)
			if err == nil {
				next.Permissions.AllowDirs = allowDirs
			}
		}
		if err == nil && patch.Shell != nil {
			var shellPath []string
			shellPath, err = config.NormalizeShellPath(patch.Shell.Path)
			if err == nil {
				next.Shell.Path = shellPath
			}
		}
		if err == nil && patch.Features != nil {
			if patch.Features.Ponytail != nil && patch.Features.Ponytail.Enabled != nil {
				next.Features.Ponytail.Enabled = *patch.Features.Ponytail.Enabled
			}
			if patch.Features.Caveman != nil && patch.Features.Caveman.Enabled != nil {
				next.Features.Caveman.Enabled = *patch.Features.Caveman.Enabled
			}
		}
		if err == nil {
			err = config.Validate(next)
		}
		if err != nil {
			status = http.StatusBadRequest
		}
		if err == nil {
			err = api.commitConfig(next, previous)
		}
		if err != nil {
			http.Error(w, err.Error(), status)
			return
		}
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
		Server: cfg.Server, Admin: cfg.Admin, Permissions: cfg.Permissions, Shell: cfg.Shell, Features: cfg.Features,
		Auth: authSettings{
			MCPEnabled: cfg.Auth.MCPEnabled, AdminEnabled: cfg.Auth.AdminEnabled,
			MCPTokenConfigured: cfg.Auth.MCPTokenHash != "", AdminTokenConfigured: cfg.Auth.AdminTokenHash != "",
		},
	}
}

func (api API) commitConfig(next, previous config.Config) error {
	if api.ReloadConfig == nil {
		_, err := api.Config.Update(func(config.Config) (config.Config, error) { return next, api.persistConfigWithFeatures(next, previous) })
		return err
	}
	if err := api.persistConfig(next); err != nil {
		return err
	}
	if err := api.ReloadConfig(next); err != nil {
		return errors.Join(err, api.persistConfig(previous))
	}
	return nil
}

func (api API) persistConfigWithFeatures(next, previous config.Config) error {
	if err := api.persistConfig(next); err != nil {
		return err
	}
	if next.Features != previous.Features && api.Tools != nil {
		if err := api.Tools.SyncFeatures(next.Features); err != nil {
			return errors.Join(err, api.persistConfig(previous))
		}
	}
	if api.Tools != nil {
		api.Tools.SetGlobalAllowDirs(next.Permissions.AllowDirs)
	}
	return nil
}

func (api API) persistConfig(value config.Config) error {
	if api.saveConfig != nil {
		return api.saveConfig(value)
	}
	return config.Save(value)
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
