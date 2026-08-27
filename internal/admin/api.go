package admin

import (
	"encoding/json"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
	"net/http"
)

func Handler() http.Handler {
	mux := http.NewServeMux()
	manager := upstream.NewManager()
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(map[string]any{"ok": true}) })
	mux.HandleFunc("/api/upstream", func(w http.ResponseWriter, r *http.Request) { json.NewEncoder(w).Encode(manager.List()) })
	return mux
}
