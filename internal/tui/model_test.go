package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestModelRendersDeepLinkAndNavigation(t *testing.T) {
	model := NewModel(Route{Kind: RouteMCP, ResourceID: "github"})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 32})
	model = updated.(Model)
	view := model.View().Content
	if !strings.Contains(view, "MCP Servers · github") || !strings.Contains(view, "Deep-linked resource: github") {
		t.Fatalf("view = %q", view)
	}
	updated, command := model.Update(tea.KeyPressMsg{Text: "5", Code: '5'})
	model = updated.(Model)
	if command == nil {
		t.Fatal("navigation action returned no command")
	}
	updated, _ = model.Update(command())
	model = updated.(Model)
	if model.router.Current().Kind != RouteLogs {
		t.Fatalf("route = %#v", model.router.Current())
	}
}

func TestModelQuitAndBack(t *testing.T) {
	model := NewModel(Route{Kind: RouteHome})
	model.router.Navigate(Route{Kind: RouteConfig})
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(Model)
	if model.router.Current().Kind != RouteHome {
		t.Fatalf("route = %#v", model.router.Current())
	}
	_, cmd := model.Update(tea.KeyPressMsg{Text: "q", Code: 'q'})
	if cmd == nil {
		t.Fatal("quit command is nil")
	}
}
