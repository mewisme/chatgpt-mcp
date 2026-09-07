package page

import (
	"encoding/json"
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
	"go.mewis.me/chatgpt-mcp/internal/approval"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/runtimecontrol"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
)

func TestRequestsPageRefreshModesAndDeepLink(t *testing.T) {
	now := time.Now().UTC()
	pending := approval.Request{ID: "req_pending_full", Status: approval.StatusPending, WorkspaceID: "ws_a", Source: "tunnel", TargetTool: "run_command", Title: "Allow update", Arguments: []byte(`{"workspace_id":"ws_a","command":"cgm update"}`), GuardReason: "guarded", CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
	approved := approval.Request{ID: "req_approved_full", Status: approval.StatusApproved, WorkspaceID: "ws_b", Source: "tunnel", TargetTool: "run_command", Title: "Allow install", Arguments: []byte(`{"workspace_id":"ws_b","command":"cgm install"}`), CreatedAt: now.Add(-time.Minute), ExpiresAt: now, ResolvedAt: now, RetryUntil: now.Add(time.Minute)}
	server := newRequestPageServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/requests":
			_ = json.NewEncoder(w).Encode([]approval.Request{pending, approved})
		case "/requests/view":
			switch r.URL.Query().Get("id") {
			case "req_pending", pending.ID:
				_ = json.NewEncoder(w).Encode(pending)
			case approved.ID:
				_ = json.NewEncoder(w).Encode(approved)
			default:
				t.Fatalf("view id=%q", r.URL.Query().Get("id"))
			}
		default:
			t.Fatalf("unexpected path=%s", r.URL.Path)
		}
	})
	defer server.Close()

	page, err := NewRequests(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	if cmd := page.Init(); cmd == nil || !page.loading {
		t.Fatalf("init cmd=%v loading=%t", cmd, page.loading)
	}
	updated, _ := page.Update(page.refreshCmd()())
	page = updated.(*RequestsPage)
	if len(page.requests) != 2 || page.mode != requestModePending {
		t.Fatalf("requests=%#v mode=%d", page.requests, page.mode)
	}
	if selected, ok := page.browser.Selected(); !ok || selected.ID != pending.ID {
		t.Fatalf("pending selected=%#v ok=%t", selected, ok)
	}
	page.setMode(requestModeHistory)
	if selected, ok := page.browser.Selected(); !ok || selected.ID != approved.ID {
		t.Fatalf("history selected=%#v ok=%t", selected, ok)
	}
	if content := page.View(100, 28); !strings.Contains(content, "History") || strings.Contains(content, pending.Title) {
		t.Fatalf("history view=%q", content)
	}

	deep, err := NewRequests(t.Context(), "req_pending")
	if err != nil {
		t.Fatal(err)
	}
	updated, _ = deep.Update(deep.refreshCmd()())
	deep = updated.(*RequestsPage)
	if deep.resourceID != pending.ID || deep.OverlayActive() || deep.mode != requestModeAll {
		t.Fatalf("deep resource=%q overlay=%t mode=%d", deep.resourceID, deep.OverlayActive(), deep.mode)
	}
	view := ansi.Strip(deep.View(100, 28))
	for _, expected := range []string{"Approval request · " + pending.ID, pending.WorkspaceID, pending.TargetTool, "v arguments", "g guard", "a approve", "d deny"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("deep view missing %q: %q", expected, view)
		}
	}
	if strings.Contains(view, "Overview   Arguments") || strings.Contains(view, "╭") {
		t.Fatalf("deep view retained tab/modal chrome: %q", view)
	}
	updated, cmd := deep.Update(tea.KeyPressMsg{Code: 'v', Text: "v"})
	deep = updated.(*RequestsPage)
	if cmd == nil {
		t.Fatal("arguments child navigation returned no command")
	}
	navigate, ok := cmd().(NavigateMsg)
	if !ok || strings.Join(navigate.Path, "/") != "requests/"+pending.ID+"/arguments" {
		t.Fatalf("arguments navigation=%#v", navigate)
	}

	arguments, err := NewRequestsRoute(t.Context(), pending.ID, "arguments")
	if err != nil {
		t.Fatal(err)
	}
	updated, _ = arguments.Update(arguments.refreshCmd()())
	arguments = updated.(*RequestsPage)
	argumentsView := ansi.Strip(arguments.View(100, 28))
	if !strings.Contains(argumentsView, `"command": "cgm update"`) || strings.Contains(argumentsView, "v arguments") {
		t.Fatalf("arguments child=%q", argumentsView)
	}

	history, err := NewRequestsRoute(t.Context(), approved.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	updated, _ = history.Update(history.refreshCmd()())
	history = updated.(*RequestsPage)
	historyView := ansi.Strip(history.View(100, 28))
	if strings.Contains(historyView, "a approve") || strings.Contains(historyView, "d deny") {
		t.Fatalf("resolved detail exposed resolution actions: %q", historyView)
	}
}

func TestRequestMutationNoticeRendersBesidePageTitle(t *testing.T) {
	page, err := NewRequests(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	page.notice = "Request approved"
	line := strings.Split(ansi.Strip(page.View(100, 24)), "\n")[0]
	if !strings.Contains(line, "Approval requests · Pending  · Request approved") {
		t.Fatalf("request title notice=%q", line)
	}
}

func TestRequestsPagePreservesExpandedBrowserHelpAcrossRefresh(t *testing.T) {
	now := time.Now().UTC()
	request := approval.Request{ID: "req_pending", Status: approval.StatusPending, WorkspaceID: "ws_a", TargetTool: "run_command", Title: "Allow update", CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
	page, _ := NewRequests(t.Context(), "")
	page.requests = []approval.Request{request}
	page.rebuildBrowser(request.ID)
	updated, _ := page.browser.Update(tea.KeyPressMsg{Code: '?'})
	page.browser = updated.(component.Browser)
	if !page.browser.HelpExpanded() {
		t.Fatal("browser help did not expand")
	}
	page.rebuildBrowser(request.ID)
	if !page.browser.HelpExpanded() {
		t.Fatal("browser help collapsed after request refresh rebuild")
	}
}

func TestRequestsPageResolutionRequiresExplicitConfirmation(t *testing.T) {
	now := time.Now().UTC()
	request := approval.Request{ID: "req_pending", Status: approval.StatusPending, WorkspaceID: "ws_a", TargetTool: "run_command", Title: "Allow update", Arguments: []byte(`{"command":"cgm update"}`), CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
	resolveCalls := 0
	server := newRequestPageServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/requests":
			_ = json.NewEncoder(w).Encode([]approval.Request{request})
		case "/requests/approve":
			resolveCalls++
			var input map[string]string
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
				t.Fatal(err)
			}
			if input["id"] != request.ID || input["reason"] != "reviewed" {
				t.Fatalf("input=%#v", input)
			}
			resolved := request
			resolved.Status, resolved.Reason, resolved.ResolvedBy = approval.StatusApproved, input["reason"], "cli"
			resolved.ResolvedAt, resolved.RetryUntil = time.Now().UTC(), time.Now().UTC().Add(time.Minute)
			_ = json.NewEncoder(w).Encode(resolved)
		default:
			t.Fatalf("unexpected path=%s", r.URL.Path)
		}
	})
	defer server.Close()
	page, _ := NewRequests(t.Context(), "")
	updated, _ := page.Update(page.refreshCmd()())
	page = updated.(*RequestsPage)

	cmd := page.handleCommand(RequestApprove, request.ID)
	if cmd == nil || page.resolveForm == nil || page.resolveForm.Confirm || page.overlay != requestOverlayForm {
		t.Fatalf("form=%#v overlay=%d cmd=%v", page.resolveForm, page.overlay, cmd)
	}
	if resolve := page.submitResolveForm(); resolve != nil || resolveCalls != 0 || !strings.Contains(page.notice, "unchanged") {
		t.Fatalf("resolve=%v calls=%d notice=%q", resolve, resolveCalls, page.notice)
	}

	_ = page.handleCommand(RequestApprove, request.ID)
	page.resolveForm.Confirm = true
	page.resolveForm.Reason = "reviewed"
	resolve := page.submitResolveForm()
	if resolve == nil || page.overlay != requestOverlayOperation {
		t.Fatalf("resolve=%v overlay=%d", resolve, page.overlay)
	}
	updated, _ = page.Update(resolve())
	page = updated.(*RequestsPage)
	resolved, ok := page.findRequest(request.ID)
	if !ok || resolved.Status != approval.StatusApproved || resolveCalls != 1 || !strings.Contains(page.notice, "Approved") {
		t.Fatalf("resolved=%#v ok=%t calls=%d notice=%q", resolved, ok, resolveCalls, page.notice)
	}
	if cmd := page.handleCommand(RequestDeny, request.ID); cmd != nil || page.err == nil || !strings.Contains(page.err.Error(), "cannot be resolved") {
		t.Fatalf("resolved request deny cmd=%v err=%v", cmd, page.err)
	}
}

func TestRequestsPageResolveCancellationIgnoresLateResult(t *testing.T) {
	now := time.Now().UTC()
	request := approval.Request{ID: "req_pending", Status: approval.StatusPending, WorkspaceID: "ws_a", TargetTool: "run_command", Title: "Allow update", CreatedAt: now, ExpiresAt: now.Add(time.Minute)}
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	server := newRequestPageServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/requests/approve" {
			t.Fatalf("unexpected path=%s", r.URL.Path)
		}
		started <- struct{}{}
		<-release
		resolved := request
		resolved.Status = approval.StatusApproved
		_ = json.NewEncoder(w).Encode(resolved)
	})
	defer server.Close()
	page, _ := NewRequests(t.Context(), "")
	page.requests = []approval.Request{request}
	page.rebuildBrowser(request.ID)
	_ = page.handleCommand(RequestApprove, request.ID)
	page.resolveForm.Confirm = true
	resolve := page.submitResolveForm()
	result := make(chan tea.Msg, 1)
	go func() { result <- resolve() }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("approval request did not start")
	}
	updated, _ := page.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	page = updated.(*RequestsPage)
	if page.overlay != requestOverlayNone || !page.operationCancelled {
		t.Fatalf("overlay=%d cancelled=%t", page.overlay, page.operationCancelled)
	}
	select {
	case message := <-result:
		updated, _ = page.Update(message)
		page = updated.(*RequestsPage)
	case <-time.After(time.Second):
		close(release)
		t.Fatal("cancelled approval request did not return")
	}
	close(release)
	if page.operationCancelled || page.err != nil || !strings.Contains(page.notice, "cancelled") {
		t.Fatalf("cancelled=%t err=%v notice=%q", page.operationCancelled, page.err, page.notice)
	}
}

func TestRequestArgumentsRenderExactValues(t *testing.T) {
	request := approval.Request{Arguments: []byte(`{"command":"printf 'a:b'","workspace_id":"ws_a","nested":{"enabled":true}}`)}
	view := requestArguments(request)
	for _, expected := range []string{`"command": "printf 'a:b'"`, `"workspace_id": "ws_a"`, `"enabled": true`} {
		if !strings.Contains(view, expected) {
			t.Fatalf("arguments missing %q: %q", expected, view)
		}
	}
}

func newRequestPageServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer runtime-secret" {
			t.Fatalf("authorization=%q", r.Header.Get("Authorization"))
		}
		handler(w, r)
	}))
	previous := configformat.RootPath()
	t.Cleanup(func() { _ = configformat.SetRootPath(previous) })
	root := filepath.Join(t.TempDir(), "config")
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	state := runtimecontrol.State{PID: os.Getpid(), Address: parsed.Host, Token: "runtime-secret", ConfigRoot: root}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, runtimecontrol.FileName), data, 0600); err != nil {
		t.Fatal(err)
	}
	return server
}
