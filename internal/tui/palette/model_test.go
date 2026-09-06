package palette

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"go.mewis.me/chatgpt-mcp/internal/tui/action"
)

func TestPaletteQueryAndSelection(t *testing.T) {
	model := New([]action.Action{
		{ID: "logs", Title: "Go to Logs", Category: "App"},
		{ID: "config", Title: "Verify configuration", Category: "Config"},
	}, action.Context{})
	model.SetQuery("verify")
	if got := model.SelectedID(); got != "config" {
		t.Fatalf("selected = %q results=%v", got, model.DebugResults())
	}
	updated, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("selection command is nil")
	}
	if message, ok := cmd().(SelectedMsg); !ok || message.ID != "config" {
		t.Fatalf("message = %#v", cmd())
	}
	if updated.Query() != "verify" {
		t.Fatalf("query = %q", updated.Query())
	}
}

func TestPaletteClose(t *testing.T) {
	model := New(nil, action.Context{})
	_, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("close command is nil")
	}
	if _, ok := cmd().(ClosedMsg); !ok {
		t.Fatalf("message = %#v", cmd())
	}
}
