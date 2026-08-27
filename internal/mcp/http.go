package mcp

import (
	"encoding/json"
	"net/http"
)

type HTTPServer struct {
	Runtime  ToolRuntime
	Sessions *SessionStore
}

func (s HTTPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var req Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, -32700, err.Error())
		return
	}
	resp := HandleRequest(s.Runtime, req)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
