package notification

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/approval"
	"go.mewis.me/chatgpt-mcp/internal/controlguard"
	"go.mewis.me/chatgpt-mcp/internal/logger"
)

type fakeProvider struct {
	available bool
	caps      Capabilities
	err       error
	block     chan struct{}
	sent      chan Notification
}

func newFakeProvider() *fakeProvider {
	return &fakeProvider{available: true, caps: Capabilities{Notification: true}, sent: make(chan Notification, 8)}
}

func (f *fakeProvider) Available(context.Context) bool { return f.available }
func (f *fakeProvider) Capabilities(context.Context) Capabilities {
	return f.caps
}
func (f *fakeProvider) Send(ctx context.Context, note Notification) error {
	if f.block != nil {
		select {
		case <-f.block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if f.err != nil {
		return f.err
	}
	select {
	case f.sent <- note:
	default:
	}
	return nil
}

func waitNote(t *testing.T, sent <-chan Notification) Notification {
	t.Helper()
	select {
	case note := <-sent:
		return note
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for notification")
	}
	return Notification{}
}

func startService(t *testing.T, provider Provider, presence func() (bool, error), settings Settings) (*Service, *approval.Manager) {
	t.Helper()
	manager := approval.NewManager("inst_test")
	service := New(Options{
		Provider: provider,
		Presence: presence,
		Settings: func() Settings { return settings },
		Log:      logger.New(logger.Error),
	})
	service.Start(manager.Events())
	t.Cleanup(service.Stop)
	return service, manager
}

func publishRequested(manager *approval.Manager, id, title string) {
	manager.Events().Publish(approval.Event{
		Name:        approval.EventRequested,
		RequestID:   id,
		WorkspaceID: "ws_demo",
		TargetTool:  "run_command",
		Title:       title,
		Status:      approval.StatusPending,
	})
}

func TestServiceNotifiesPendingRequest(t *testing.T) {
	provider := newFakeProvider()
	_, manager := startService(t, provider, func() (bool, error) { return false, nil }, DefaultSettings())
	publishRequested(manager, "req_one", "Allow git push")
	note := waitNote(t, provider.sent)
	if note.Title != "ChatGPT MCP approval requested" || !strings.Contains(note.Body, "Allow git push") || !strings.Contains(note.Body, "Workspace request") {
		t.Fatalf("note=%#v", note)
	}
	if strings.Contains(note.Body, "git push origin") || strings.Contains(note.Body, "--force") {
		t.Fatalf("notification leaked command text: %#v", note)
	}
}

func TestServiceSuppressesWhenTUIActive(t *testing.T) {
	provider := newFakeProvider()
	_, manager := startService(t, provider, func() (bool, error) { return true, nil }, DefaultSettings())
	publishRequested(manager, "req_tui", "Allow git push")
	select {
	case note := <-provider.sent:
		t.Fatalf("notified while TUI active: %#v", note)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestServiceNotifiesWhenPresenceCheckFails(t *testing.T) {
	provider := newFakeProvider()
	_, manager := startService(t, provider, func() (bool, error) { return false, errors.New("lock broken") }, DefaultSettings())
	publishRequested(manager, "req_presence", "Allow git push")
	_ = waitNote(t, provider.sent)
}

func TestServiceDedupesRequestedEvents(t *testing.T) {
	provider := newFakeProvider()
	_, manager := startService(t, provider, func() (bool, error) { return false, nil }, DefaultSettings())
	publishRequested(manager, "req_dupe", "Allow git push")
	publishRequested(manager, "req_dupe", "Allow git push")
	_ = waitNote(t, provider.sent)
	select {
	case note := <-provider.sent:
		t.Fatalf("duplicate notification: %#v", note)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestServiceIgnoresResolutionEvents(t *testing.T) {
	provider := newFakeProvider()
	_, manager := startService(t, provider, func() (bool, error) { return false, nil }, DefaultSettings())
	manager.Events().Publish(approval.Event{Name: approval.EventApproved, RequestID: "req_done", Title: "Allow git push", Status: approval.StatusApproved})
	select {
	case note := <-provider.sent:
		t.Fatalf("resolution notified: %#v", note)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestServiceSkipsDisabledSettings(t *testing.T) {
	provider := newFakeProvider()
	settings := DefaultSettings()
	settings.Enabled = false
	_, manager := startService(t, provider, func() (bool, error) { return false, nil }, settings)
	publishRequested(manager, "req_off", "Allow git push")
	select {
	case note := <-provider.sent:
		t.Fatalf("disabled notified: %#v", note)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestServiceProviderFailureLeavesApprovalPending(t *testing.T) {
	provider := newFakeProvider()
	provider.err = errors.New("dbus down")
	_, manager := startService(t, provider, func() (bool, error) { return false, nil }, DefaultSettings())
	challenge, _, err := manager.CreateChallenge(approval.ChallengeInput{
		SessionID: "session", WorkspaceID: "ws_demo", TargetTool: "run_command",
		Arguments: map[string]any{"command": "rm -rf /tmp/secret"}, Title: "Allow cleanup", Command: "rm -rf /tmp/secret",
		GuardCode: controlguard.CodeExternalMutation,
	})
	if err != nil {
		t.Fatal(err)
	}
	request, _, err := manager.CreateRequest(challenge.ID, "session", "ws_demo")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	got, ok := manager.Get(request.ID)
	if !ok || got.Status != approval.StatusPending {
		t.Fatalf("request=%#v ok=%t", got, ok)
	}
}

func TestServiceSendDoesNotBlockRequestCreation(t *testing.T) {
	provider := newFakeProvider()
	provider.block = make(chan struct{})
	_, manager := startService(t, provider, func() (bool, error) { return false, nil }, DefaultSettings())
	done := make(chan struct{})
	go func() {
		defer close(done)
		publishRequested(manager, "req_async", "Allow git push")
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("publishing approval event blocked on notification send")
	}
	close(provider.block)
	_ = waitNote(t, provider.sent)
}

func TestServiceOpenActionFollowsCapabilities(t *testing.T) {
	provider := newFakeProvider()
	provider.caps = Capabilities{Notification: true, Actions: true}
	_, manager := startService(t, provider, func() (bool, error) { return false, nil }, DefaultSettings())
	publishRequested(manager, "req_action", "Allow git push")
	note := waitNote(t, provider.sent)
	if !note.OpenAction {
		t.Fatalf("expected open action: %#v", note)
	}
}

func TestUnavailableProviderDoesNotNotify(t *testing.T) {
	provider := newFakeProvider()
	provider.available = false
	_, manager := startService(t, provider, func() (bool, error) { return false, nil }, DefaultSettings())
	publishRequested(manager, "req_none", "Allow git push")
	select {
	case note := <-provider.sent:
		t.Fatalf("unavailable provider notified: %#v", note)
	case <-time.After(200 * time.Millisecond):
	}
}
