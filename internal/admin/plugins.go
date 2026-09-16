package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/application"
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

type pluginListItem struct {
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Origin      pluginpkg.Origin      `json:"origin"`
	OriginLabel string                `json:"origin_label"`
	Enabled     bool                  `json:"enabled"`
	Lifecycle   pluginpkg.Lifecycle   `json:"lifecycle"`
	Scope       pluginpkg.PluginScope `json:"scope"`
	WorkspaceID string                `json:"workspace_id,omitempty"`
}

type pluginConfigView struct {
	ID          pluginpkg.PluginID       `json:"id"`
	Name        string                   `json:"name"`
	Origin      pluginpkg.Origin         `json:"origin"`
	Scope       string                   `json:"scope"`
	WorkspaceID string                   `json:"workspace_id,omitempty"`
	Schema      pluginpkg.SettingsSchema `json:"schema"`
	Values      map[string]any           `json:"values"`
}

type pluginConfigPatch struct {
	Values map[string]any `json:"values"`
}

type pluginConfigResetRequest struct {
	Key string `json:"key"`
}

func (api API) handlePlugins(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	service, err := api.pluginServiceFor(r)
	if err != nil {
		http.Error(w, err.Error(), pluginScopeStatus(err))
		return
	}
	items, err := service.Installed()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	out := make([]pluginListItem, 0, len(items))
	for _, item := range items {
		out = append(out, pluginListItem{
			ID: string(item.ID), Name: item.Installed.Manifest.Name, Origin: item.Origin,
			OriginLabel: item.Origin.Label(), Enabled: item.Lock.Enabled, Lifecycle: item.Lifecycle,
			Scope: item.Scope, WorkspaceID: item.Workspace,
		})
	}
	writeJSON(w, out)
}

func (api API) handlePlugin(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/plugins/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "config" {
		http.NotFound(w, r)
		return
	}
	id := pluginpkg.PluginID(parts[0])
	if len(parts) == 3 && parts[2] == "reset" {
		api.handlePluginConfigReset(w, r, id)
		return
	}
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet:
		api.handlePluginConfigGet(w, r, id)
	case http.MethodPut:
		api.handlePluginConfigPut(w, r, id)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api API) handlePluginConfigGet(w http.ResponseWriter, r *http.Request, id pluginpkg.PluginID) {
	view, status, err := api.pluginConfigView(r, id)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, view)
}

func (api API) handlePluginConfigPut(w http.ResponseWriter, r *http.Request, id pluginpkg.PluginID) {
	var patch pluginConfigPatch
	if err := decodeJSONBody(w, r, &patch); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	service, err := api.pluginServiceFor(r)
	if err != nil {
		http.Error(w, err.Error(), pluginScopeStatus(err))
		return
	}
	schema, err := service.Manager.SettingsSchema(id)
	if err != nil {
		http.Error(w, err.Error(), pluginConfigStatus(err))
		return
	}
	for key, raw := range patch.Values {
		field, ok := schema.Field(key)
		if !ok {
			http.Error(w, fmt.Sprintf("%s: %s", pluginpkg.ErrUnknownConfigKey, key), http.StatusBadRequest)
			return
		}
		if field.Sensitive && strings.TrimSpace(jsonSettingRaw(raw)) == "" {
			continue
		}
		if err := service.SetPluginSetting(r.Context(), id, key, jsonSettingRaw(raw)); err != nil {
			http.Error(w, err.Error(), pluginConfigStatus(err))
			return
		}
	}
	view, status, err := api.pluginConfigView(r, id)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, view)
}

func (api API) handlePluginConfigReset(w http.ResponseWriter, r *http.Request, id pluginpkg.PluginID) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var request pluginConfigResetRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := decodeJSONBody(w, r, &request); err != nil && !errors.Is(err, io.EOF) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	service, err := api.pluginServiceFor(r)
	if err != nil {
		http.Error(w, err.Error(), pluginScopeStatus(err))
		return
	}
	if err := service.ResetPluginSetting(r.Context(), id, request.Key); err != nil {
		http.Error(w, err.Error(), pluginConfigStatus(err))
		return
	}
	view, status, err := api.pluginConfigView(r, id)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, view)
}

func (api API) pluginConfigView(r *http.Request, id pluginpkg.PluginID) (pluginConfigView, int, error) {
	service, err := api.pluginServiceFor(r)
	if err != nil {
		return pluginConfigView{}, pluginScopeStatus(err), err
	}
	schema, values, err := service.PluginSettings(id)
	if err != nil {
		return pluginConfigView{}, pluginConfigStatus(err), err
	}
	name, origin := string(id), pluginpkg.OriginInstalled
	if detail, err := service.InstalledDetail(id); err == nil {
		name, origin = detail.Manifest.Name, detail.Origin
	}
	return pluginConfigView{ID: id, Name: name, Origin: origin, Scope: string(service.Layout.EffectiveScope()), WorkspaceID: service.Workspace, Schema: schema, Values: values}, 0, nil
}

func (api API) pluginService() (*application.PluginService, error) {
	if api.Plugins != nil {
		return api.Plugins, nil
	}
	return application.NewPluginService()
}

func (api API) pluginServiceFor(r *http.Request) (*application.PluginService, error) {
	opts, err := api.pluginScopeOptions(r)
	if err != nil {
		return nil, err
	}
	if opts.Scope == "" && opts.Workspace == "" {
		return api.pluginService()
	}
	return application.NewPluginServiceForOptions(opts)
}

func (api API) pluginScopeOptions(r *http.Request) (application.PluginScopeOptions, error) {
	if r == nil {
		return application.PluginScopeOptions{}, nil
	}
	scope := strings.TrimSpace(r.URL.Query().Get("scope"))
	workspaceID := strings.TrimSpace(r.URL.Query().Get("workspace_id"))
	if strings.ContainsAny(workspaceID, `/\`) {
		return application.PluginScopeOptions{}, fmt.Errorf("workspace_id must be a workspace id")
	}
	if scope == string(pluginpkg.ScopeWorkspace) && workspaceID == "" {
		return application.PluginScopeOptions{}, fmt.Errorf("workspace scope requires workspace_id")
	}
	if workspaceID != "" && scope == "" {
		scope = string(pluginpkg.ScopeWorkspace)
	}
	if workspaceID != "" {
		manager := api.workspaceManager()
		if manager == nil {
			return application.PluginScopeOptions{}, fmt.Errorf("workspace registry unavailable")
		}
		if _, err := manager.Get(workspaceID); err != nil {
			return application.PluginScopeOptions{}, err
		}
	}
	return application.PluginScopeOptions{Scope: scope, Workspace: workspaceID}, nil
}

func pluginScopeStatus(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case errors.Is(err, workspace.ErrNotFound), errors.Is(err, workspace.ErrUnavailable):
		return http.StatusNotFound
	case strings.Contains(err.Error(), "unavailable"):
		return http.StatusServiceUnavailable
	default:
		return http.StatusBadRequest
	}
}

func pluginConfigStatus(err error) int {
	switch {
	case errors.Is(err, pluginpkg.ErrUnknownConfigKey), errors.Is(err, pluginpkg.ErrInvalidConfigValue):
		return http.StatusBadRequest
	case errors.Is(err, pluginpkg.ErrNoPluginConfig):
		return http.StatusNotFound
	default:
		if err != nil && (strings.Contains(err.Error(), "not installed") || strings.Contains(err.Error(), "is not installed")) {
			return http.StatusNotFound
		}
		return http.StatusInternalServerError
	}
}

func jsonSettingRaw(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case json.Number:
		return typed.String()
	default:
		return fmt.Sprint(typed)
	}
}
