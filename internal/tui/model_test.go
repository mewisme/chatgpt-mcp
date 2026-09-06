package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
)

func TestModelFillsTerminalAndEnforcesMinimumLayout(t *testing.T) {
	model := NewModel(Route{Kind: RouteHome})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model = updated.(Model)
	view := model.View().Content
	if width, height := lipgloss.Width(view), lipgloss.Height(view); width != 120 || height != 40 {
		t.Fatalf("full terminal layout=%dx%d want 120x40", width, height)
	}
	updated, _ = model.Update(tea.WindowSizeMsg{Width: 20, Height: 8})
	model = updated.(Model)
	view = model.View().Content
	if width, height := lipgloss.Width(view), lipgloss.Height(view); width != minTerminalWidth || height != minTerminalHeight {
		t.Fatalf("minimum layout=%dx%d want %dx%d", width, height, minTerminalWidth, minTerminalHeight)
	}
}

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

func TestModelOpensAndRunsCommandPalette(t *testing.T) {
	model := NewModel(Route{Kind: RouteHome})
	updated, _ := model.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl | tea.ModShift})
	model = updated.(Model)
	if model.palette == nil {
		t.Fatal("Ctrl+Shift+P did not open palette")
	}
	model.palette.SetQuery("logs")
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(Model)
	if command == nil {
		t.Fatal("palette enter returned no command")
	}
	updated, command = model.Update(command())
	model = updated.(Model)
	if command == nil {
		t.Fatal("palette action returned no navigation command")
	}
	updated, _ = model.Update(command())
	model = updated.(Model)
	if model.router.Current().Kind != RouteLogs || model.palette != nil {
		t.Fatalf("route=%#v palette=%v", model.router.Current(), model.palette != nil)
	}
}

func TestModelPaletteFallbackKeys(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{{Code: 'p', Mod: tea.ModCtrl}, {Text: ":", Code: ':'}} {
		model := NewModel(Route{Kind: RouteHome})
		updated, _ := model.Update(key)
		model = updated.(Model)
		if model.palette == nil {
			t.Fatalf("%q did not open palette", key.String())
		}
	}
}

func TestModelQuickOpenNavigatesPage(t *testing.T) {
	defer configformat.SetRootPath("")
	if err := configformat.SetRootPath(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	model := NewModel(Route{Kind: RouteHome})
	updated, _ := model.Update(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl})
	model = updated.(Model)
	if model.palette == nil || model.overlay != overlayQuickOpen {
		t.Fatal("Ctrl+O did not open Quick Open")
	}
	model.palette.SetQuery("config")
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(Model)
	if command == nil {
		t.Fatal("Quick Open enter returned no command")
	}
	updated, _ = model.Update(command())
	model = updated.(Model)
	if model.router.Current().Kind != RouteConfig || model.palette != nil {
		t.Fatalf("route=%#v overlay=%d", model.router.Current(), model.overlay)
	}
}
