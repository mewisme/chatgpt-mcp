package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"go.mewis.me/chatgpt-mcp/internal/approval"
)

func TestRequestInteractiveFilterConfirmAndResolve(t *testing.T) {
	now := time.Now().UTC()
	first := approval.Request{ID: "req_first", Status: approval.StatusPending, WorkspaceID: "ws_a", TargetTool: "run_command", Title: "Allow cgm update", ExpiresAt: now.Add(time.Minute)}
	second := approval.Request{ID: "req_second", Status: approval.StatusPending, WorkspaceID: "ws_b", TargetTool: "run_command", Title: "Allow cgm install", ExpiresAt: now.Add(time.Minute)}
	approvedID := ""
	client := requestInteractiveClient{
		list: func(context.Context) ([]approval.Request, error) { return []approval.Request{first, second}, nil },
		view: func(context.Context, string) (approval.Request, error) { return approval.Request{}, nil },
		approve: func(_ context.Context, id, _ string) (approval.Request, error) {
			approvedID = id
			value := second
			value.Status = approval.StatusApproved
			return value, nil
		},
		deny: func(context.Context, string, string) (approval.Request, error) { return approval.Request{}, nil },
	}
	model := newRequestInteractiveModel(context.Background(), []approval.Request{first, second}, client)
	model.now = now
	model = updateRequestInteractive(t, model, keyText("/"))
	model = updateRequestInteractive(t, model, keyText("ws_b"))
	model = updateRequestInteractive(t, model, keyCode(tea.KeyEnter))
	if model.filter != "ws_b" || model.filtering || len(model.filtered()) != 1 {
		t.Fatalf("filter=%q filtering=%t items=%d", model.filter, model.filtering, len(model.filtered()))
	}
	model = updateRequestInteractive(t, model, keyText("a"))
	if !model.confirm.Active() || model.confirm.Action != "approve" || model.confirm.Target != second.ID {
		t.Fatalf("confirm=%#v", model.confirm)
	}
	updated, cmd := model.Update(keyText("y"))
	model = updated.(requestInteractiveModel)
	if cmd == nil {
		t.Fatal("approve confirmation did not return command")
	}
	message := cmd()
	resolved, ok := message.(requestInteractiveResolveMsg)
	if !ok || resolved.err != nil || resolved.request.ID != second.ID || approvedID != second.ID {
		t.Fatalf("resolve=%#v approved=%q", message, approvedID)
	}
	updated, _ = model.Update(message)
	model = updated.(requestInteractiveModel)
	if model.busy || model.err != nil || !strings.Contains(model.notice, second.ID) {
		t.Fatalf("resolved model=%#v", model)
	}
}

func TestRequestInteractiveDetailUsesAuthoritativeView(t *testing.T) {
	now := time.Now().UTC()
	request := approval.Request{ID: "req_detail", Status: approval.StatusPending, WorkspaceID: "ws_a", Source: "tunnel", TargetTool: "run_command", Title: "Allow cgm update", Arguments: []byte(`{"workspace_id":"ws_a","command":"cgm update"}`), GuardReason: "guarded", ExpiresAt: now.Add(time.Minute)}
	client := requestInteractiveClient{
		list: func(context.Context) ([]approval.Request, error) { return []approval.Request{request}, nil },
		view: func(_ context.Context, id string) (approval.Request, error) {
			if id != request.ID {
				t.Fatalf("view id=%q", id)
			}
			return request, nil
		},
		approve: func(context.Context, string, string) (approval.Request, error) { return approval.Request{}, nil },
		deny:    func(context.Context, string, string) (approval.Request, error) { return approval.Request{}, nil },
	}
	model := newRequestInteractiveModel(context.Background(), []approval.Request{request}, client)
	model.now = now
	updated, cmd := model.Update(keyCode(tea.KeyEnter))
	model = updated.(requestInteractiveModel)
	if cmd == nil {
		t.Fatal("detail key did not request detail")
	}
	updated, _ = model.Update(cmd())
	model = updated.(requestInteractiveModel)
	if !model.detail || model.detailRequest.ID != request.ID {
		t.Fatalf("detail model=%#v", model)
	}
	view := model.View().Content
	if !strings.Contains(view, `"command": "cgm update"`) || !strings.Contains(view, "guarded") {
		t.Fatalf("detail view=%q", view)
	}
}

func TestRequestInteractiveResolvedItemCannotBeApproved(t *testing.T) {
	request := approval.Request{ID: "req_denied", Status: approval.StatusDenied, WorkspaceID: "ws_a", TargetTool: "run_command", Title: "Denied"}
	model := newRequestInteractiveModel(context.Background(), []approval.Request{request}, requestInteractiveClient{})
	model = updateRequestInteractive(t, model, keyText("a"))
	if model.confirm.Active() || !strings.Contains(model.notice, "cannot be resolved") {
		t.Fatalf("model=%#v", model)
	}
}

func updateRequestInteractive(t *testing.T, model requestInteractiveModel, msg tea.Msg) requestInteractiveModel {
	t.Helper()
	updated, _ := model.Update(msg)
	value, ok := updated.(requestInteractiveModel)
	if !ok {
		t.Fatalf("updated model type=%T", updated)
	}
	return value
}

func keyText(value string) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Text: value, Code: []rune(value)[0]})
}
func keyCode(code rune) tea.KeyPressMsg { return tea.KeyPressMsg(tea.Key{Code: code}) }
