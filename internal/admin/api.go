package admin

import "net/http"

func Handler() http.Handler {
	mux := http.NewServeMux()
	json := func(value string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(value))
		}
	}
	mux.HandleFunc("/api/health", json(`{"ok":true}`))
	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPut {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		json(`{}`)(w, r)
	})
	mux.HandleFunc("/api/workspaces", json(`[]`))
	mux.HandleFunc("/api/tools", json(`[{"name":"read_files","description":"Read workspace files"},{"name":"run_command","description":"Run workspace commands"}]`))
	mux.HandleFunc("/api/upstream", json(`[]`))
	return mux
}
