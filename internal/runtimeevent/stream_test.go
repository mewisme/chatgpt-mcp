package runtimeevent

import (
	"testing"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/logger"
)

func TestRecorderPersistsAndPublishesSameSequence(t *testing.T) {
	metadata := Metadata{RunID: "run_test", PID: 42}
	journal, err := NewJournal(t.TempDir(), Options{Metadata: metadata})
	if err != nil {
		t.Fatal(err)
	}
	recorder := NewRecorder(journal, metadata)
	sub := recorder.Stream.Subscribe()
	defer recorder.Stream.Unsubscribe(sub)
	event := logger.Event{Time: time.Now(), Level: logger.Info, Kind: logger.KindSuccess, Name: "server.ready", Component: "SERVER", Message: "Server ready"}
	if err := recorder.WriteEvent(event); err != nil {
		t.Fatal(err)
	}
	live := <-sub
	if live.Sequence != 1 || live.RunID != "run_test" {
		t.Fatalf("live = %#v", live)
	}
	var persisted Event
	if err := ReadFile(journal.Path(), func(event Event) error { persisted = event; return nil }); err != nil {
		t.Fatal(err)
	}
	if persisted.Sequence != live.Sequence || persisted.Name != live.Name {
		t.Fatalf("persisted=%#v live=%#v", persisted, live)
	}
}

func TestRecorderRecordAddsRuntimeMetadataToSyntheticEvent(t *testing.T) {
	metadata := Metadata{RunID: "run_session", PID: 77, Managed: true, ServiceID: "service_test", ServiceScope: "user"}
	journal, err := NewJournal(t.TempDir(), Options{Metadata: metadata})
	if err != nil {
		t.Fatal(err)
	}
	recorder := NewRecorder(journal, metadata)
	if err := recorder.Record(Event{Level: "info", Kind: "action", Name: "runtime.session.started", Component: "SESSION", Message: "Runtime session started"}); err != nil {
		t.Fatal(err)
	}
	var got Event
	if err := ReadFile(journal.Path(), func(event Event) error { got = event; return nil }); err != nil {
		t.Fatal(err)
	}
	if got.Sequence != 1 || got.RunID != metadata.RunID || got.PID != metadata.PID || !got.Managed || got.ServiceID != metadata.ServiceID || got.ServiceScope != metadata.ServiceScope || got.Time.IsZero() {
		t.Fatalf("synthetic event = %#v", got)
	}
}
