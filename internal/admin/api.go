package admin

import (
	"encoding/json"
	"net/http"

	"go.mewis.me/chatgpt-mcp/internal/tools"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
)

type API struct {
	Upstream *upstream.Manager
	Tools    *tools.Registry
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func New(api API) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, map[string]bool{"ok": true}) })
	mux.HandleFunc("/api/tools", func(w http.ResponseWriter, r *http.Request) {
		if api.Tools == nil {
			writeJSON(w, []any{})
			return
		}
		writeJSON(w, api.Tools.ListSchemas())
	})
	mux.HandleFunc("/api/upstream", func(w http.ResponseWriter, r *http.Request) {
		if api.Upstream == nil {
			writeJSON(w, []any{})
			return
		}
		writeJSON(w, api.Upstream.List())
	})
	return mux
}

func Handler() http.Handler { return New(API{}) }
