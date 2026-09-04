package interactive

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestBrowserFilterDetailAndRefreshPreservesSelection(t *testing.T) {
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
	model = updateBrowser(t, model, browserKeyText("/"))
	model = updateBrowser(t, model, browserKeyText("beta"))
	model = updateBrowser(t, model, browserKeyCode(tea.KeyEnter))
	if model.filter != "beta" || model.filtering || len(model.filtered()) != 1 {
		t.Fatalf("filter=%q filtering=%t count=%d", model.filter, model.filtering, len(model.filtered()))
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
	updated, _ = model.Update(cmd())
	model = updated.(Browser)
	selected, ok := model.selected()
	if refreshCalls != 1 || !ok || selected.ID != "two" || !model.detail || selected.Detail != "updated second" {
		t.Fatalf("refreshCalls=%d selected=%#v ok=%t detail=%t", refreshCalls, selected, ok, model.detail)
	}
}

func TestBrowserRefreshErrorKeepsRows(t *testing.T) {
	model := NewBrowser(context.Background(), "Items", []Row{{ID: "one", Summary: "one"}}, func(context.Context) ([]Row, error) {
		return nil, errors.New("refresh failed")
	})
	updated, cmd := model.Update(browserKeyText("r"))
	model = updated.(Browser)
	updated, _ = model.Update(cmd())
	model = updated.(Browser)
	if model.err == nil || model.err.Error() != "refresh failed" || len(model.rows) != 1 || model.rows[0].ID != "one" {
		t.Fatalf("model=%#v", model)
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
	updated, _ = model.Update(cmd())
	model = updated.(Browser)
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
	if !model.detail || model.viewport.Width() != 64 || model.viewport.Height() != 4 {
		t.Fatalf("detail=%t viewport=%dx%d", model.detail, model.viewport.Width(), model.viewport.Height())
	}
	before := model.viewport.YOffset()
	model = updateBrowser(t, model, browserKeyText("j"))
	if model.viewport.YOffset() <= before {
		t.Fatalf("viewport did not scroll: before=%d after=%d", before, model.viewport.YOffset())
	}
}

func updateBrowser(t *testing.T, model Browser, msg tea.Msg) Browser {
	t.Helper()
	updated, _ := model.Update(msg)
	value, ok := updated.(Browser)
	if !ok {
		t.Fatalf("updated model type=%T", updated)
	}
	return value
}

func browserKeyText(value string) tea.KeyPressMsg {
	return tea.KeyPressMsg(tea.Key{Text: value, Code: []rune(value)[0]})
}
func browserKeyCode(code rune) tea.KeyPressMsg { return tea.KeyPressMsg(tea.Key{Code: code}) }
