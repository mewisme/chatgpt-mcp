package activity

import (
	"net/http"
	"strconv"
)

func Handler(stream *Stream) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		ch, recent := stream.SubscribeWithRecent(historyLimit(r))
		defer stream.Unsubscribe(ch)
		for _, event := range recent {
			if _, err := w.Write([]byte("data: " + Encode(event) + "\n\n")); err != nil {
				return
			}
		}
		flusher.Flush()
		for {
			select {
			case <-r.Context().Done():
				return
			case event, ok := <-ch:
				if !ok {
					return
				}
				if _, err := w.Write([]byte("data: " + Encode(event) + "\n\n")); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	})
}

func historyLimit(r *http.Request) int {
	const fallback = 100
	raw := r.URL.Query().Get("history")
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return fallback
	}
	if value > defaultRecentLimit {
		return defaultRecentLimit
	}
	return value
}
