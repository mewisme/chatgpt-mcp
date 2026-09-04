package admin

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/instructioncontext"
	"go.mewis.me/chatgpt-mcp/internal/projectcontext"
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
	parts := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/workspaces/"), "/"), "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		http.NotFound(w, r)
		return
	}
	id := parts[0]
	value, err := manager.Get(id)
	if err != nil {
		if errors.Is(err, workspace.ErrNotFound) {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	if len(parts) == 2 && parts[1] == "context" {
		api.handleWorkspaceContext(w, r, manager, value)
		return
	}
	if len(parts) >= 2 && parts[1] == "executions" {
		api.handleWorkspaceExecutions(w, r, value, parts[2:])
		return
	}
	if len(parts) != 1 {
		http.NotFound(w, r)
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

func (api API) handleWorkspaceContext(w http.ResponseWriter, r *http.Request, manager *workspace.Manager, item workspace.Workspace) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	toolProfile := func() instructioncontext.ToolProfile {
		count := 0
		if api.Tools != nil {
			count = len(api.Tools.List())
		}
		return instructioncontext.ToolProfile{Name: "full", Count: count}
	}
	service := projectcontext.New(manager, toolProfile)
	adminEnabled, adminPort := false, 0
	if api.Config != nil {
		cfg := api.Config.Snapshot()
		adminEnabled, adminPort = cfg.Admin.Enabled, cfg.Admin.Port
	}
	result, err := service.Build(r.Context(), item.ID, projectcontext.Options{
		Path:                strings.TrimSpace(r.URL.Query().Get("path")),
		MaxInstructionBytes: queryInt(r, "max_instruction_bytes", instructioncontext.DefaultInstructionMaxBytes, 1, 1_000_000),
		MaxSectionBytes:     queryInt(r, "max_section_bytes", instructioncontext.DefaultSectionMaxBytes, 1, 500_000),
		MaxLinesPerSection:  queryInt(r, "max_lines_per_section", instructioncontext.DefaultSectionMaxLines, 1, 5_000),
		IncludeGit:          queryBool(r, "include_git", true),
		IncludeMemory:       queryBool(r, "include_memory", true),
		IncludeSkills:       queryBool(r, "include_skills", true),
		AdminEnabled:        adminEnabled,
		AdminPort:           adminPort,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, result)
}

func queryInt(r *http.Request, key string, fallback, min, max int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < min || value > max {
		return fallback
	}
	return value
}
