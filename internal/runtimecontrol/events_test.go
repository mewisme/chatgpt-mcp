package runtimecontrol

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEventStreamSkipsControlFramesAndDecodesRuntimeEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/events" || r.Header.Get("Authorization") != "Bearer runtime-secret" {
			t.Fatalf("request=%s auth=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: ready\ndata: {\"latest_sequence\":1}\n\nevent: heartbeat\ndata: {}\n\nid: 2\nevent: runtime\ndata: {\"sequence\":2,\"time\":\"2026-09-06T00:00:00Z\",\"level\":\"info\",\"kind\":\"success\",\"event\":\"server.ready\",\"message\":\"Ready\"}\n\n")
	}))
	defer server.Close()
	root := setupRuntimeControlRoot(t)
	writeRuntimeControlState(t, root, server.URL, "runtime-secret")
	stream, state, err := OpenEvents(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	if state.PID <= 0 {
		t.Fatalf("state=%#v", state)
	}
	event, err := stream.Next()
	if err != nil {
		t.Fatal(err)
	}
	if event.Sequence != 2 || event.Name != "server.ready" || event.Message != "Ready" {
		t.Fatalf("event=%#v", event)
	}
}

func TestEventStreamStopsOnContextCancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if flusher, ok := w.(http.Flusher); ok {
			_, _ = fmt.Fprint(w, "event: ready\ndata: {}\n\n")
			flusher.Flush()
		}
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()
	root := setupRuntimeControlRoot(t)
	writeRuntimeControlState(t, root, server.URL, "runtime-secret")
	ctx, cancel := context.WithCancel(t.Context())
	stream, _, err := OpenEvents(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	<-started
	done := make(chan error, 1)
	go func() { _, err := stream.Next(); done <- err }()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("event stream did not stop after cancellation")
	}
}
