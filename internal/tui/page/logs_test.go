package page

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/runtimecontrol"
	"go.mewis.me/chatgpt-mcp/internal/runtimeevent"
)

func TestLogsPageLoadsHistoryAndShowsOfflineReconnectState(t *testing.T) {
	root := setupLogsPageRoot(t)
	appendLogEvents(t, root, runtimeevent.Event{Sequence: 1, Time: time.Now(), RunID: "run_one", Level: "info", Name: "server.ready", Component: "SERVER", Message: "Ready"})
	page, err := NewLogs(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer page.Close()
	msg := page.Init()()
	updated, reconnect := page.Update(msg)
	page = updated.(*LogsPage)
	if len(page.events) != 1 || !page.loaded || page.connected || !page.reconnecting || reconnect == nil {
		t.Fatalf("page loaded=%t connected=%t reconnect=%t events=%d cmd=%v", page.loaded, page.connected, page.reconnecting, len(page.events), reconnect)
	}
	plain := ansi.Strip(page.View(100, 28))
	for _, want := range []string{"Logs", "RECONNECTING", "server.ready", "space Pause", "f Filters", "d Clear"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("view missing %q: %q", want, plain)
		}
	}
}

func TestLogsPageMergesLiveStreamAfterHistory(t *testing.T) {
	root := setupLogsPageRoot(t)
	base := time.Now().UTC()
	appendLogEvents(t, root, runtimeevent.Event{Sequence: 1, Time: base, RunID: "run_live", Level: "info", Name: "history", Component: "SERVER", Message: "History"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/events" || r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("request=%s auth=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: ready\ndata: {}\n\n")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		_, _ = fmt.Fprintf(w, "event: runtime\ndata: {\"sequence\":2,\"time\":%q,\"run_id\":\"run_live\",\"level\":\"warn\",\"event\":\"live\",\"component\":\"TOOL\",\"message\":\"Live\"}\n\n", base.Add(time.Second).Format(time.RFC3339Nano))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer server.Close()
	writeLogsRuntimeState(t, root, server.URL, "run_live")
	page, _ := NewLogs(t.Context())
	defer page.Close()
	updated, next := page.Update(page.Init()())
	page = updated.(*LogsPage)
	if !page.connected || next == nil || len(page.events) != 1 {
		t.Fatalf("connected=%t next=%v events=%d", page.connected, next, len(page.events))
	}
	updated, next = page.Update(next())
	page = updated.(*LogsPage)
	if len(page.events) != 2 || page.events[1].Name != "live" || next == nil {
		t.Fatalf("events=%#v next=%v", page.events, next)
	}
}

func TestLogsPagePauseBuffersWithoutFollowingAndResumeReturnsToTail(t *testing.T) {
	page, _ := NewLogs(t.Context())
	defer page.Close()
	base := time.Now().UTC()
	page.mergeEvents([]runtimeevent.Event{{Sequence: 1, Time: base, RunID: "run", Level: "info", Name: "one", Message: "one"}, {Sequence: 2, Time: base.Add(time.Second), RunID: "run", Level: "info", Name: "two", Message: "two"}})
	if !page.browser.SelectID("run:1") {
		t.Fatal("could not select first event")
	}
	page.appendEvent(runtimeevent.Event{Sequence: 3, Time: base.Add(2 * time.Second), RunID: "run", Level: "info", Name: "three", Message: "three"})
	if !page.paused || page.selectedID() != "run:1" || len(page.events) != 3 {
		t.Fatalf("paused=%t selected=%q events=%d", page.paused, page.selectedID(), len(page.events))
	}
	page.togglePause()
	if page.paused || page.selectedID() != "run:3" {
		t.Fatalf("resume paused=%t selected=%q", page.paused, page.selectedID())
	}
}

func TestLogsPageBufferIsBounded(t *testing.T) {
	page, _ := NewLogs(t.Context())
	defer page.Close()
	base := time.Now().UTC()
	events := make([]runtimeevent.Event, logsBufferCap+500)
	for index := range events {
		events[index] = runtimeevent.Event{Sequence: uint64(index + 1), Time: base.Add(time.Duration(index) * time.Millisecond), RunID: "run", Level: "info", Name: "event", Message: "value"}
	}
	page.mergeEvents(events)
	if len(page.events) != logsBufferCap || page.events[0].Sequence != 501 || page.events[len(page.events)-1].Sequence != uint64(logsBufferCap+500) {
		t.Fatalf("bounded events=%d first=%d last=%d", len(page.events), page.events[0].Sequence, page.events[len(page.events)-1].Sequence)
	}
}

func TestLogsPageDoesNotExposeDebugFields(t *testing.T) {
	page, _ := NewLogs(t.Context())
	defer page.Close()
	event := runtimeevent.Event{Sequence: 1, Time: time.Now(), RunID: "run", Level: "info", Name: "safe", Message: "message", Fields: []runtimeevent.Field{{Key: "visible", Value: "ok"}, {Key: "secret-debug", Value: "never-show", Visibility: logger.VisibilityDebug}}}
	row := page.logRow(event)
	joined := row.Search
	for _, tab := range row.DetailTabs {
		joined += "\n" + tab.Content
	}
	if !strings.Contains(joined, "visible") || strings.Contains(joined, "secret-debug") || strings.Contains(joined, "never-show") {
		t.Fatalf("row leaked hidden field: %q", joined)
	}
}

func TestLogsFilterFormUsesSharedQueryValidation(t *testing.T) {
	form, data := newLogsFilterForm(application.LogsQueryOptions{Tail: 100})
	_ = form
	data.Tail, data.Level, data.Event = "25", "warn", "tool.*"
	options, err := data.Options()
	if err != nil || options.Tail != 25 || options.Level != "warn" || options.Event != "tool.*" {
		t.Fatalf("options=%#v err=%v", options, err)
	}
	data.All, data.Session = true, "run_one"
	if _, err := data.Options(); err == nil {
		t.Fatal("all + session was accepted")
	}
}

func TestLogsClearRequiresExplicitConfirmationAndInfoShowsJournal(t *testing.T) {
	root := setupLogsPageRoot(t)
	appendLogEvents(t, root, runtimeevent.Event{Sequence: 1, Time: time.Now(), RunID: "run", Level: "info", Name: "one", Message: "one"})
	page, _ := NewLogs(t.Context())
	defer page.Close()
	if cmd := page.openCommand(LogsClear); cmd != nil || page.overlay != logsOverlayConfirm || page.confirm.AffirmativeSelected() {
		t.Fatalf("clear overlay=%d affirmative=%t cmd=%v", page.overlay, page.confirm.AffirmativeSelected(), cmd)
	}
	if cmd := page.updateClearConfirm(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd != nil || page.overlay != logsOverlayNone {
		t.Fatalf("default clear executed: overlay=%d cmd=%v", page.overlay, cmd)
	}
	page.openCommand(LogsClear)
	page.confirm.Select(true)
	if cmd := page.updateClearConfirm(tea.KeyPressMsg{Code: tea.KeyEnter}); cmd == nil || page.overlay != logsOverlayOperation {
		t.Fatalf("confirmed clear cmd=%v overlay=%d", cmd, page.overlay)
	}
	page.closeOverlay()
	page.openCommand(LogsInfo)
	if page.overlay != logsOverlayInfo || page.info.Path != runtimeevent.Path(root) || page.info.Files == 0 {
		t.Fatalf("info=%#v overlay=%d", page.info, page.overlay)
	}
}

func TestLogsPageCloseCancelsLiveStream(t *testing.T) {
	root := setupLogsPageRoot(t)
	closed := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: ready\ndata: {}\n\n")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
		closed <- struct{}{}
	}))
	defer server.Close()
	writeLogsRuntimeState(t, root, server.URL, "run")
	page, _ := NewLogs(t.Context())
	updated, next := page.Update(page.Init()())
	page = updated.(*LogsPage)
	if next == nil || !page.connected {
		t.Fatalf("next=%v connected=%t", next, page.connected)
	}
	page.Close()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("stream context not cancelled on page close")
	}
}

func setupLogsPageRoot(t *testing.T) string {
	t.Helper()
	previous := configformat.RootPath()
	t.Cleanup(func() { _ = configformat.SetRootPath(previous) })
	root := t.TempDir()
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	return root
}

func appendLogEvents(t *testing.T, root string, events ...runtimeevent.Event) {
	t.Helper()
	journal, err := runtimeevent.NewJournal(root, runtimeevent.Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if err := journal.Append(event); err != nil {
			t.Fatal(err)
		}
	}
}

func writeLogsRuntimeState(t *testing.T, root, rawURL, runID string) {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	state := runtimecontrol.State{PID: os.Getpid(), Address: parsed.Host, Token: "token", RunID: runID, ConfigRoot: root}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, runtimecontrol.FileName), data, 0600); err != nil {
		t.Fatal(err)
	}
}
