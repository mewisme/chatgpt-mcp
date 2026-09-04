package interactive

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestBrowserDefaultListFilterDetailAndRefreshPreservesSelection(t *testing.T) {
	rows := []Row{{ID: "one", Summary: "one alpha", Detail: "first detail"}, {ID: "two", Summary: "two beta", Detail: "second detail"}}
	refreshCalls := 0
	model := NewBrowser(context.Background(), "Items", rows, func(context.Context) ([]Row, error) {
		refreshCalls++
		return []Row{{ID: "one", Summary: "one alpha", Detail: "updated first"}, {ID: "two", Summary: "two beta updated", Detail: "updated second"}}, nil
	})
	model = updateBrowser(t, model, browserKeyText("j"))
	if selected, ok := model.selected(); !ok || selected.ID != "two" {
		t.Fatalf("selected=%#v ok=%t", selected, ok)
	}
	model.list.SetFilterText("beta")
	if model.list.FilterValue() != "beta" || len(model.list.VisibleItems()) != 1 {
		t.Fatalf("filter=%q visible=%d", model.list.FilterValue(), len(model.list.VisibleItems()))
	}
	model = updateBrowser(t, model, browserKeyCode(tea.KeyEnter))
	if !model.detail || !strings.Contains(model.View().Content, "second detail") {
		t.Fatalf("detail=%t view=%q", model.detail, model.View().Content)
	}
	updated, cmd := model.Update(browserKeyText("r"))
	model = updated.(Browser)
	if cmd == nil || !model.loading {
		t.Fatalf("refresh cmd=%v loading=%t", cmd, model.loading)
	}
	updated, cmd = model.Update(cmd())
	model = updated.(Browser)
	model = runBrowserCmd(t, model, cmd)
	selected, ok := model.selected()
	if refreshCalls != 1 || !ok || selected.ID != "two" || !model.detail || selected.Detail != "updated second" {
		t.Fatalf("refreshCalls=%d selected=%#v ok=%t detail=%t", refreshCalls, selected, ok, model.detail)
	}
}

func TestBrowserRefreshErrorKeepsItems(t *testing.T) {
	model := NewBrowser(context.Background(), "Items", []Row{{ID: "one", Summary: "one"}}, func(context.Context) ([]Row, error) {
		return nil, errors.New("refresh failed")
	})
	updated, cmd := model.Update(browserKeyText("r"))
	model = updated.(Browser)
	updated, _ = model.Update(cmd())
	model = updated.(Browser)
	if model.err == nil || model.err.Error() != "refresh failed" || len(model.list.Items()) != 1 {
		t.Fatalf("err=%v items=%d", model.err, len(model.list.Items()))
	}
}

func TestBrowserRefreshClosesRemovedDetail(t *testing.T) {
	model := NewBrowser(context.Background(), "Items", []Row{{ID: "one", Summary: "one"}, {ID: "two", Summary: "two"}}, func(context.Context) ([]Row, error) {
		return []Row{{ID: "two", Summary: "two"}}, nil
	})
	model = updateBrowser(t, model, browserKeyCode(tea.KeyEnter))
	if !model.detail {
		t.Fatal("detail did not open")
	}
	updated, cmd := model.Update(browserKeyText("r"))
	model = updated.(Browser)
	updated, next := model.Update(cmd())
	model = updated.(Browser)
	model = runBrowserCmd(t, model, next)
	if model.detail {
		t.Fatal("detail remained open after selected item disappeared")
	}
}

func TestBrowserDetailViewportScrollsAndResizes(t *testing.T) {
	lines := make([]string, 30)
	for index := range lines {
		lines[index] = "detail line " + string(rune('A'+index%26))
	}
	model := NewBrowser(context.Background(), "Items", []Row{{ID: "one", Title: "One", Description: "first", Detail: strings.Join(lines, "\n")}}, nil)
	model = updateBrowser(t, model, tea.WindowSizeMsg{Width: 72, Height: 14})
	model = updateBrowser(t, model, browserKeyCode(tea.KeyEnter))
	if !model.detail || model.viewport.Width() != 64 || model.viewport.Height() != 8 {
		t.Fatalf("detail=%t viewport=%dx%d", model.detail, model.viewport.Width(), model.viewport.Height())
	}
	before := model.viewport.YOffset()
	model = updateBrowser(t, model, browserKeyText("j"))
	if model.viewport.YOffset() <= before {
		t.Fatalf("viewport did not scroll: before=%d after=%d", before, model.viewport.YOffset())
	}
}

func TestBrowserRowActionUsesSelectedItemAndDefaultHelp(t *testing.T) {
	copied := ""
	model := NewBrowser(context.Background(), "Items", []Row{{ID: "one", Title: "One"}, {ID: "two", Title: "Two"}}, nil).WithAction(RowAction{Key: "c", Desc: "copy ID", Run: func(row Row) (string, tea.Cmd, error) {
		copied = row.ID
		return "Copied " + row.ID, nil, nil
	}})
	model = updateBrowser(t, model, browserKeyText("j"))
	model = updateBrowser(t, model, browserKeyText("c"))
	if copied != "two" || model.notice != "Copied two" || model.err != nil {
		t.Fatalf("copied=%q notice=%q err=%v", copied, model.notice, model.err)
	}
	if !strings.Contains(model.list.View(), "copy ID") {
		t.Fatalf("view=%q", model.list.View())
	}
}

func updateBrowser(t *testing.T, model Browser, msg tea.Msg) Browser {
	t.Helper()
	updated, cmd := model.Update(msg)
	value, ok := updated.(Browser)
	if !ok {
		t.Fatalf("updated model type=%T", updated)
	}
	return runBrowserCmd(t, value, cmd)
}

func runBrowserCmd(t *testing.T, model Browser, cmd tea.Cmd) Browser {
	t.Helper()
	if cmd == nil {
		return model
	}
	message := cmd()
	if batch, ok := message.(tea.BatchMsg); ok {
		for _, next := range batch {
			model = runBrowserCmd(t, model, next)
		}
		return model
	}
	updated, next := model.Update(message)
	value, ok := updated.(Browser)
	if !ok {
		t.Fatalf("updated model type=%T", updated)
	}
	return runBrowserCmd(t, value, next)
}

func browserKeyText(value string) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Text: value, Code: []rune(value)[0]})
}
func browserKeyCode(code rune) tea.KeyPressMsg { return tea.KeyPressMsg(tea.Key{Code: code}) }
