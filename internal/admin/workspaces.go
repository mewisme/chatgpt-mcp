package admin

import (
	"errors"
	"net/http"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

type workspaceRequest struct {
	Path string `json:"path"`
}

func (api API) handleWorkspaces(w http.ResponseWriter, r *http.Request) {
	manager := api.workspaceManager()
	if manager == nil {
		http.Error(w, "workspace registry unavailable", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		values, err := manager.List()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, values)
	case http.MethodPost:
		var request workspaceRequest
		if err := decodeJSONBody(w, r, &request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(request.Path) == "" {
			http.Error(w, "path is required", http.StatusBadRequest)
			return
		}
		value, err := manager.Register(request.Path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, value)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api API) handleWorkspace(w http.ResponseWriter, r *http.Request) {
	manager := api.workspaceManager()
	if manager == nil {
		http.Error(w, "workspace registry unavailable", http.StatusServiceUnavailable)
		return
	}
	id := singlePathID(r.URL.Path, "/api/workspaces/")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	value, err := manager.Get(id)
	if err != nil {
		if errors.Is(err, workspace.ErrNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, value)
	case http.MethodDelete:
		if err := manager.Unregister(id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func singlePathID(path, prefix string) string {
	value := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	if value == "" || strings.Contains(value, "/") {
		return ""
	}
	return value
}
