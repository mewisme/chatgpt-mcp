package admin

import (
	"encoding/json"
	"net/http"
	"strings"
)

type API struct {
	Upstream any
}

func Handler() http.Handler { return HandlerWith(API{}) }

func HandlerWith(api API) http.Handler {
	mux := http.NewServeMux()
	jsonWrite := func(w http.ResponseWriter, value any) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(value)
	}
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) { jsonWrite(w, map[string]any{"ok": true}) })
	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) { jsonWrite(w, map[string]any{}) })
	mux.HandleFunc("/api/workspaces", func(w http.ResponseWriter, r *http.Request) { jsonWrite(w, []any{}) })
	mux.HandleFunc("/api/tools", func(w http.ResponseWriter, r *http.Request) { jsonWrite(w, []any{}) })
	mux.HandleFunc("/api/upstream", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			jsonWrite(w, map[string]any{"ok": true})
			return
		}
		if strings.HasPrefix(r.Method, "POST") {
			jsonWrite(w, map[string]any{"ok": true})
			return
		}
		jsonWrite(w, api.Upstream)
	})
	return mux
}
