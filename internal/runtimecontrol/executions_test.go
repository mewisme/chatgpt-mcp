package runtimecontrol

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	shellruntime "go.mewis.me/chatgpt-mcp/internal/shell"
)

func TestExecutionFeedStreamReplaysCombinedEventsAndContinues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/executions/stream" || r.Header.Get("Authorization") != "Bearer runtime-secret" {
			t.Fatalf("request=%s auth=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: ready\ndata: {\"events\":[{\"sequence\":1,\"type\":\"started\",\"execution_id\":\"exec_1\",\"workspace_id\":\"ws_a\",\"execution\":{\"id\":\"exec_1\",\"workspace_id\":\"ws_a\",\"tool\":\"run_command\",\"command\":\"printf test\",\"cwd\":\"/tmp\",\"started_at\":\"2026-09-06T00:00:00Z\",\"status\":\"running\"},\"timestamp\":\"2026-09-06T00:00:00Z\"}],\"latest_sequence\":1}\n\nevent: heartbeat\ndata: {\"latest_sequence\":1}\n\nid: 2\nevent: output\ndata: {\"sequence\":2,\"type\":\"output\",\"execution_id\":\"exec_1\",\"workspace_id\":\"ws_a\",\"stream\":\"stderr\",\"data\":\"err\\n\",\"timestamp\":\"2026-09-06T00:00:01Z\"}\n\n")
	}))
	defer server.Close()
	root := setupRuntimeControlRoot(t)
	writeRuntimeControlState(t, root, server.URL, "runtime-secret")
	stream, state, err := OpenExecutionFeed(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	snapshot := stream.Snapshot()
	if state.PID <= 0 || snapshot.LatestSequence != 1 || len(snapshot.Events) != 1 || snapshot.Events[0].ExecutionID != "exec_1" {
		t.Fatalf("state=%#v snapshot=%#v", state, snapshot)
	}
	event, err := stream.Next()
	if err != nil {
		t.Fatal(err)
	}
	if event.Sequence != 2 || event.Type != shellruntime.ExecutionEventOutput || event.Stream != "stderr" || event.Data != "err\n" {
		t.Fatalf("event=%#v", event)
	}
}

func TestExecutionFeedStreamReportsOverflowAndPreservesStateOnFailure(t *testing.T) {
	t.Run("overflow", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprint(w, "event: ready\ndata: {\"events\":[],\"latest_sequence\":4}\n\nevent: overflow\ndata: {\"dropped_sequence\":5}\n\n")
		}))
		defer server.Close()
		root := setupRuntimeControlRoot(t)
		writeRuntimeControlState(t, root, server.URL, "runtime-secret")
		stream, _, err := OpenExecutionFeed(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer stream.Close()
		if _, err := stream.Next(); !errors.Is(err, ErrExecutionFeedOverflow) {
			t.Fatalf("overflow err=%v", err)
		}
	})
	t.Run("http", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprint(w, "offline")
		}))
		defer server.Close()
		root := setupRuntimeControlRoot(t)
		writeRuntimeControlState(t, root, server.URL, "runtime-secret")
		stream, state, err := OpenExecutionFeed(t.Context())
		if err == nil || stream != nil || state.PID <= 0 || !strings.Contains(err.Error(), "HTTP 503") {
			t.Fatalf("stream=%v state=%#v err=%v", stream, state, err)
		}
	})
}
