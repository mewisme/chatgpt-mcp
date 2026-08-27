package mcp

import (
	"encoding/json"
	"net/http"
)

func (h HTTPRuntime) serveNotificationStream(w http.ResponseWriter, r *http.Request) {
	id := ReadSessionID(r)
	if id == "" {
		http.Error(w, "missing MCP-Session-Id", http.StatusBadRequest)
		return
	}
	session, ok := h.Sessions.Get(id)
	if !ok {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case notification := <-session.Notifications:
			data, err := json.Marshal(notification)
			if err != nil {
				continue
			}
			_, _ = w.Write([]byte("data: " + string(data) + "\n\n"))
			flusher.Flush()
		}
	}
}
