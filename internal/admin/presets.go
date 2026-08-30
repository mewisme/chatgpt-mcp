package admin

import (
	"net/http"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/config"
)

type configPresetList struct {
	Current string          `json:"current"`
	Presets []config.Preset `json:"presets"`
}

func (api API) handleConfigPresets(w http.ResponseWriter, r *http.Request) {
	if api.Config == nil {
		http.Error(w, "config unavailable", http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, configPresetList{Current: config.MatchPreset(api.Config.Snapshot()), Presets: config.Presets()})
}

func (api API) handleConfigPreset(w http.ResponseWriter, r *http.Request) {
	if api.Config == nil {
		http.Error(w, "config unavailable", http.StatusServiceUnavailable)
		return
	}
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/config/presets/"), "/")
	if name == "" || strings.Contains(name, "/") {
		http.NotFound(w, r)
		return
	}
	preset, err := config.PresetByName(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, preset)
	case http.MethodPost:
		status := http.StatusInternalServerError
		next, err := api.Config.Update(func(next config.Config) (config.Config, error) {
			previous := next
			if err := config.ApplyPreset(&next, name); err != nil {
				status = http.StatusBadRequest
				return next, err
			}
			return next, api.persistConfigWithFeatures(next, previous)
		})
		if err != nil {
			http.Error(w, err.Error(), status)
			return
		}
		writeJSON(w, publicConfigView(next))
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}
