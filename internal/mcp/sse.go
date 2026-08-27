package mcp

import (
	"fmt"
	"net/http"
)

type SSE struct{ Stream <-chan string }

func (s SSE) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	if s.Stream == nil {
		return
	}
	for msg := range s.Stream {
		_, _ = fmt.Fprintf(w, "data: %s\n\n", msg)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}
