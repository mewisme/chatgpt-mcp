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
	model.list.SetFilterText("ws_b")
	if model.list.FilterValue() != "ws_b" || len(model.list.VisibleItems()) != 1 {
		t.Fatalf("filter=%q items=%d", model.list.FilterValue(), len(model.list.VisibleItems()))
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
	if len(model.requests) != 1 || model.requests[0].ID != first.ID {
		t.Fatalf("resolved request remained in inbox: %#v", model.requests)
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

func TestRequestInteractiveHidesResolvedItems(t *testing.T) {
	pending := approval.Request{ID: "req_pending", Status: approval.StatusPending, WorkspaceID: "ws_a", TargetTool: "run_command", Title: "Pending"}
	approved := approval.Request{ID: "req_approved", Status: approval.StatusApproved, WorkspaceID: "ws_a", TargetTool: "run_command", Title: "Approved"}
	denied := approval.Request{ID: "req_denied", Status: approval.StatusDenied, WorkspaceID: "ws_a", TargetTool: "run_command", Title: "Denied"}
	consumed := approval.Request{ID: "req_consumed", Status: approval.StatusConsumed, WorkspaceID: "ws_a", TargetTool: "run_command", Title: "Consumed"}
	model := newRequestInteractiveModel(context.Background(), []approval.Request{pending, approved, denied, consumed}, requestInteractiveClient{})
	if len(model.requests) != 1 || model.requests[0].ID != pending.ID {
		t.Fatalf("interactive inbox=%#v", model.requests)
	}
	view := model.View().Content
	if !strings.Contains(view, "Pending control approvals") || strings.Contains(view, approved.ID) || strings.Contains(view, denied.ID) || strings.Contains(view, consumed.ID) {
		t.Fatalf("view=%q", view)
	}
}

func TestRequestInteractiveRefreshDropsResolvedItems(t *testing.T) {
	now := time.Now().UTC()
	pending := approval.Request{ID: "req_pending", Status: approval.StatusPending, WorkspaceID: "ws_a", TargetTool: "run_command", Title: "Pending", ExpiresAt: now.Add(time.Minute)}
	approved := pending
	approved.Status = approval.StatusApproved
	client := requestInteractiveClient{list: func(context.Context) ([]approval.Request, error) { return []approval.Request{approved}, nil }}
	model := newRequestInteractiveModel(context.Background(), []approval.Request{pending}, client)
	updated, cmd := model.Update(keyText("r"))
	model = updated.(requestInteractiveModel)
	updated, _ = model.Update(cmd())
	model = updated.(requestInteractiveModel)
	if len(model.requests) != 0 {
		t.Fatalf("resolved request remained after refresh: %#v", model.requests)
	}
}

func TestRequestInteractiveViewRendersHierarchyAndScrollableDetail(t *testing.T) {
	now := time.Now().UTC()
	arguments := `{"workspace_id":"ws_a","command":"cgm update","notes":"line one\nline two\nline three\nline four\nline five\nline six\nline seven\nline eight"}`
	request := approval.Request{ID: "req_visual", Status: approval.StatusPending, WorkspaceID: "ws_a", Source: "tunnel", TargetTool: "run_command", Title: "Allow cgm update", Arguments: []byte(arguments), GuardReason: "control-plane mutation", ExpiresAt: now.Add(time.Minute)}
	client := requestInteractiveClient{list: func(context.Context) ([]approval.Request, error) { return []approval.Request{request}, nil }, view: func(context.Context, string) (approval.Request, error) { return request, nil }}
	model := newRequestInteractiveModel(context.Background(), []approval.Request{request}, client)
	model.now = now
	model = updateRequestInteractive(t, model, tea.WindowSizeMsg{Width: 76, Height: 16})
	listView := model.View().Content
	for _, expected := range []string{"Pending control approvals", "Allow cgm update", "PENDING", "req_visual", "ws_a", "run_command"} {
		if !strings.Contains(listView, expected) {
			t.Fatalf("list view missing %q: %q", expected, listView)
		}
	}
	updated, cmd := model.Update(keyCode(tea.KeyEnter))
	model = updated.(requestInteractiveModel)
	updated, _ = model.Update(cmd())
	model = updated.(requestInteractiveModel)
	if !model.detail || !strings.Contains(model.viewport.GetContent(), "Arguments") || !strings.Contains(model.viewport.GetContent(), `"command": "cgm update"`) {
		t.Fatalf("detail content=%q", model.viewport.GetContent())
	}
	before := model.viewport.YOffset()
	model = updateRequestInteractive(t, model, keyText("j"))
	if model.viewport.YOffset() <= before {
		t.Fatalf("viewport did not scroll: before=%d after=%d", before, model.viewport.YOffset())
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
