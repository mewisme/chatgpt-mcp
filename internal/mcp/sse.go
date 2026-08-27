package mcp

import (
	"net/http"

	"go.mewis.me/chatgpt-mcp/internal/activity"
)

type SSEHandler struct{ Stream *activity.Stream }

func (s SSEHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}
	ch := s.Stream.Subscribe()
	defer s.Stream.Unsubscribe(ch)
	for event := range ch {
		_, _ = w.Write([]byte("data: " + activity.Encode(event) + "\n\n"))
		flusher.Flush()
	}
}
