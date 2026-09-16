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
)

type pluginListItem struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Origin      pluginpkg.Origin    `json:"origin"`
	OriginLabel string              `json:"origin_label"`
	Enabled     bool                `json:"enabled"`
	Lifecycle   pluginpkg.Lifecycle `json:"lifecycle"`
}

type pluginConfigView struct {
	ID     pluginpkg.PluginID       `json:"id"`
	Name   string                   `json:"name"`
	Origin pluginpkg.Origin         `json:"origin"`
	Scope  string                   `json:"scope"`
	Schema pluginpkg.SettingsSchema `json:"schema"`
	Values map[string]any           `json:"values"`
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
	service, err := api.pluginService()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
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
		api.handlePluginConfigGet(w, id)
	case http.MethodPut:
		api.handlePluginConfigPut(w, r, id)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api API) handlePluginConfigGet(w http.ResponseWriter, id pluginpkg.PluginID) {
	view, status, err := api.pluginConfigView(id)
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
	service, err := api.pluginService()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
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
	view, status, err := api.pluginConfigView(id)
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
	service, err := api.pluginService()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	if err := service.ResetPluginSetting(r.Context(), id, request.Key); err != nil {
		http.Error(w, err.Error(), pluginConfigStatus(err))
		return
	}
	view, status, err := api.pluginConfigView(id)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	writeJSON(w, view)
}

func (api API) pluginConfigView(id pluginpkg.PluginID) (pluginConfigView, int, error) {
	service, err := api.pluginService()
	if err != nil {
		return pluginConfigView{}, http.StatusServiceUnavailable, err
	}
	schema, values, err := service.PluginSettings(id)
	if err != nil {
		return pluginConfigView{}, pluginConfigStatus(err), err
	}
	name, origin := string(id), pluginpkg.OriginInstalled
	if detail, err := service.InstalledDetail(id); err == nil {
		name, origin = detail.Manifest.Name, detail.Origin
	}
	return pluginConfigView{ID: id, Name: name, Origin: origin, Scope: "global", Schema: schema, Values: values}, 0, nil
}

func (api API) pluginService() (*application.PluginService, error) {
	if api.Plugins != nil {
		return api.Plugins, nil
	}
	return application.NewPluginService()
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
