package admin

import (
	"encoding/json"
	"net/http"
)

func jsonResponse(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) { jsonResponse(w, map[string]any{"ok": true}) })
	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			jsonResponse(w, map[string]any{"ok": true})
			return
		}
		jsonResponse(w, map[string]any{})
	})
	mux.HandleFunc("/api/workspaces", func(w http.ResponseWriter, r *http.Request) { jsonResponse(w, []any{}) })
	mux.HandleFunc("/api/tools", func(w http.ResponseWriter, r *http.Request) { jsonResponse(w, []any{}) })
	mux.HandleFunc("/api/upstream", func(w http.ResponseWriter, r *http.Request) { jsonResponse(w, []any{}) })
	return mux
}
