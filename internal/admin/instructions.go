package admin

import (
	"net/http"
	"os"

	"go.mewis.me/chatgpt-mcp/internal/instructioncontext"
	"go.mewis.me/chatgpt-mcp/internal/instructionpolicy"
)

type instructionSettingsResponse struct {
	Version         int                                       `json:"version"`
	Context         string                                    `json:"context"`
	Rules           []instructionpolicy.GlobalRule            `json:"rules"`
	SourcePolicy    map[string]instructionpolicy.SourcePolicy `json:"source_policy"`
	DetectedSources []instructioncontext.SourceSnapshot       `json:"detected_sources"`
}

type instructionSettingsPatch struct {
	Context      *string                                   `json:"context,omitempty"`
	Rules        *[]instructionpolicy.GlobalRule           `json:"rules,omitempty"`
	SourcePolicy map[string]instructionpolicy.SourcePolicy `json:"source_policy,omitempty"`
}

func (api API) handleGlobalInstructions(w http.ResponseWriter, r *http.Request) {
	store := instructionpolicy.DefaultStore()
	switch r.Method {
	case http.MethodGet:
		value, err := store.Load()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		response, err := instructionSettingsView(value)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, response)
	case http.MethodPut:
		var patch instructionSettingsPatch
		if err := decodeJSONBody(w, r, &patch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		value, err := store.Load()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if patch.Context != nil {
			value.Context = *patch.Context
		}
		if patch.Rules != nil {
			value.Rules = append([]instructionpolicy.GlobalRule(nil), (*patch.Rules)...)
		}
		if value.Sources == nil {
			value.Sources = map[string]instructionpolicy.SourcePolicy{}
		}
		for provider, source := range patch.SourcePolicy {
			value.Sources[instructionpolicy.ProviderID(provider)] = source
		}
		if err := store.Save(value); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		response, err := instructionSettingsView(value)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, response)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func instructionSettingsView(value instructionpolicy.Config) (instructionSettingsResponse, error) {
	home, _ := os.UserHomeDir()
	sources, err := instructioncontext.DiscoverUserSources(home, value)
	if err != nil {
		return instructionSettingsResponse{}, err
	}
	return instructionSettingsResponse{
		Version: value.Version, Context: value.Context, Rules: value.Rules, SourcePolicy: value.Sources, DetectedSources: sources,
	}, nil
}
