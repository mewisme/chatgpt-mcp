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
