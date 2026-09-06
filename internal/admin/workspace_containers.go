package admin

import (
	"errors"
	"net/http"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

type workspaceContainerRequest struct {
	Name string `json:"name"`
}

type workspaceContainerMembersRequest struct {
	WorkspaceIDs []string `json:"workspace_ids"`
}

type workspaceContainersMembershipRequest struct {
	ContainerIDs []string `json:"container_ids"`
}

func (api API) handleWorkspaceContainers(w http.ResponseWriter, r *http.Request) {
	manager := api.workspaceManager()
	if manager == nil {
		http.Error(w, "workspace registry unavailable", http.StatusServiceUnavailable)
		return
	}
	switch r.Method {
	case http.MethodGet:
		values, err := manager.ListContainers()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, values)
	case http.MethodPost:
		var request workspaceContainerRequest
		if err := decodeJSONBody(w, r, &request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		value, err := manager.CreateContainer(request.Name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, value)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api API) handleWorkspaceContainer(w http.ResponseWriter, r *http.Request) {
	manager := api.workspaceManager()
	if manager == nil {
		http.Error(w, "workspace registry unavailable", http.StatusServiceUnavailable)
		return
	}
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/workspace-containers/"), "/"), "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	value, err := manager.GetContainer(id)
	if err != nil {
		writeWorkspaceContainerError(w, err)
		return
	}
	if len(parts) == 2 && parts[1] == "workspaces" {
		api.handleWorkspaceContainerMembers(w, r, manager, value)
		return
	}
	if len(parts) != 1 {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, value)
	case http.MethodPatch:
		var request workspaceContainerRequest
		if err := decodeJSONBody(w, r, &request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		updated, err := manager.RenameContainer(id, request.Name)
		if err != nil {
			writeWorkspaceContainerError(w, err)
			return
		}
		writeJSON(w, updated)
	case http.MethodDelete:
		if err := manager.DeleteContainer(id); err != nil {
			writeWorkspaceContainerError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api API) handleWorkspaceContainerMembers(w http.ResponseWriter, r *http.Request, manager *workspace.Manager, container workspace.WorkspaceContainer) {
	switch r.Method {
	case http.MethodGet:
		values, err := manager.WorkspacesForContainer(container.ID)
		if err != nil {
			writeWorkspaceContainerError(w, err)
			return
		}
		writeJSON(w, values)
	case http.MethodPost, http.MethodDelete:
		var request workspaceContainerMembersRequest
		if err := decodeJSONBody(w, r, &request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var value workspace.WorkspaceContainer
		var err error
		if r.Method == http.MethodPost {
			value, err = manager.AddWorkspacesToContainer(container.ID, request.WorkspaceIDs)
		} else {
			value, err = manager.RemoveWorkspacesFromContainer(container.ID, request.WorkspaceIDs)
		}
		if err != nil {
			writeWorkspaceContainerError(w, err)
			return
		}
		writeJSON(w, value)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api API) handleWorkspaceContainersMembership(w http.ResponseWriter, r *http.Request, manager *workspace.Manager, item workspace.Workspace) {
	switch r.Method {
	case http.MethodGet:
		values, err := manager.ContainersForWorkspace(item.ID)
		if err != nil {
			writeWorkspaceContainerError(w, err)
			return
		}
		writeJSON(w, values)
	case http.MethodPost, http.MethodDelete:
		var request workspaceContainersMembershipRequest
		if err := decodeJSONBody(w, r, &request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var values []workspace.WorkspaceContainer
		var err error
		if r.Method == http.MethodPost {
			values, err = manager.AddWorkspaceToContainers(item.ID, request.ContainerIDs)
		} else {
			values, err = manager.RemoveWorkspaceFromContainers(item.ID, request.ContainerIDs)
		}
		if err != nil {
			writeWorkspaceContainerError(w, err)
			return
		}
		writeJSON(w, values)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func writeWorkspaceContainerError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, workspace.ErrContainerNotFound) || errors.Is(err, workspace.ErrNotFound) {
		status = http.StatusNotFound
	}
	http.Error(w, err.Error(), status)
}
