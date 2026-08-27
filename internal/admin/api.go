package admin

import (
	"encoding/json"
	"net/http"

	"go.mewis.me/chatgpt-mcp/internal/upstream"
)

func Handler(manager *upstream.Manager) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(map[string]bool{"ok": true}) })
	mux.HandleFunc("/api/upstream", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			json.NewEncoder(w).Encode(manager.List())
		case http.MethodPost:
			var server upstream.Server
			if err := json.NewDecoder(r.Body).Decode(&server); err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			if err := manager.Add(server); err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			json.NewEncoder(w).Encode(server)
		}
	})
	return mux
}
