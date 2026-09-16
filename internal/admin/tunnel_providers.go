package admin

import (
	"errors"
	"net/http"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/runtimecontrol"
	"go.mewis.me/chatgpt-mcp/internal/tunnelprovider"
)

type tunnelProviderActionRequest struct {
	Target string `json:"target"`
}

func (api API) handleTunnelProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	cfg := api.snapshotConfig()
	writeJSON(w, application.ListConfiguredTunnelProviders(cfg))
}

func (api API) handleTunnelProvider(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/tunnel-providers/"), "/")
	if path == "" {
		api.handleTunnelProviders(w, r)
		return
	}
	parts := strings.Split(path, "/")
	provider := strings.TrimSpace(parts[0])
	if provider == "" {
		http.NotFound(w, r)
		return
	}
	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		item, err := application.ConfiguredTunnelProviderStatus(api.snapshotConfig(), provider)
		if err != nil {
			writeTunnelProviderError(w, err)
			return
		}
		writeJSON(w, item)
		return
	}
	if len(parts) != 2 || r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	target := "all"
	if r.Body != nil && r.ContentLength != 0 {
		var req tunnelProviderActionRequest
		if err := decodeJSONBody(w, r, &req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Target) != "" {
			target = req.Target
		}
	}
	cfg := api.snapshotConfig()
	var err error
	switch parts[1] {
	case "start":
		err = application.StartTunnelProvider(r.Context(), cfg, provider, target)
	case "stop":
		err = application.StopTunnelProvider(r.Context(), provider, target)
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		writeTunnelProviderError(w, err)
		return
	}
	item, statusErr := application.ConfiguredTunnelProviderStatus(cfg, provider)
	if statusErr != nil {
		writeJSON(w, runtimecontrol.TunnelProviderStatus{Provider: provider})
		return
	}
	writeJSON(w, item)
}

func (api API) snapshotConfig() config.Config {
	if api.Config != nil {
		return api.Config.Snapshot()
	}
	loaded, err := config.Load()
	if err != nil {
		return config.Default()
	}
	return loaded
}

func writeTunnelProviderError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, tunnelprovider.ErrNotInstalled):
		status = http.StatusNotFound
	case errors.Is(err, tunnelprovider.ErrMCPHTTPDisabled), errors.Is(err, tunnelprovider.ErrMCPAuthDisabled), errors.Is(err, tunnelprovider.ErrMCPTokenMissing),
		errors.Is(err, tunnelprovider.ErrAdminHTTPDisabled), errors.Is(err, tunnelprovider.ErrAdminAuthDisabled), errors.Is(err, tunnelprovider.ErrAdminTokenMissing),
		errors.Is(err, tunnelprovider.ErrListenerNotReady):
		status = http.StatusConflict
	}
	http.Error(w, err.Error(), status)
}
