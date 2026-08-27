package admin

import "net/http"

func json(w http.ResponseWriter, value string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(value))
}

func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) { json(w, `{"ok":true}`) })
	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) { json(w, `{"port":0,"auth":true}`) })
	mux.HandleFunc("/api/workspaces", func(w http.ResponseWriter, r *http.Request) { json(w, `[]`) })
	mux.HandleFunc("/api/tools", func(w http.ResponseWriter, r *http.Request) {
		json(w, `[{"name":"read_files","description":"Read workspace files"},{"name":"run_command","description":"Run workspace command"}]`)
	})
	mux.HandleFunc("/api/upstream", func(w http.ResponseWriter, r *http.Request) { json(w, `[]`) })
	return mux
}
