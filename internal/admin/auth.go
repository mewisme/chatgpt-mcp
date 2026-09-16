package admin

import (
	"errors"
	"net/http"

	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/auth"
	"go.mewis.me/chatgpt-mcp/internal/config"
)

type mcpTokenView struct {
	Token      string `json:"token,omitempty"`
	Configured bool   `json:"configured"`
	Revealable bool   `json:"revealable"`
	Enabled    bool   `json:"enabled"`
}

func (api API) handleMCPToken(w http.ResponseWriter, r *http.Request) {
	if api.Config == nil {
		http.Error(w, "config unavailable", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		token, err := application.RevealMCPToken()
		if err != nil {
			http.Error(w, err.Error(), mcpTokenStatus(err))
			return
		}
		status, err := application.GetAuthStatus()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, mcpTokenView{Token: token, Configured: status.MCPConfigured, Revealable: true, Enabled: status.MCPEnabled})
	case http.MethodPost:
		token, status, err := application.RotateAuthToken(r.Context(), "mcp")
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		api.syncConfigStore()
		writeJSON(w, mcpTokenView{Token: token, Configured: status.MCPConfigured, Revealable: status.MCPRevealable, Enabled: status.MCPEnabled})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api API) syncConfigStore() {
	if api.Config == nil {
		return
	}
	loaded, err := config.Load()
	if err != nil {
		return
	}
	_, _ = api.Config.Update(func(config.Config) (config.Config, error) { return loaded, nil })
}

func mcpTokenStatus(err error) int {
	if errors.Is(err, application.ErrMCPTokenMissing) {
		return http.StatusNotFound
	}
	if errors.Is(err, application.ErrMCPTokenNotRevealable) {
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

func mcpTokenRevealable(hash string) bool {
	if hash == "" {
		return false
	}
	token, err := config.GetMCPToken()
	return err == nil && token != "" && auth.VerifyToken(token, hash)
}
