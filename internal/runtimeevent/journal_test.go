package runtimeevent

import (
	"os"
	"strings"
	"testing"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/logger"
)

func TestJournalPersistsHiddenEventsAndSanitizesSecrets(t *testing.T) {
	journal, err := NewJournal(t.TempDir(), Options{MaxBytes: 1 << 20, MaxFiles: 3, Metadata: Metadata{RunID: "run_test", PID: 42}})
	if err != nil {
		t.Fatal(err)
	}
	event := logger.Event{Time: time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC), Level: logger.Debug, Visibility: logger.VisibilityDebug, Kind: logger.KindInfo, Name: "tool.call.started", Component: "TOOL", Message: "Bearer mcp_supersecret", Fields: []logger.Field{logger.With("workspace", "ws_test"), logger.With("tool", "run_command"), logger.With("api_key", "secret"), logger.WithDebug("status", "ok")}}
	if err := journal.WriteEvent(event); err != nil {
		t.Fatal(err)
	}
	var got Event
	if err := ReadFile(journal.Path(), func(event Event) error { got = event; return nil }); err != nil {
		t.Fatal(err)
	}
	if got.Level != "debug" || got.Visibility != logger.VisibilityDebug || got.WorkspaceID != "ws_test" || got.Tool != "run_command" || got.Status != "ok" || got.RunID != "run_test" || got.PID != 42 {
		t.Fatalf("event = %#v", got)
	}
	data, err := os.ReadFile(journal.Path())
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "mcp_supersecret") || strings.Contains(text, `"api_key":"secret"`) || !strings.Contains(text, "<redacted>") {
		t.Fatalf("journal did not sanitize secrets: %s", text)
	}
}

func TestJournalRotatesOldestFirst(t *testing.T) {
	journal, err := NewJournal(t.TempDir(), Options{MaxBytes: 500, MaxFiles: 3})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		if err := journal.WriteEvent(logger.Event{Time: time.Now(), Level: logger.Info, Kind: logger.KindInfo, Name: "test.event", Component: "TEST", Message: strings.Repeat("x", 80)}); err != nil {
			t.Fatal(err)
		}
	}
	files := journal.FilesOldestFirst()
	if len(files) != 3 {
		t.Fatalf("files = %#v", files)
	}
	if !strings.HasSuffix(files[0], "runtime.jsonl.2") || !strings.HasSuffix(files[1], "runtime.jsonl.1") || !strings.HasSuffix(files[2], "runtime.jsonl") {
		t.Fatalf("unexpected order: %#v", files)
	}
}
