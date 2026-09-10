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
	"go.mewis.me/chatgpt-mcp/internal/workspace"
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
	if !strings.Contains(page.notice, "Journal loaded") || page.ShouldToastNotice() {
		t.Fatalf("bootstrap notice=%q toast=%t", page.notice, page.ShouldToastNotice())
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

func TestLogsEventChildDetailStaysPinnedWhileLiveEventsAppend(t *testing.T) {
	page, _ := NewLogsRoute(t.Context(), "run:2", "")
	defer page.Close()
	base := time.Now().UTC()
	page.mergeEvents([]runtimeevent.Event{
		{Sequence: 1, Time: base, RunID: "run", Level: "info", Name: "one", Message: "one"},
		{Sequence: 2, Time: base.Add(time.Second), RunID: "run", Level: "info", Name: "two", Message: "two"},
	})
	if page.OverlayActive() || page.paused {
		t.Fatalf("overlay=%t paused=%t", page.OverlayActive(), page.paused)
	}
	page.appendEvent(runtimeevent.Event{Sequence: 3, Time: base.Add(2 * time.Second), RunID: "run", Level: "info", Name: "three", Message: "three"})
	if page.paused {
		t.Fatalf("detail child unexpectedly paused follow state")
	}
	detail := ansi.Strip(page.View(100, 26))
	if !strings.Contains(detail, "Log event · two") || strings.Contains(detail, "Log event · three") {
		t.Fatalf("detail jumped after live append: %q", detail)
	}
	_, cmd := page.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	if cmd == nil {
		t.Fatal("fields child navigation returned no command")
	}
	navigate, ok := cmd().(NavigateMsg)
	if !ok || strings.Join(navigate.Path, "/") != "logs/run:2/fields" {
		t.Fatalf("fields navigation=%#v", navigate)
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
	page.resourceID, page.section, page.events = "run:1", "fields", []runtimeevent.Event{event}
	page.syncDetail()
	joined += "\n" + ansi.Strip(page.detail.View())
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
		runtimeevent.Event{Sequence: 4, Time: base.Add(3 * time.Millisecond), RunID: "run_visibility", Level: "info", Kind: "info", Name: "tool.call.started", Component: "TOOL", Message: "Tool call started", Tool: "run_command", Status: "running", Visibility: logger.VisibilityVerbose},
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
	want := []string{"approval.requested", "tunnel.connected", "tool.call.completed", "tool.call.started"}
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
	editor, data := newLogsFilterEditor(application.LogsQueryOptions{Tail: 100}, logger.VisibilityVerbose)
	if editor.ActiveSectionID() != "range" {
		t.Fatalf("active section=%q", editor.ActiveSectionID())
	}
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
	if !page.connected || page.generation != 7 || len(page.events) != 0 || page.notice != "Runtime logs cleared" || !page.ShouldToastNotice() {
		t.Fatalf("clear state connected=%t generation=%d events=%d notice=%q toast=%t", page.connected, page.generation, len(page.events), page.notice, page.ShouldToastNotice())
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

	updated, filterCmd := page.Update(LogsCommandMsg{Command: LogsFilter})
	page = updated.(*LogsPage)
	if filterCmd == nil || page.editor == nil || page.OverlayActive() || !page.InputActive() {
		t.Fatalf("filter editor=%v active=%t input=%t", page.editor != nil, page.OverlayActive(), page.InputActive())
	}
	updated, cancelCmd := page.Update(component.EditorCancelMsg{})
	page = updated.(*LogsPage)
	if cancelCmd == nil || page.editor != nil || page.OverlayActive() || page.InputActive() {
		t.Fatalf("cancel editor=%v active=%t input=%t", page.editor != nil, page.OverlayActive(), page.InputActive())
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
	page.initFilterEditor()
	page.filterForm.Tail = "25"
	page.filterForm.Visibility = "debug"
	page.filterForm.Level = "warn"
	page.filterForm.Event = "tool.*"
	updated, bootstrap := page.Update(component.EditorSubmitMsg{})
	page = updated.(*LogsPage)
	if bootstrap == nil || page.editor != nil || page.options.Tail != 25 || page.options.Level != "warn" || page.options.Event != "tool.*" || page.visibility != logger.VisibilityDebug {
		t.Fatalf("options=%#v visibility=%d editor=%v cmd=%v", page.options, page.visibility, page.editor != nil, bootstrap)
	}

	page.initFilterEditor()
	page.filterForm.All, page.filterForm.Session = true, "run_stale"
	updated, bootstrap = page.Update(component.EditorSubmitMsg{})
	page = updated.(*LogsPage)
	plain := ansi.Strip(page.View(44, 18))
	if bootstrap != nil || page.editor == nil || page.filterForm.Session != "run_stale" || page.err != nil || !strings.Contains(plain, "all sessions and session filter") {
		t.Fatalf("invalid filter editor=%v pageErr=%v cmd=%v view=%q", page.editor != nil, page.err, bootstrap, plain)
	}
}

func TestLogsFilterDeepLinkUsesNativeWrappedEditor(t *testing.T) {
	page, err := NewLogsRouteAction(t.Context(), "", "", "filter")
	if err != nil {
		t.Fatal(err)
	}
	defer page.Close()
	if page.editor == nil || page.OverlayActive() || !page.InputActive() {
		t.Fatalf("editor=%v overlay=%t input=%t", page.editor != nil, page.OverlayActive(), page.InputActive())
	}
	view := page.View(28, 18)
	for _, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(line); got > 28 {
			t.Fatalf("line width=%d want <=28: %q", got, ansi.Strip(line))
		}
	}
	plain := ansi.Strip(view)
	for _, want := range []string{"Log Filters", "Range", "Filters"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("filter editor missing %q: %q", want, plain)
		}
	}
	if strings.Contains(plain, "ctrl+s apply") {
		t.Fatalf("non-mutating filter editor still advertises ctrl+s: %q", plain)
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
	if page.ShouldToastNotice() {
		t.Fatal("stream disconnect status unexpectedly marked as toast")
	}
	if cmd := page.finishStreamEvent(logsStreamEventMsg{generation: 1, event: runtimeevent.Event{RunID: "run", Sequence: 1}}); cmd != nil {
		t.Fatalf("stale event scheduled cmd=%v", cmd)
	}
}

func TestLogsCommandExecutionRouteStreamsCombinedOutputInEventOrder(t *testing.T) {
	root := setupLogsPageRoot(t)
	code := 0
	started := time.Now().UTC()
	info := shellruntime.ExecutionInfo{ID: "exec_test", WorkspaceID: "ws_a", Tool: "run_command", Command: "printf demo", CWD: "/tmp", Source: "mcp", CallID: "call_test", SessionHash: "session-test", ReceivedByInstanceID: "instance-a", ExecutedByInstanceID: "instance-b", StartedAt: started.Format(time.RFC3339Nano), Status: shellruntime.ExecutionStatusRunning}
	snapshot := shellruntime.ExecutionFeedSnapshot{Events: []shellruntime.ExecutionFeedEvent{
		{Sequence: 1, Type: shellruntime.ExecutionEventStarted, ExecutionID: info.ID, WorkspaceID: info.WorkspaceID, Execution: &info, Status: shellruntime.ExecutionStatusRunning, Timestamp: started.Format(time.RFC3339Nano)},
		{Sequence: 2, Type: shellruntime.ExecutionEventOutput, ExecutionID: info.ID, WorkspaceID: info.WorkspaceID, Execution: &info, Stream: "stdout", Data: "out\n", Timestamp: started.Add(500 * time.Millisecond).Format(time.RFC3339Nano)},
	}, LatestSequence: 2}
	ready, _ := json.Marshal(snapshot)
	live := shellruntime.ExecutionFeedEvent{Sequence: 3, Type: shellruntime.ExecutionEventOutput, ExecutionID: info.ID, WorkspaceID: info.WorkspaceID, Execution: &info, Stream: "stderr", Data: "err\n", Timestamp: started.Add(time.Second).Format(time.RFC3339Nano)}
	liveData, _ := json.Marshal(live)
	finished := info
	finished.FinishedAt, finished.Status, finished.ExitCode = started.Add(2*time.Second).Format(time.RFC3339Nano), shellruntime.ExecutionStatusSuccess, &code
	completed := shellruntime.ExecutionFeedEvent{Sequence: 4, Type: shellruntime.ExecutionEventCompleted, ExecutionID: info.ID, WorkspaceID: info.WorkspaceID, Execution: &finished, Status: shellruntime.ExecutionStatusSuccess, ExitCode: &code, Timestamp: finished.FinishedAt}
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
	page, _ := NewCommandExecutionLogs(t.Context())
	defer page.Close()
	open := page.Init()
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
	for _, want := range []string{"Runtime", "Command Execution", "Mode  combined", "[START]", "exec_id=exec_test", "$ printf demo", "workspace: ws_a", "source: mcp", "session: session-test", "call: call_test", "route: received: instance-a  executed: instance-b", "out", "err", "[END]", "status: success  exit: 0  duration: 2s", "←/→ tabs"} {
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
	page, _ := NewCommandExecutionLogs(t.Context())
	defer page.Close()
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
	started := time.Now().UTC()
	info := shellruntime.ExecutionInfo{ID: "exec_order", WorkspaceID: "ws_a", Command: "demo", CWD: "/work", StartedAt: started.Format(time.RFC3339Nano)}
	finished := info
	finished.FinishedAt = started.Add(2 * time.Second).Format(time.RFC3339Nano)
	events := []shellruntime.ExecutionFeedEvent{
		{Sequence: 1, Type: shellruntime.ExecutionEventStarted, ExecutionID: info.ID, Execution: &info, Timestamp: started.Format(time.RFC3339Nano)},
		{Sequence: 2, Type: shellruntime.ExecutionEventOutput, ExecutionID: info.ID, Execution: &info, Stream: "stdout", Data: "A", Timestamp: started.Add(250 * time.Millisecond).Format(time.RFC3339Nano)},
		{Sequence: 3, Type: shellruntime.ExecutionEventOutput, ExecutionID: info.ID, Execution: &info, Stream: "stderr", Data: "B", Timestamp: started.Add(500 * time.Millisecond).Format(time.RFC3339Nano)},
		{Sequence: 4, Type: shellruntime.ExecutionEventOutput, ExecutionID: info.ID, Execution: &info, Stream: "stdout", Data: "C\n", Timestamp: started.Add(time.Second).Format(time.RFC3339Nano)},
		{Sequence: 5, Type: shellruntime.ExecutionEventCompleted, ExecutionID: info.ID, Execution: &finished, Status: shellruntime.ExecutionStatusFailed, ExitCode: &code, Timestamp: finished.FinishedAt},
	}
	view := formatExecutionFeed(events)
	if !strings.Contains(view, "ABC\n╭") || !strings.Contains(view, "[END]") || !strings.Contains(view, "status: failed  exit: 7  duration: 2s") || strings.Count(view, "╭") != 2 || strings.Count(view, "╯") != 2 || strings.Contains(view, "stdout") || strings.Contains(view, "stderr") {
		t.Fatalf("combined feed=%q", view)
	}
}

func TestFormatExecutionFeedMarksInterleavedContinuations(t *testing.T) {
	code := 0
	started := time.Now().UTC()
	infoA := shellruntime.ExecutionInfo{ID: "exec_a", WorkspaceID: "ws_a", Command: "first", StartedAt: started.Format(time.RFC3339Nano)}
	infoB := shellruntime.ExecutionInfo{ID: "exec_b", WorkspaceID: "ws_b", Command: "second", StartedAt: started.Add(100 * time.Millisecond).Format(time.RFC3339Nano)}
	finishedA, finishedB := infoA, infoB
	finishedA.FinishedAt = started.Add(900 * time.Millisecond).Format(time.RFC3339Nano)
	finishedB.FinishedAt = started.Add(700 * time.Millisecond).Format(time.RFC3339Nano)
	events := []shellruntime.ExecutionFeedEvent{
		{Sequence: 1, Type: shellruntime.ExecutionEventStarted, ExecutionID: infoA.ID, WorkspaceID: infoA.WorkspaceID, Execution: &infoA, Timestamp: infoA.StartedAt},
		{Sequence: 2, Type: shellruntime.ExecutionEventOutput, ExecutionID: infoA.ID, WorkspaceID: infoA.WorkspaceID, Execution: &infoA, Data: "A1\n", Timestamp: started.Add(200 * time.Millisecond).Format(time.RFC3339Nano)},
		{Sequence: 3, Type: shellruntime.ExecutionEventStarted, ExecutionID: infoB.ID, WorkspaceID: infoB.WorkspaceID, Execution: &infoB, Timestamp: infoB.StartedAt},
		{Sequence: 4, Type: shellruntime.ExecutionEventOutput, ExecutionID: infoB.ID, WorkspaceID: infoB.WorkspaceID, Execution: &infoB, Data: "B1\n", Timestamp: started.Add(300 * time.Millisecond).Format(time.RFC3339Nano)},
		{Sequence: 5, Type: shellruntime.ExecutionEventOutput, ExecutionID: infoA.ID, WorkspaceID: infoA.WorkspaceID, Execution: &infoA, Data: "A2\n", Timestamp: started.Add(500 * time.Millisecond).Format(time.RFC3339Nano)},
		{Sequence: 6, Type: shellruntime.ExecutionEventOutput, ExecutionID: infoB.ID, WorkspaceID: infoB.WorkspaceID, Execution: &infoB, Data: "B2\n", Timestamp: started.Add(600 * time.Millisecond).Format(time.RFC3339Nano)},
		{Sequence: 7, Type: shellruntime.ExecutionEventCompleted, ExecutionID: infoB.ID, WorkspaceID: infoB.WorkspaceID, Execution: &finishedB, Status: shellruntime.ExecutionStatusSuccess, ExitCode: &code, Timestamp: finishedB.FinishedAt},
		{Sequence: 8, Type: shellruntime.ExecutionEventCompleted, ExecutionID: infoA.ID, WorkspaceID: infoA.WorkspaceID, Execution: &finishedA, Status: shellruntime.ExecutionStatusSuccess, ExitCode: &code, Timestamp: finishedA.FinishedAt},
	}
	view := formatExecutionFeed(events)
	if strings.Count(view, "[START]") != 2 || strings.Count(view, "[CONTINUE]") != 3 || strings.Count(view, "[END]") != 2 {
		t.Fatalf("interleaved markers=%q", view)
	}
	for _, want := range []string{"[CONTINUE]", "exec_id=exec_a  +500ms", "exec_id=exec_b  +500ms", "A1", "B1", "A2", "B2"} {
		if !strings.Contains(view, want) {
			t.Fatalf("interleaved feed missing %q: %q", want, view)
		}
	}
	if !(strings.Index(view, "A1") < strings.Index(view, "B1") && strings.Index(view, "B1") < strings.Index(view, "A2") && strings.Index(view, "A2") < strings.Index(view, "B2")) {
		t.Fatalf("interleaved output order changed: %q", view)
	}
}

func TestFormatExecutionFeedSeparatesConcurrentExecutionsInSameWorkspace(t *testing.T) {
	started := time.Now().UTC()
	infoA := shellruntime.ExecutionInfo{ID: "exec_a", WorkspaceID: "ws_same", StartedAt: started.Format(time.RFC3339Nano)}
	infoB := shellruntime.ExecutionInfo{ID: "exec_b", WorkspaceID: "ws_same", StartedAt: started.Add(time.Millisecond).Format(time.RFC3339Nano)}
	view := formatExecutionFeed([]shellruntime.ExecutionFeedEvent{
		{Sequence: 1, Type: shellruntime.ExecutionEventStarted, ExecutionID: infoA.ID, WorkspaceID: infoA.WorkspaceID, Execution: &infoA, Timestamp: infoA.StartedAt},
		{Sequence: 2, Type: shellruntime.ExecutionEventOutput, ExecutionID: infoA.ID, WorkspaceID: infoA.WorkspaceID, Execution: &infoA, Data: "A\n", Timestamp: started.Add(2 * time.Millisecond).Format(time.RFC3339Nano)},
		{Sequence: 3, Type: shellruntime.ExecutionEventStarted, ExecutionID: infoB.ID, WorkspaceID: infoB.WorkspaceID, Execution: &infoB, Timestamp: infoB.StartedAt},
		{Sequence: 4, Type: shellruntime.ExecutionEventOutput, ExecutionID: infoB.ID, WorkspaceID: infoB.WorkspaceID, Execution: &infoB, Data: "B\n", Timestamp: started.Add(3 * time.Millisecond).Format(time.RFC3339Nano)},
		{Sequence: 5, Type: shellruntime.ExecutionEventOutput, ExecutionID: infoA.ID, WorkspaceID: infoA.WorkspaceID, Execution: &infoA, Data: "A2\n", Timestamp: started.Add(4 * time.Millisecond).Format(time.RFC3339Nano)},
	})
	if strings.Count(view, "[START]") != 2 || strings.Count(view, "[CONTINUE]") != 1 || !strings.Contains(view, "[CONTINUE]") || !strings.Contains(view, "exec_id=exec_a") {
		t.Fatalf("same-workspace interleave=%q", view)
	}
}

func TestExecutionScopeFiltersCombinedWorkspaceAndContainer(t *testing.T) {
	page, _ := NewCommandExecutionLogs(t.Context())
	defer page.Close()
	page.exec.events = []shellruntime.ExecutionFeedEvent{
		{Sequence: 1, ExecutionID: "exec_a", WorkspaceID: "ws_a", Type: shellruntime.ExecutionEventOutput, Data: "A\n"},
		{Sequence: 2, ExecutionID: "exec_b", WorkspaceID: "ws_b", Type: shellruntime.ExecutionEventOutput, Data: "B\n"},
		{Sequence: 3, ExecutionID: "exec_c", WorkspaceID: "ws_c", Type: shellruntime.ExecutionEventOutput, Data: "C\n"},
	}
	if got := len(page.visibleExecutionEvents()); got != 3 {
		t.Fatalf("combined events=%d", got)
	}
	page.exec.scopeMode, page.exec.workspaceID = executionScopeWorkspace, "ws_b"
	if visible := page.visibleExecutionEvents(); len(visible) != 1 || visible[0].WorkspaceID != "ws_b" {
		t.Fatalf("workspace events=%#v", visible)
	}
	page.exec.scopeMode, page.exec.containerMembers = executionScopeContainer, map[string]struct{}{"ws_a": {}, "ws_c": {}}
	if visible := page.visibleExecutionEvents(); len(visible) != 2 || visible[0].WorkspaceID != "ws_a" || visible[1].WorkspaceID != "ws_c" {
		t.Fatalf("container events=%#v", visible)
	}
}

func TestExecutionScopeEditorAppliesWithoutReconnectingGlobalFeed(t *testing.T) {
	setupLogsPageRoot(t)
	manager := workspace.NewManager(workspace.DefaultStorePath())
	first, err := manager.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	container, err := manager.CreateContainer("project")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.AddWorkspacesToContainer(container.ID, []string{first.ID, second.ID}); err != nil {
		t.Fatal(err)
	}
	page, _ := NewCommandExecutionLogs(t.Context())
	defer page.Close()
	page.exec.generation = 9
	if cmd := page.openExecutionScopeEditor(); cmd == nil || page.exec.scopeEditor == nil || page.exec.scopeForm == nil {
		t.Fatalf("scope editor missing: cmd=%v editor=%v form=%v", cmd, page.exec.scopeEditor != nil, page.exec.scopeForm != nil)
	}
	page.exec.scopeForm.Mode, page.exec.scopeForm.ContainerID = string(executionScopeContainer), container.ID
	page.submitExecutionScopeEditor()
	if page.exec.scopeEditor != nil || page.exec.scopeMode != executionScopeContainer || page.exec.containerID != container.ID || page.exec.containerName != "project" || len(page.exec.containerMembers) != 2 || page.exec.generation != 9 {
		t.Fatalf("scope applied=%#v", page.exec)
	}
	if label := page.executionScopeLabel(); !strings.Contains(label, "container · project · 2 workspaces") {
		t.Fatalf("scope label=%q", label)
	}
}

func TestExecutionScopeEditorCompletesWithEnterForDynamicScopes(t *testing.T) {
	setupLogsPageRoot(t)
	manager := workspace.NewManager(workspace.DefaultStorePath())
	workspaceItem, err := manager.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	container, err := manager.CreateContainer("project")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.AddWorkspaceToContainer(container.ID, workspaceItem.ID); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name      string
		down      int
		wantMode  executionScopeMode
		wantID    string
		needsNext bool
	}{
		{name: "combined", wantMode: executionScopeCombined},
		{name: "workspace", down: 1, wantMode: executionScopeWorkspace, wantID: workspaceItem.ID, needsNext: true},
		{name: "container", down: 2, wantMode: executionScopeContainer, wantID: container.ID, needsNext: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			page, _ := NewCommandExecutionLogs(t.Context())
			defer page.Close()
			page.exec.generation = 11
			page = runLogsPageCmd(t, page, page.openExecutionScopeEditor())
			for range test.down {
				updated, cmd := page.Update(tea.KeyPressMsg{Code: tea.KeyDown})
				page = runLogsPageCmd(t, updated.(*LogsPage), cmd)
			}
			updated, cmd := page.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			page = runLogsPageCmd(t, updated.(*LogsPage), cmd)
			if test.needsNext {
				if page.exec.scopeEditor == nil {
					t.Fatal("mode selection submitted before scope selector")
				}
				updated, cmd = page.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
				page = runLogsPageCmd(t, updated.(*LogsPage), cmd)
			}
			if page.exec.scopeEditor != nil || page.exec.scopeMode != test.wantMode || page.exec.generation != 11 {
				t.Fatalf("scope editor=%v mode=%s generation=%d", page.exec.scopeEditor != nil, page.exec.scopeMode, page.exec.generation)
			}
			if test.wantMode == executionScopeWorkspace && page.exec.workspaceID != test.wantID {
				t.Fatalf("workspace=%q want=%q", page.exec.workspaceID, test.wantID)
			}
			if test.wantMode == executionScopeContainer && page.exec.containerID != test.wantID {
				t.Fatalf("container=%q want=%q", page.exec.containerID, test.wantID)
			}
		})
	}
}

func TestExecutionScopeRefreshTracksMembershipAndStaleContainer(t *testing.T) {
	setupLogsPageRoot(t)
	manager := workspace.NewManager(workspace.DefaultStorePath())
	first, _ := manager.Register(t.TempDir())
	second, _ := manager.Register(t.TempDir())
	container, _ := manager.CreateContainer("project")
	_, _ = manager.AddWorkspaceToContainer(container.ID, first.ID)
	page, _ := NewCommandExecutionLogs(t.Context())
	defer page.Close()
	page.exec.scopeMode, page.exec.containerID = executionScopeContainer, container.ID
	page.refreshExecutionScope()
	if len(page.exec.containerMembers) != 1 {
		t.Fatalf("initial members=%#v", page.exec.containerMembers)
	}
	writer := workspace.NewManager(workspace.DefaultStorePath())
	if _, err := writer.AddWorkspaceToContainer(container.ID, second.ID); err != nil {
		t.Fatal(err)
	}
	page.refreshExecutionScope()
	if len(page.exec.containerMembers) != 2 {
		t.Fatalf("refreshed members=%#v", page.exec.containerMembers)
	}
	if err := writer.DeleteContainer(container.ID); err != nil {
		t.Fatal(err)
	}
	page.refreshExecutionScope()
	if !page.exec.scopeStale || page.exec.scopeNotice == "" || len(page.visibleExecutionEvents()) != 0 {
		t.Fatalf("stale scope stale=%t notice=%q visible=%#v", page.exec.scopeStale, page.exec.scopeNotice, page.visibleExecutionEvents())
	}
}

func TestExecutionScopeFilteringDoesNotCreateSequenceGaps(t *testing.T) {
	page, _ := NewCommandExecutionLogs(t.Context())
	defer page.Close()
	page.exec.generation = 4
	page.exec.scopeMode, page.exec.workspaceID = executionScopeWorkspace, "ws_selected"
	page.finishExecutionFeedEvent(logsExecutionEventMsg{generation: 4, event: shellruntime.ExecutionFeedEvent{Sequence: 1, ExecutionID: "visible", WorkspaceID: "ws_selected", Type: shellruntime.ExecutionEventStarted}})
	page.finishExecutionFeedEvent(logsExecutionEventMsg{generation: 4, event: shellruntime.ExecutionFeedEvent{Sequence: 2, ExecutionID: "hidden", WorkspaceID: "ws_other", Type: shellruntime.ExecutionEventOutput, Data: "hidden\n"}})
	page.finishExecutionFeedEvent(logsExecutionEventMsg{generation: 4, event: shellruntime.ExecutionFeedEvent{Sequence: 3, ExecutionID: "visible", WorkspaceID: "ws_selected", Type: shellruntime.ExecutionEventOutput, Data: "visible\n"}})
	visible := page.visibleExecutionEvents()
	if page.exec.latestSeq != 3 || len(page.exec.events) != 3 || len(visible) != 2 || page.exec.generation != 4 || strings.Contains(page.exec.notice, "gap") || strings.Contains(formatExecutionFeed(visible), "[CONTINUE]") {
		t.Fatalf("filtered sequence=%d raw=%d visible=%d generation=%d notice=%q", page.exec.latestSeq, len(page.exec.events), len(page.visibleExecutionEvents()), page.exec.generation, page.exec.notice)
	}
	page.exec.scopeMode, page.exec.containerMembers = executionScopeContainer, map[string]struct{}{"ws_selected": {}}
	visible = page.visibleExecutionEvents()
	if len(visible) != 2 || strings.Contains(formatExecutionFeed(visible), "[CONTINUE]") {
		t.Fatalf("container-filtered events=%#v view=%q", visible, formatExecutionFeed(visible))
	}
}

func TestExecutionScopeSurvivesSnapshotReplayAndOverflow(t *testing.T) {
	root := setupLogsPageRoot(t)
	manager := workspace.NewManager(workspace.DefaultStorePath())
	selected, err := manager.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	other, err := manager.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	snapshot := shellruntime.ExecutionFeedSnapshot{Events: []shellruntime.ExecutionFeedEvent{
		{Sequence: 1, ExecutionID: "selected", WorkspaceID: selected.ID, Type: shellruntime.ExecutionEventOutput, Data: "selected\n"},
		{Sequence: 2, ExecutionID: "other", WorkspaceID: other.ID, Type: shellruntime.ExecutionEventOutput, Data: "other\n"},
	}, LatestSequence: 2}
	ready, _ := json.Marshal(snapshot)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, "event: ready\ndata: %s\n\n", ready)
	}))
	defer server.Close()
	writeLogsRuntimeState(t, root, server.URL, "run_scope")
	page, _ := NewCommandExecutionLogs(t.Context())
	defer page.Close()
	page.exec.scopeMode, page.exec.workspaceID = executionScopeWorkspace, selected.ID
	open := page.Init()
	updated, next := page.Update(open())
	page = updated.(*LogsPage)
	if !page.exec.connected || page.exec.scopeMode != executionScopeWorkspace || page.exec.workspaceID != selected.ID || page.exec.latestSeq != 2 || len(page.exec.events) != 2 || len(page.visibleExecutionEvents()) != 1 || next == nil {
		t.Fatalf("snapshot scope=%s workspace=%s seq=%d raw=%d visible=%d connected=%t", page.exec.scopeMode, page.exec.workspaceID, page.exec.latestSeq, len(page.exec.events), len(page.visibleExecutionEvents()), page.exec.connected)
	}
	generation := page.exec.generation
	reconnect := page.finishExecutionFeedEvent(logsExecutionEventMsg{generation: generation, err: runtimecontrol.ErrExecutionFeedOverflow})
	if reconnect == nil || page.exec.scopeMode != executionScopeWorkspace || page.exec.workspaceID != selected.ID || !page.exec.reconnecting {
		t.Fatalf("overflow reset scope: mode=%s workspace=%s reconnect=%t cmd=%v", page.exec.scopeMode, page.exec.workspaceID, page.exec.reconnecting, reconnect)
	}
}

func TestExecutionScopeEditorWrapsAtNarrowWidths(t *testing.T) {
	setupLogsPageRoot(t)
	manager := workspace.NewManager(workspace.DefaultStorePath())
	if _, err := manager.Register(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	page, _ := NewCommandExecutionLogs(t.Context())
	defer page.Close()
	if cmd := page.openExecutionScopeEditor(); cmd == nil {
		t.Fatal("scope editor command missing")
	}
	for _, width := range []int{120, 80, 24} {
		view := page.executionScopeEditorView(width, 24)
		for _, line := range strings.Split(view, "\n") {
			if got := lipgloss.Width(line); got > width {
				t.Fatalf("width=%d line=%d: %q", width, got, ansi.Strip(line))
			}
		}
	}
}

func TestLogsRuntimeAndCommandExecutionRemainTabbedParentViews(t *testing.T) {
	setupLogsPageRoot(t)
	page, _ := NewLogs(t.Context())
	defer page.Close()
	page.width, page.height = 100, 24
	if view := ansi.Strip(page.View(page.width, page.height)); !strings.Contains(view, "Runtime") || !strings.Contains(view, "Command Execution") {
		t.Fatalf("runtime logs tab view=%q", view)
	}
	updated, cmd := page.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	page = updated.(*LogsPage)
	if page.tab != logsTabCommandExec || cmd == nil {
		t.Fatalf("execution tab=%d cmd=%v", page.tab, cmd)
	}
	if view := ansi.Strip(page.View(page.width, page.height)); !strings.Contains(view, "Runtime") || !strings.Contains(view, "Command Execution") || !strings.Contains(view, "Waiting for command output") {
		t.Fatalf("execution tab view=%q", view)
	}
	foundTabTarget := false
	for _, target := range page.MouseTargets(0, 0, 1) {
		if target.ID == "logs.tab" {
			foundTabTarget = true
			break
		}
	}
	if !foundTabTarget {
		t.Fatal("logs tab mouse target missing")
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

func TestCommandExecutionViewportReflowsLongReadableContent(t *testing.T) {
	page, err := NewCommandExecutionLogs(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer page.Close()
	commandToken, outputToken := strings.Repeat("c", 64), strings.Repeat("o", 80)
	info := shellruntime.ExecutionInfo{ID: "exec_long", WorkspaceID: "ws_long", Tool: "run_command", Command: "printf " + commandToken, CWD: "/very/long/workspace/" + commandToken}
	page.exec.events = []shellruntime.ExecutionFeedEvent{
		{Sequence: 1, Type: shellruntime.ExecutionEventStarted, ExecutionID: info.ID, WorkspaceID: info.WorkspaceID, Execution: &info},
		{Sequence: 2, Type: shellruntime.ExecutionEventOutput, ExecutionID: info.ID, WorkspaceID: info.WorkspaceID, Stream: "stdout", Data: outputToken},
	}
	page.exec.paused = true
	for _, width := range []int{24, 11} {
		page.resizeExecutionViewport(width, 8)
		content := page.exec.viewport.GetContent()
		for _, line := range strings.Split(content, "\n") {
			if got := lipgloss.Width(line); got > width {
				t.Fatalf("width=%d line=%d: %q", width, got, ansi.Strip(line))
			}
		}
		flat := strings.ReplaceAll(ansi.Strip(content), "\n", "")
		if !strings.Contains(flat, commandToken) || !strings.Contains(flat, outputToken) {
			t.Fatalf("width=%d content was truncated: %q", width, flat)
		}
	}
}

func TestConfirmOverlayBodyWrapsLongDescription(t *testing.T) {
	confirm := component.NewConfirmButtons("Continue", "Cancel", true)
	description := "Remove " + strings.Repeat("nested/", 12) + "workspace"
	width := 44
	body := confirmOverlayBody(confirm, "Confirm operation", description, width)
	for _, line := range strings.Split(body, "\n") {
		if got := lipgloss.Width(line); got > component.ModalContentWidth(width) {
			t.Fatalf("confirm body line width=%d want <=%d: %q", got, component.ModalContentWidth(width), ansi.Strip(line))
		}
	}
	flat := strings.ReplaceAll(strings.ReplaceAll(ansi.Strip(body), "\n", ""), " ", "")
	if !strings.Contains(flat, strings.ReplaceAll(description, " ", "")) {
		t.Fatalf("confirm description changed: %q", ansi.Strip(body))
	}
}

func runLogsPageCmd(t *testing.T, page *LogsPage, cmd tea.Cmd) *LogsPage {
	t.Helper()
	queue := []tea.Cmd{cmd}
	for steps := 0; steps < 64 && len(queue) > 0; steps++ {
		next := queue[0]
		queue = queue[1:]
		if next == nil {
			continue
		}
		message := next()
		if batch, ok := message.(tea.BatchMsg); ok {
			queue = append(queue, batch...)
			continue
		}
		updated, followup := page.Update(message)
		page = updated.(*LogsPage)
		if followup != nil {
			queue = append(queue, followup)
		}
	}
	return page
}
