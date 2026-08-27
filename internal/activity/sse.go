package activity

import "net/http"

func Handler(stream *Stream) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}
		ch := stream.Subscribe()
		defer stream.Unsubscribe(ch)
		for {
			select {
			case <-r.Context().Done():
				return
			case event := <-ch:
				_, _ = w.Write([]byte("data: " + Encode(event) + "\n\n"))
				flusher.Flush()
			}
		}
	})
}
