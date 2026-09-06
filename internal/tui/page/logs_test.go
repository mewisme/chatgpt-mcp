package page

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/runtimecontrol"
	"go.mewis.me/chatgpt-mcp/internal/runtimeevent"
	shellruntime "go.mewis.me/chatgpt-mcp/internal/shell"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
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
	updated, connect := page.Update(msg)
	page = updated.(*LogsPage)
	if len(page.events) != 1 || !page.loaded || page.connected || !page.reconnecting || connect == nil {
		t.Fatalf("journal loaded=%t connected=%t reconnect=%t events=%d cmd=%v", page.loaded, page.connected, page.reconnecting, len(page.events), connect)
	}
	updated, reconnect := page.Update(connect())
	page = updated.(*LogsPage)
	if page.connected || !page.reconnecting || reconnect == nil {
		t.Fatalf("offline stream connected=%t reconnect=%t cmd=%v", page.connected, page.reconnecting, reconnect)
	}
	plain := ansi.Strip(page.View(180, 28))
	for _, want := range []string{"Runtime", "Command Execution", "RECONNECTING", "server.ready", "? more"} {
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
	updated, connect := page.Update(page.Init()())
	page = updated.(*LogsPage)
	if page.connected || connect == nil || len(page.events) != 1 {
		t.Fatalf("history connected=%t connect=%v events=%d", page.connected, connect, len(page.events))
	}
	updated, next := page.Update(connect())
	page = updated.(*LogsPage)
	if !page.connected || next == nil {
		t.Fatalf("connected=%t next=%v", page.connected, next)
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

func TestLogsPageFollowKeepsOpenDetailPinnedWhileSelectingLatest(t *testing.T) {
	page, _ := NewLogs(t.Context())
	defer page.Close()
	base := time.Now().UTC()
	page.mergeEvents([]runtimeevent.Event{
		{Sequence: 1, Time: base, RunID: "run", Level: "info", Name: "one", Message: "one"},
		{Sequence: 2, Time: base.Add(time.Second), RunID: "run", Level: "info", Name: "two", Message: "two"},
	})
	if !page.browser.OpenDetail("run:2") || page.paused {
		t.Fatalf("detail=%t paused=%t", page.browser.DetailOpen(), page.paused)
	}
	page.appendEvent(runtimeevent.Event{Sequence: 3, Time: base.Add(2 * time.Second), RunID: "run", Level: "info", Name: "three", Message: "three"})
	if page.paused || page.selectedID() != "run:3" || !page.browser.DetailOpen() {
		t.Fatalf("follow paused=%t selected=%q detail=%t", page.paused, page.selectedID(), page.browser.DetailOpen())
	}
	detail := ansi.Strip(page.browser.Content())
	if !strings.Contains(detail, "Log event · two") || strings.Contains(detail, "Log event · three") {
		t.Fatalf("detail jumped after live append: %q", detail)
	}
	updated, _ := page.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	page = updated.(*LogsPage)
	if page.browser.DetailOpen() || page.paused || page.selectedID() != "run:3" {
		t.Fatalf("after close detail=%t paused=%t selected=%q", page.browser.DetailOpen(), page.paused, page.selectedID())
	}
	page.appendEvent(runtimeevent.Event{Sequence: 4, Time: base.Add(3 * time.Second), RunID: "run", Level: "info", Name: "four", Message: "four"})
	if page.paused || page.selectedID() != "run:4" {
		t.Fatalf("follow did not continue paused=%t selected=%q", page.paused, page.selectedID())
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

func TestLogsPageMouseActionsUseKeyboardMessages(t *testing.T) {
	page, _ := NewLogs(t.Context())
	defer page.Close()
	page.width, page.height = 180, 28
	updated, _ := page.browser.Update(tea.KeyPressMsg{Code: '?'})
	page.browser = updated.(component.Browser)
	_ = page.View(page.width, page.height)
	targets := page.MouseTargets(0, 0, 1)
	want := map[string]bool{"space": false, "f": false, "r": false, "i": false, "d": false}
	for _, target := range targets {
		if target.ID != "browser.help" {
			continue
		}
		message, ok := target.Handle(component.MouseEvent{Button: tea.MouseLeft}).(tea.KeyPressMsg)
		if ok {
			if _, exists := want[message.String()]; exists {
				want[message.String()] = true
			}
		}
	}
	for key, found := range want {
		if !found {
			t.Errorf("logs mouse action %q not found", key)
		}
	}
}

func TestShortValuePreservesUTF8AndDisplayWidth(t *testing.T) {
	for _, tc := range []struct {
		value string
		limit int
	}{
		{value: "workspace-你好-very-long", limit: 12},
		{value: "café-déjà-vu", limit: 8},
		{value: "🙂🙂🙂", limit: 3},
	} {
		got := shortValue(tc.value, tc.limit)
		if !utf8.ValidString(got) {
			t.Fatalf("shortValue(%q, %d) returned invalid UTF-8: %q", tc.value, tc.limit, got)
		}
		if lipgloss.Width(got) > tc.limit {
			t.Fatalf("shortValue(%q, %d) width=%d value=%q", tc.value, tc.limit, lipgloss.Width(got), got)
		}
	}
}

func TestLogsPageDefaultsToVerboseWithoutExposingDebugFields(t *testing.T) {
	page, _ := NewLogs(t.Context())
	defer page.Close()
	if page.visibility != logger.VisibilityVerbose {
		t.Fatalf("default visibility=%d", page.visibility)
	}
	event := runtimeevent.Event{Sequence: 1, Time: time.Now(), RunID: "run", Level: "info", Name: "safe", Message: "message", Fields: []runtimeevent.Field{{Key: "visible", Value: "ok"}, {Key: "verbose", Value: "useful", Visibility: logger.VisibilityVerbose}, {Key: "secret-debug", Value: "never-show", Visibility: logger.VisibilityDebug}}}
	row := page.logRow(event)
	joined := row.Search
	for _, tab := range row.DetailTabs {
		joined += "\n" + tab.Content
	}
	if !strings.Contains(joined, "visible") || !strings.Contains(joined, "verbose") || !strings.Contains(joined, "useful") || strings.Contains(joined, "secret-debug") || strings.Contains(joined, "never-show") {
		t.Fatalf("row leaked hidden field: %q", joined)
	}
}

func TestLogsPageDefaultVerboseShowsRuntimeEventsButHidesDebugNoise(t *testing.T) {
	root := setupLogsPageRoot(t)
	base := time.Now().UTC()
	appendLogEvents(t, root,
		runtimeevent.Event{Sequence: 1, Time: base, RunID: "run_visibility", Level: "info", Kind: "info", Name: "approval.requested", Component: "APPROVAL", Message: "Control approval requested", Visibility: logger.VisibilityDefault},
		runtimeevent.Event{Sequence: 2, Time: base.Add(time.Millisecond), RunID: "run_visibility", Level: "info", Kind: "success", Name: "tunnel.connected", Component: "TUNNEL", Message: "Tunnel connected", Visibility: logger.VisibilityDefault},
		runtimeevent.Event{Sequence: 3, Time: base.Add(2 * time.Millisecond), RunID: "run_visibility", Level: "info", Kind: "success", Name: "tool.call.completed", Component: "TOOL", Message: "Tool call completed", Tool: "run_command", Status: "ok", Visibility: logger.VisibilityVerbose},
		runtimeevent.Event{Sequence: 4, Time: base.Add(3 * time.Millisecond), RunID: "run_visibility", Level: "debug", Kind: "info", Name: "tool.call.started", Component: "TOOL", Message: "Tool call started", Tool: "run_command", Visibility: logger.VisibilityDebug},
	)
	writeLogsRuntimeState(t, root, "http://127.0.0.1:1", "run_visibility")
	page, _ := NewLogs(t.Context())
	defer page.Close()
	updated, _ := page.Update(page.Init()())
	page = updated.(*LogsPage)
	got := make([]string, 0, len(page.events))
	for _, event := range page.events {
		got = append(got, event.Name)
	}
	want := []string{"approval.requested", "tunnel.connected", "tool.call.completed"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("default verbose events=%#v want %#v", got, want)
	}
	status := ansi.Strip(page.statusView(180))
	if !strings.Contains(status, "View") || !strings.Contains(status, "verbose") {
		t.Fatalf("status missing visibility: %q", status)
	}
}

func TestLogsPageLiveVisibilityNormalVerboseAndDebug(t *testing.T) {
	page, _ := NewLogs(t.Context())
	defer page.Close()
	page.generation = 1
	page.query.RunID = "run_live_visibility"
	page.streamRunID = "run_live_visibility"
	base := time.Now().UTC()
	feed := func(sequence uint64, name string, visibility logger.Visibility) {
		page.finishStreamEvent(logsStreamEventMsg{generation: 1, event: runtimeevent.Event{Sequence: sequence, Time: base.Add(time.Duration(sequence) * time.Millisecond), RunID: "run_live_visibility", Level: "info", Name: name, Message: name, Visibility: visibility}})
	}

	page.visibility = logger.VisibilityDefault
	feed(1, "normal", logger.VisibilityDefault)
	feed(2, "verbose-hidden", logger.VisibilityVerbose)
	if len(page.events) != 1 || page.events[0].Name != "normal" || page.streamSeq != 2 {
		t.Fatalf("normal view events=%#v seq=%d", page.events, page.streamSeq)
	}

	page.visibility = logger.VisibilityVerbose
	feed(3, "verbose", logger.VisibilityVerbose)
	feed(4, "debug-hidden", logger.VisibilityDebug)
	if len(page.events) != 2 || page.events[1].Name != "verbose" || page.streamSeq != 4 {
		t.Fatalf("verbose view events=%#v seq=%d", page.events, page.streamSeq)
	}

	page.visibility = logger.VisibilityDebug
	feed(5, "debug", logger.VisibilityDebug)
	if len(page.events) != 3 || page.events[2].Name != "debug" || page.streamSeq != 5 {
		t.Fatalf("debug view events=%#v seq=%d", page.events, page.streamSeq)
	}
}

func TestLogsFilterFormUsesSharedQueryValidation(t *testing.T) {
	form, data := newLogsFilterForm(application.LogsQueryOptions{Tail: 100}, logger.VisibilityVerbose)
	_ = form
	if data.Visibility != "verbose" {
		t.Fatalf("form visibility=%q", data.Visibility)
	}
	data.Tail, data.Level, data.Event = "25", "warn", "tool.*"
	options, visibility, err := data.Options()
	if err != nil || options.Tail != 25 || options.Level != "warn" || options.Event != "tool.*" || visibility != logger.VisibilityVerbose {
		t.Fatalf("options=%#v visibility=%d err=%v", options, visibility, err)
	}
	data.All, data.Session = true, "run_one"
	if _, _, err := data.Options(); err == nil {
		t.Fatal("all + session was accepted")
	}
	data.All, data.Session, data.Visibility = false, "", "invalid"
	if _, _, err := data.Options(); err == nil {
		t.Fatal("invalid visibility was accepted")
	}
}

func TestLogsVisibilityValuesRoundTrip(t *testing.T) {
	for _, test := range []struct {
		visibility logger.Visibility
		value      string
	}{
		{visibility: logger.VisibilityDefault, value: "normal"},
		{visibility: logger.VisibilityVerbose, value: "verbose"},
		{visibility: logger.VisibilityDebug, value: "debug"},
	} {
		if got := logsVisibilityValue(test.visibility); got != test.value {
			t.Fatalf("logsVisibilityValue(%d)=%q want %q", test.visibility, got, test.value)
		}
		got, err := parseLogsVisibility(test.value)
		if err != nil || got != test.visibility {
			t.Fatalf("parseLogsVisibility(%q)=%d,%v want %d", test.value, got, err, test.visibility)
		}
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
	updated, connect := page.Update(page.Init()())
	page = updated.(*LogsPage)
	if connect == nil || page.connected {
		t.Fatalf("connect=%v connected=%t", connect, page.connected)
	}
	updated, next := page.Update(connect())
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

func TestLogsPageLoadsCurrentRuntimeSessionBeforeOpeningStream(t *testing.T) {
	root := setupLogsPageRoot(t)
	base := time.Now().UTC()
	appendLogEvents(t, root,
		runtimeevent.Event{Sequence: 1, Time: base, RunID: "run_current", Level: "info", Name: "current", Message: "Current runtime"},
		runtimeevent.Event{Sequence: 1, Time: base.Add(time.Second), RunID: "run_old", Level: "info", Name: "old", Message: "Latest journal entry but old session"},
	)
	writeLogsRuntimeState(t, root, "http://127.0.0.1:1", "run_current")
	page, _ := NewLogs(t.Context())
	defer page.Close()
	updated, connect := page.Update(page.Init()())
	page = updated.(*LogsPage)
	if connect == nil || page.query.RunID != "run_current" || len(page.events) != 1 || page.events[0].Name != "current" {
		t.Fatalf("session=%q events=%#v connect=%v", page.query.RunID, page.events, connect)
	}
}

func TestLogsStatusShowsFullSessionID(t *testing.T) {
	page, _ := NewLogs(t.Context())
	session := "run_0123456789abcdef0123456789abcdef0123456789abcdef"
	page.query.RunID = session
	plain := ansi.Strip(page.statusView(160))
	if !strings.Contains(plain, session) || strings.Contains(plain, "run_0123456789abcdef0123…") {
		t.Fatalf("session was truncated: %q", plain)
	}
}

func TestLogsClearKeepsLiveStreamAndStableNotice(t *testing.T) {
	page, _ := NewLogs(t.Context())
	page.connected = true
	page.loaded = true
	page.generation = 7
	page.clearSeq = 3
	page.streamRunID = "run_live"
	page.streamSeq = 41
	page.events = []runtimeevent.Event{{RunID: "run_live", Sequence: 41, Message: "before clear"}}
	updated, _ := page.Update(logsClearMsg{operation: 3})
	page = updated.(*LogsPage)
	if !page.connected || page.generation != 7 || len(page.events) != 0 || page.notice != "Runtime logs cleared" {
		t.Fatalf("clear state connected=%t generation=%d events=%d notice=%q", page.connected, page.generation, len(page.events), page.notice)
	}
	page.finishStreamEvent(logsStreamEventMsg{generation: 7, event: runtimeevent.Event{RunID: "run_live", Sequence: 42, Message: "after clear"}})
	if page.notice != "Runtime logs cleared" || page.generation != 7 || page.streamSeq != 42 {
		t.Fatalf("live stream changed clear feedback: notice=%q generation=%d sequence=%d", page.notice, page.generation, page.streamSeq)
	}
}

func TestLogsPageResyncsJournalWhenLiveSequenceHasGap(t *testing.T) {
	root := setupLogsPageRoot(t)
	appendLogEvents(t, root,
		runtimeevent.Event{Sequence: 1, Time: time.Now().UTC(), RunID: "run_gap", Level: "info", Name: "one", Message: "one"},
		runtimeevent.Event{Sequence: 2, Time: time.Now().UTC(), RunID: "run_gap", Level: "info", Name: "two", Message: "two"},
		runtimeevent.Event{Sequence: 3, Time: time.Now().UTC(), RunID: "run_gap", Level: "info", Name: "three", Message: "three"},
	)
	page, _ := NewLogs(t.Context())
	defer page.Close()
	page.generation = 4
	page.connected = true
	page.query.RunID = "run_gap"
	page.streamRunID, page.streamSeq = "run_gap", 1
	cmd := page.finishStreamEvent(logsStreamEventMsg{generation: 4, event: runtimeevent.Event{Sequence: 3, Time: time.Now().UTC(), RunID: "run_gap", Level: "info", Name: "three", Message: "three"}})
	if cmd == nil || page.generation != 5 || len(page.events) != 0 || !strings.Contains(page.notice, "gap") {
		t.Fatalf("gap resync generation=%d events=%d notice=%q cmd=%v", page.generation, len(page.events), page.notice, cmd)
	}
}

func TestLogsPageResyncsWhenStreamAdvancedDuringJournalLoad(t *testing.T) {
	root := setupLogsPageRoot(t)
	base := time.Now().UTC()
	appendLogEvents(t, root, runtimeevent.Event{Sequence: 1, Time: base, RunID: "run_race", Level: "info", Name: "one", Message: "one"})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: ready\ndata: {\"latest_sequence\":2}\n\n")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer server.Close()
	writeLogsRuntimeState(t, root, server.URL, "run_race")
	page, _ := NewLogs(t.Context())
	defer page.Close()
	updated, connect := page.Update(page.Init()())
	page = updated.(*LogsPage)
	if page.streamSeq != 1 || connect == nil {
		t.Fatalf("journal watermark=%d connect=%v", page.streamSeq, connect)
	}
	generation := page.generation
	updated, resync := page.Update(connect())
	page = updated.(*LogsPage)
	if resync == nil || page.generation != generation+1 || page.connected || !strings.Contains(page.notice, "advanced") {
		t.Fatalf("resync generation=%d connected=%t notice=%q cmd=%v", page.generation, page.connected, page.notice, resync)
	}
}

func TestLogsPageExplicitSessionAndAllOverrideRuntimeSessionSelection(t *testing.T) {
	root := setupLogsPageRoot(t)
	base := time.Now().UTC()
	appendLogEvents(t, root,
		runtimeevent.Event{Sequence: 1, Time: base, RunID: "run_current", Level: "info", Name: "current", Message: "current"},
		runtimeevent.Event{Sequence: 1, Time: base.Add(time.Second), RunID: "run_old", Level: "info", Name: "old", Message: "old"},
	)
	writeLogsRuntimeState(t, root, "http://127.0.0.1:1", "run_current")
	for _, test := range []struct {
		name      string
		configure func(*LogsPage)
		wantRun   string
		wantCount int
	}{
		{name: "explicit-session", configure: func(page *LogsPage) { page.options.Session = "run_old" }, wantRun: "run_old", wantCount: 1},
		{name: "all", configure: func(page *LogsPage) { page.options.All = true }, wantRun: "", wantCount: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			page, _ := NewLogs(t.Context())
			defer page.Close()
			test.configure(page)
			updated, connect := page.Update(page.Init()())
			page = updated.(*LogsPage)
			if connect == nil || page.query.RunID != test.wantRun || len(page.events) != test.wantCount {
				t.Fatalf("run=%q events=%d connect=%v", page.query.RunID, len(page.events), connect)
			}
		})
	}
}

func TestLogsPageStreamSequenceTracksFilteredEventsWithoutFalseGap(t *testing.T) {
	page, _ := NewLogs(t.Context())
	defer page.Close()
	page.generation = 7
	page.query = runtimeevent.Query{RunID: "run_filter", MinLevel: "error"}
	page.streamRunID = "run_filter"
	if cmd := page.finishStreamEvent(logsStreamEventMsg{generation: 7, event: runtimeevent.Event{Sequence: 1, Time: time.Now().UTC(), RunID: "run_filter", Level: "info", Name: "hidden", Message: "hidden"}}); cmd != nil {
		t.Fatalf("filtered event unexpectedly scheduled command: %v", cmd)
	}
	if page.streamSeq != 1 || len(page.events) != 0 || page.generation != 7 {
		t.Fatalf("filtered sequence=%d events=%d generation=%d", page.streamSeq, len(page.events), page.generation)
	}
	page.finishStreamEvent(logsStreamEventMsg{generation: 7, event: runtimeevent.Event{Sequence: 2, Time: time.Now().UTC(), RunID: "run_filter", Level: "error", Name: "visible", Message: "visible"}})
	if page.streamSeq != 2 || len(page.events) != 1 || page.events[0].Name != "visible" || page.generation != 7 {
		t.Fatalf("visible sequence=%d events=%#v generation=%d", page.streamSeq, page.events, page.generation)
	}
}

func TestLogsPageUpdateCoversBrowserActionsAndOverlays(t *testing.T) {
	root := setupLogsPageRoot(t)
	appendLogEvents(t, root, runtimeevent.Event{Sequence: 1, Time: time.Now().UTC(), RunID: "run_ui", Level: "info", Name: "one", Message: "one"})
	page, _ := NewLogs(t.Context())
	defer page.Close()
	if page.OverlayActive() || page.InputActive() {
		t.Fatal("new logs page unexpectedly active")
	}
	updated, _ := page.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	page = updated.(*LogsPage)
	if page.width != 120 || page.height != 30 {
		t.Fatalf("size=%dx%d", page.width, page.height)
	}

	updated, _ = page.Update(LogsCommandMsg{Command: LogsFilter})
	page = updated.(*LogsPage)
	if page.overlay != logsOverlayForm || !page.OverlayActive() || !page.InputActive() {
		t.Fatalf("filter overlay=%d active=%t input=%t", page.overlay, page.OverlayActive(), page.InputActive())
	}
	updated, _ = page.Update(component.FormCancelledMsg{})
	page = updated.(*LogsPage)
	if page.overlay != logsOverlayNone || page.OverlayActive() || page.InputActive() {
		t.Fatalf("cancel overlay=%d active=%t input=%t", page.overlay, page.OverlayActive(), page.InputActive())
	}

	updated, _ = page.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	page = updated.(*LogsPage)
	if page.overlay != logsOverlayInfo || !page.OverlayActive() {
		t.Fatalf("info overlay=%d", page.overlay)
	}
	updated, _ = page.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	page = updated.(*LogsPage)
	if page.overlay != logsOverlayNone {
		t.Fatalf("info close overlay=%d", page.overlay)
	}

	updated, _ = page.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	page = updated.(*LogsPage)
	if page.overlay != logsOverlayConfirm || page.confirm.AffirmativeSelected() {
		t.Fatalf("clear overlay=%d affirmative=%t", page.overlay, page.confirm.AffirmativeSelected())
	}
	updated, _ = page.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	page = updated.(*LogsPage)
	if page.overlay != logsOverlayNone {
		t.Fatalf("clear cancel overlay=%d", page.overlay)
	}

	updated, _ = page.Update(LogsCommandMsg{Command: LogsToggle})
	page = updated.(*LogsPage)
	if !page.paused {
		t.Fatal("toggle command did not pause")
	}
	updated, _ = page.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	page = updated.(*LogsPage)
	if page.paused {
		t.Fatal("space did not resume")
	}
}

func TestLogsPageUpdateSubmitsAndRejectsFilters(t *testing.T) {
	setupLogsPageRoot(t)
	page, _ := NewLogs(t.Context())
	defer page.Close()
	page.form, page.filterForm = newLogsFilterForm(page.options, page.visibility)
	page.overlay = logsOverlayForm
	page.filterForm.Tail = "25"
	page.filterForm.Visibility = "debug"
	page.filterForm.Level = "warn"
	page.filterForm.Event = "tool.*"
	updated, bootstrap := page.Update(component.FormSubmittedMsg{})
	page = updated.(*LogsPage)
	if bootstrap == nil || page.overlay != logsOverlayNone || page.options.Tail != 25 || page.options.Level != "warn" || page.options.Event != "tool.*" || page.visibility != logger.VisibilityDebug {
		t.Fatalf("options=%#v visibility=%d overlay=%d cmd=%v", page.options, page.visibility, page.overlay, bootstrap)
	}

	page.form, page.filterForm = newLogsFilterForm(page.options, page.visibility)
	page.overlay = logsOverlayForm
	page.filterForm.Tail = "-1"
	updated, bootstrap = page.Update(component.FormSubmittedMsg{})
	page = updated.(*LogsPage)
	if bootstrap != nil || page.err == nil || page.overlay != logsOverlayForm {
		t.Fatalf("invalid filter err=%v overlay=%d cmd=%v", page.err, page.overlay, bootstrap)
	}
}

func TestLogsPageStreamOpenAndReconnectBranches(t *testing.T) {
	page, _ := NewLogs(t.Context())
	defer page.Close()
	page.generation = 10
	page.loaded = true
	page.query.RunID = "run_one"
	page.streamRunID, page.streamSeq = "run_one", 3

	staleServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "event: ready\ndata: {\"latest_sequence\":3}\n\n")
	}))
	defer staleServer.Close()
	root := setupLogsPageRoot(t)
	writeLogsRuntimeState(t, root, staleServer.URL, "run_one")
	stream, state, err := runtimecontrol.OpenEvents(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if cmd := page.finishStreamOpen(logsStreamOpenMsg{generation: 9, stream: stream, state: state}); cmd != nil {
		t.Fatalf("stale stream scheduled cmd=%v", cmd)
	}

	page.generation = 10
	page.err = nil
	cmd := page.finishStreamOpen(logsStreamOpenMsg{generation: 10, state: state, err: fmt.Errorf("offline")})
	if cmd == nil || page.connected || !page.reconnecting || !strings.Contains(page.notice, "offline") {
		t.Fatalf("offline connected=%t reconnect=%t notice=%q cmd=%v", page.connected, page.reconnecting, page.notice, cmd)
	}

	page.generation = 10
	page.connected = false
	page.reconnecting = true
	updated, connect := page.Update(logsReconnectMsg(9))
	page = updated.(*LogsPage)
	if connect != nil || page.generation != 10 {
		t.Fatalf("stale reconnect generation=%d cmd=%v", page.generation, connect)
	}
}

func TestLogsPageStreamEventDisconnectAndGapHelpers(t *testing.T) {
	page, _ := NewLogs(t.Context())
	defer page.Close()
	page.generation = 2
	page.connected = true
	page.streamRunID, page.streamSeq = "run", 4
	if page.streamGap(runtimeevent.Event{}) {
		t.Fatal("empty event reported gap")
	}
	if page.streamGap(runtimeevent.Event{RunID: "other", Sequence: 7}) || page.streamRunID != "other" || page.streamSeq != 7 {
		t.Fatalf("new run tracking=%q/%d", page.streamRunID, page.streamSeq)
	}
	if page.streamGap(runtimeevent.Event{RunID: "other", Sequence: 8}) {
		t.Fatal("sequential event reported gap")
	}
	if !page.streamGap(runtimeevent.Event{RunID: "other", Sequence: 10}) {
		t.Fatal("sequence gap not detected")
	}

	page.connected = true
	cmd := page.finishStreamEvent(logsStreamEventMsg{generation: 2, err: io.EOF})
	if cmd == nil || page.connected || !page.reconnecting || !strings.Contains(page.notice, "disconnected") {
		t.Fatalf("disconnect connected=%t reconnect=%t notice=%q cmd=%v", page.connected, page.reconnecting, page.notice, cmd)
	}
	if cmd := page.finishStreamEvent(logsStreamEventMsg{generation: 1, event: runtimeevent.Event{RunID: "run", Sequence: 1}}); cmd != nil {
		t.Fatalf("stale event scheduled cmd=%v", cmd)
	}
}

func TestLogsCommandExecTabStreamsCombinedOutputInEventOrder(t *testing.T) {
	root := setupLogsPageRoot(t)
	code := 0
	info := shellruntime.ExecutionInfo{ID: "exec_test", WorkspaceID: "ws_a", Tool: "run_command", Command: "printf demo", CWD: "/tmp", Source: "mcp", StartedAt: time.Now().UTC().Format(time.RFC3339Nano), Status: shellruntime.ExecutionStatusRunning}
	snapshot := shellruntime.ExecutionFeedSnapshot{Events: []shellruntime.ExecutionFeedEvent{
		{Sequence: 1, Type: shellruntime.ExecutionEventStarted, ExecutionID: info.ID, WorkspaceID: info.WorkspaceID, Execution: &info, Status: shellruntime.ExecutionStatusRunning},
		{Sequence: 2, Type: shellruntime.ExecutionEventOutput, ExecutionID: info.ID, WorkspaceID: info.WorkspaceID, Stream: "stdout", Data: "out\n"},
	}, LatestSequence: 2}
	ready, _ := json.Marshal(snapshot)
	live := shellruntime.ExecutionFeedEvent{Sequence: 3, Type: shellruntime.ExecutionEventOutput, ExecutionID: info.ID, WorkspaceID: info.WorkspaceID, Stream: "stderr", Data: "err\n"}
	liveData, _ := json.Marshal(live)
	completed := shellruntime.ExecutionFeedEvent{Sequence: 4, Type: shellruntime.ExecutionEventCompleted, ExecutionID: info.ID, WorkspaceID: info.WorkspaceID, Status: shellruntime.ExecutionStatusSuccess, ExitCode: &code}
	completedData, _ := json.Marshal(completed)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/executions/stream" || r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("request=%s auth=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, "event: ready\ndata: %s\n\nid: 3\nevent: output\ndata: %s\n\nid: 4\nevent: completed\ndata: %s\n\n", ready, liveData, completedData)
	}))
	defer server.Close()
	writeLogsRuntimeState(t, root, server.URL, "run_exec")
	page, _ := NewLogs(t.Context())
	defer page.Close()
	updated, open := page.Update(tea.KeyPressMsg{Code: '2'})
	page = updated.(*LogsPage)
	if page.tab != logsTabCommandExec || open == nil {
		t.Fatalf("tab=%d open=%v", page.tab, open)
	}
	updated, next := page.Update(open())
	page = updated.(*LogsPage)
	if !page.exec.connected || len(page.exec.events) != 2 || next == nil {
		t.Fatalf("connected=%t events=%#v next=%v", page.exec.connected, page.exec.events, next)
	}
	updated, next = page.Update(next())
	page = updated.(*LogsPage)
	if len(page.exec.events) != 3 || next == nil {
		t.Fatalf("live events=%#v next=%v", page.exec.events, next)
	}
	updated, next = page.Update(next())
	page = updated.(*LogsPage)
	if len(page.exec.events) != 4 || next == nil {
		t.Fatalf("completed events=%#v next=%v", page.exec.events, next)
	}
	plain := ansi.Strip(page.View(120, 28))
	for _, want := range []string{"Runtime   Command Execution", "Mode  combined", "exec_id=exec_test", "$ printf demo", "workspace: ws_a", "out", "err", "[success, exit 0]"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("command exec view missing %q: %q", want, plain)
		}
	}
	if strings.Contains(plain, "stdout:") || strings.Contains(plain, "stderr:") {
		t.Fatalf("combined view split streams: %q", plain)
	}
}

func TestLogsCommandExecutionUnsupportedRuntimeStopsReconnectLoop(t *testing.T) {
	page, _ := NewLogs(t.Context())
	defer page.Close()
	page.exec.generation = 7
	page.exec.loading = true
	cmd := page.finishExecutionFeedOpen(logsExecutionOpenMsg{generation: 7, err: runtimecontrol.ErrExecutionFeedUnsupported})
	if cmd != nil || page.exec.connected || page.exec.reconnecting || !page.exec.unsupported || !page.exec.loaded {
		t.Fatalf("cmd=%v connected=%t reconnecting=%t unsupported=%t loaded=%t", cmd, page.exec.connected, page.exec.reconnecting, page.exec.unsupported, page.exec.loaded)
	}
	view := ansi.Strip(page.executionStatusView(100))
	if !strings.Contains(view, "RESTART REQUIRED") || !strings.Contains(page.exec.notice, "Restart the running server") {
		t.Fatalf("status=%q notice=%q", view, page.exec.notice)
	}
}

func TestLogsCommandExecutionEmptyViewPinsHelpToBottom(t *testing.T) {
	page, _ := NewLogs(t.Context())
	defer page.Close()
	page.tab = logsTabCommandExec
	page.exec.connected, page.exec.loaded = true, true
	plain := ansi.Strip(page.View(120, 28))
	lines := strings.Split(plain, "\n")
	last := len(lines) - 1
	for last >= 0 && strings.TrimSpace(lines[last]) == "" {
		last--
	}
	if last < 0 || !strings.Contains(lines[last], "reconnect") || !strings.Contains(lines[last], "clear view") {
		t.Fatalf("bottom help not pinned: last=%d line=%q view=%q", last, lines[last], plain)
	}
	waiting := -1
	for index, line := range lines {
		if strings.Contains(line, "Waiting for command output") {
			waiting = index
			break
		}
	}
	if waiting < 0 || last-waiting < 10 {
		t.Fatalf("empty body did not reserve vertical space: waiting=%d help=%d", waiting, last)
	}
}

func TestFormatExecutionFeedCombinesStdoutAndStderrWithoutStreamSections(t *testing.T) {
	code := 7
	info := shellruntime.ExecutionInfo{ID: "exec_order", WorkspaceID: "ws_a", Command: "demo", CWD: "/work"}
	events := []shellruntime.ExecutionFeedEvent{
		{Sequence: 1, Type: shellruntime.ExecutionEventStarted, ExecutionID: info.ID, Execution: &info},
		{Sequence: 2, Type: shellruntime.ExecutionEventOutput, ExecutionID: info.ID, Stream: "stdout", Data: "A"},
		{Sequence: 3, Type: shellruntime.ExecutionEventOutput, ExecutionID: info.ID, Stream: "stderr", Data: "B"},
		{Sequence: 4, Type: shellruntime.ExecutionEventOutput, ExecutionID: info.ID, Stream: "stdout", Data: "C\n"},
		{Sequence: 5, Type: shellruntime.ExecutionEventCompleted, ExecutionID: info.ID, Status: shellruntime.ExecutionStatusFailed, ExitCode: &code},
	}
	view := formatExecutionFeed(events)
	if !strings.Contains(view, "ABC\n[failed, exit 7]") || strings.Contains(view, "stdout") || strings.Contains(view, "stderr") {
		t.Fatalf("combined feed=%q", view)
	}
}

func TestLogsCommandExecTabNavigationAndMouseTargets(t *testing.T) {
	setupLogsPageRoot(t)
	page, _ := NewLogs(t.Context())
	defer page.Close()
	page.width, page.height = 100, 24
	if view := ansi.Strip(page.View(page.width, page.height)); !strings.Contains(view, "Runtime   Command Execution") {
		t.Fatalf("tabs view=%q", view)
	}
	targets := page.MouseTargets(0, 0, 1)
	var commandTab component.MouseTarget
	for _, target := range targets {
		if target.ID == "logs.tab" {
			if msg, ok := target.Handle(component.MouseEvent{Button: tea.MouseLeft}).(tea.KeyPressMsg); ok && msg.String() == "2" {
				commandTab = target
				break
			}
		}
	}
	if commandTab.Handle == nil {
		t.Fatal("command exec tab mouse target missing")
	}
	updated, _ := page.Update(commandTab.Handle(component.MouseEvent{Button: tea.MouseLeft}))
	page = updated.(*LogsPage)
	if page.tab != logsTabCommandExec {
		t.Fatalf("tab=%d", page.tab)
	}
	updated, _ = page.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	page = updated.(*LogsPage)
	if page.tab != logsTabRuntime {
		t.Fatalf("left did not return runtime tab: %d", page.tab)
	}
}

func TestLogsFormattingHelpersCoverBoundaries(t *testing.T) {
	if logEventID(runtimeevent.Event{Time: time.Unix(1, 2), Name: "event", Message: "message"}) == "" {
		t.Fatal("fallback event id empty")
	}
	if durationLabel(0) != "" || durationLabel(12) != "12ms" {
		t.Fatalf("duration labels=%q/%q", durationLabel(0), durationLabel(12))
	}
	if shortValue("abc", 0) != "" || shortValue("abc", 3) != "abc" || lipgloss.Width(shortValue("abcdef", 1)) > 1 {
		t.Fatalf("short values=%q/%q/%q", shortValue("abc", 0), shortValue("abc", 3), shortValue("abcdef", 1))
	}
	for value, want := range map[int64]string{10: "10 B", 2048: "2.0 KiB", 2 * 1024 * 1024: "2.0 MiB"} {
		if got := humanBytes(value); got != want {
			t.Fatalf("humanBytes(%d)=%q want %q", value, got, want)
		}
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

func TestLogsPageBootstrapLoadsJournalBeforeOpeningLiveHTTP(t *testing.T) {
	root := setupLogsPageRoot(t)
	appendLogEvents(t, root, runtimeevent.Event{Sequence: 1, Time: time.Now().UTC(), RunID: "run_fast", Level: "info", Name: "journal.ready", Message: "Journal ready"})
	requests := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- struct{}{}
		<-r.Context().Done()
	}))
	defer server.Close()
	writeLogsRuntimeState(t, root, server.URL, "run_fast")
	page, _ := NewLogs(t.Context())
	defer page.Close()
	bootstrap := page.Init()
	if bootstrap == nil {
		t.Fatal("bootstrap command missing")
	}
	msg := bootstrap()
	select {
	case <-requests:
		t.Fatal("bootstrap contacted live HTTP before journal load completed")
	default:
	}
	updated, connect := page.Update(msg)
	page = updated.(*LogsPage)
	if connect == nil || len(page.events) != 1 || page.events[0].Name != "journal.ready" || !page.loaded {
		t.Fatalf("journal bootstrap loaded=%t events=%#v connect=%v", page.loaded, page.events, connect)
	}
}
