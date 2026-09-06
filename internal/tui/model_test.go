package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
	tuipage "go.mewis.me/chatgpt-mcp/internal/tui/page"
)

func TestModelFillsExactTerminalSizeWithoutMinimumLayout(t *testing.T) {
	defer configformat.SetRootPath("")
	if err := configformat.SetRootPath(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	for _, route := range []Route{{Kind: RouteHome}, {Kind: RouteWorkspaces}, {Kind: RouteMCP}, {Kind: RouteLogs}, {Kind: RouteConfig}, {Kind: RouteRuntime}, {Kind: RouteAbout}} {
		for _, size := range [][2]int{{120, 40}, {20, 8}, {3, 3}, {1, 1}} {
			model := NewModel(route)
			updated, _ := model.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			model = updated.(Model)
			view := model.View().Content
			if width, height := lipgloss.Width(view), lipgloss.Height(view); width != size[0] || height != size[1] {
				t.Fatalf("%s layout=%dx%d want %dx%d", route.Kind, width, height, size[0], size[1])
			}
		}
	}
}

func TestModelRendersDeepLinkAndNavigation(t *testing.T) {
	model := NewModel(Route{Kind: RouteMCP, ResourceID: "github"})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 32})
	model = updated.(Model)
	view := model.View().Content
	plain := ansi.Strip(view)
	lines := strings.Split(plain, "\n")
	if len(lines) < 2 || !strings.Contains(lines[0], "ChatGPT MCP") || strings.Contains(lines[1], "ChatGPT MCP") || !strings.Contains(plain, "Deep-linked resource: github") {
		t.Fatalf("view = %q", view)
	}
	cellWidth := (100 - 4) / len(headerPages)
	if 1 < (100-4)%len(headerPages) {
		cellWidth++
	}
	active := model.theme.navActive.Padding(0).Width(cellWidth).Align(lipgloss.Center).Render("MCP")
	if !strings.Contains(view, active) {
		t.Fatal("MCP header button is not active")
	}
	updated, command := model.Update(navigateMsg{route: Route{Kind: RouteLogs}, sibling: true})
	model = updated.(Model)
	if model.router.Current().Kind != RouteLogs {
		t.Fatalf("route = %#v", model.router.Current())
	}
	if command != nil {
		updated, _ = model.Update(command())
		model = updated.(Model)
	}
}

func TestModelHeaderCellsFillUsableWidth(t *testing.T) {
	model := NewModel(Route{Kind: RouteRequests})
	for _, width := range []int{52, 73, 96, 117} {
		header, targets := model.header(width, 2, 1)
		if lipgloss.Width(header) != width {
			t.Fatalf("header width=%d want=%d", lipgloss.Width(header), width)
		}
		if len(targets) != len(headerPages) {
			t.Fatalf("targets=%d want=%d", len(targets), len(headerPages))
		}
		x, total := 2, 0
		for index, target := range targets {
			if target.Rect.X != x || target.Rect.Y != 1 || target.Rect.Height != 1 {
				t.Fatalf("target %d rect=%#v want x=%d y=1", index, target.Rect, x)
			}
			x += target.Rect.Width
			total += target.Rect.Width
		}
		if total != width || x != width+2 {
			t.Fatalf("navbar coverage total=%d end=%d want total=%d end=%d", total, x, width, width+2)
		}
	}
}

func TestModelHidesNavbarWhenTerminalIsTooNarrow(t *testing.T) {
	model := NewModel(Route{Kind: RouteRequests})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	model = updated.(Model)
	if model.frameMetrics(60, 20).showNavbar {
		t.Fatal("narrow terminal kept navbar visible")
	}
	_, targets := model.render()
	for _, target := range targets {
		if strings.HasPrefix(target.ID, "app.header.") {
			t.Fatalf("hidden navbar exposed mouse target %q", target.ID)
		}
	}
	updated, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	model = updated.(Model)
	if !model.frameMetrics(100, 20).showNavbar {
		t.Fatal("wide terminal did not restore navbar")
	}
}

func TestModelNumberKeysDoNotSwitchHeaderPages(t *testing.T) {
	for _, value := range "1234567" {
		model := NewModel(Route{Kind: RouteHome})
		updated, cmd := model.Update(tea.KeyPressMsg{Code: value, Text: string(value)})
		model = updated.(Model)
		if cmd != nil || model.router.Current().Kind != RouteHome {
			t.Fatalf("number %q switched page: route=%s cmd=%v", value, model.router.Current().Kind, cmd)
		}
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
	updated, cmd := model.Update(tea.KeyPressMsg{Text: "q", Code: 'q'})
	model = updated.(Model)
	if cmd != nil || model.overlay != overlayNone {
		t.Fatalf("q triggered quit flow: overlay=%d cmd=%v", model.overlay, cmd)
	}
	updated, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(Model)
	if cmd != nil || model.overlay != overlayExitConfirm {
		t.Fatalf("root escape did not open exit confirmation: overlay=%d cmd=%v", model.overlay, cmd)
	}
}

func TestModelBackIntoRequestsRestartsPageInit(t *testing.T) {
	model := NewModel(Route{Kind: RouteHome})
	model.navigate(Route{Kind: RouteRequests})
	model.navigate(Route{Kind: RouteLogs})
	updated, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(Model)
	if model.router.Current().Kind != RouteRequests {
		t.Fatalf("route = %#v", model.router.Current())
	}
	if cmd == nil {
		t.Fatal("returning to requests did not restart page init")
	}
}

func TestModelOpensAndRunsCommandPalette(t *testing.T) {
	model := NewModel(Route{Kind: RouteHome})
	updated, _ := model.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	model = updated.(Model)
	if model.palette == nil {
		t.Fatal("Ctrl+P did not open palette")
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

func TestModelCommandPaletteOnlyUsesCtrlP(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{{Code: 'p', Mod: tea.ModCtrl | tea.ModShift}, {Text: ":", Code: ':'}} {
		model := NewModel(Route{Kind: RouteHome})
		updated, _ := model.Update(key)
		model = updated.(Model)
		if model.palette != nil {
			t.Fatalf("%q unexpectedly opened palette", key.String())
		}
	}
}

func TestModelFooterKeepsOnlyGlobalShortcuts(t *testing.T) {
	model := NewModel(Route{Kind: RouteHome})
	footer := ansi.Strip(model.shortcutFooter())
	for _, want := range []string{"ctrl+p commands", "ctrl+o open", "alt+←/→ pages", "esc quit"} {
		if !strings.Contains(footer, want) {
			t.Fatalf("footer missing %q: %q", want, footer)
		}
	}
	if strings.Contains(footer, "Commands") || strings.Contains(footer, "Quit") || strings.Contains(footer, "esc back") || strings.Contains(footer, "q quit") {
		t.Fatalf("footer=%q", footer)
	}
	model.router.Navigate(Route{Kind: RouteLogs})
	footer = ansi.Strip(model.shortcutFooter())
	if !strings.Contains(footer, "esc back") {
		t.Fatalf("nested footer=%q", footer)
	}
}

func TestModelHeaderPageCyclingWrapsWithoutGrowingHistory(t *testing.T) {
	model := NewModel(Route{Kind: RouteWorkspaces})
	for range 3 {
		updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModAlt})
		model = updated.(Model)
		if model.router.Current().Kind != RouteRuntime {
			t.Fatalf("left wrap route=%#v", model.router.Current())
		}
		updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModAlt})
		model = updated.(Model)
		if model.router.Current().Kind != RouteWorkspaces {
			t.Fatalf("right wrap route=%#v", model.router.Current())
		}
	}
	if len(model.router.stack) != 1 {
		t.Fatalf("sibling page cycling grew route history: %#v", model.router.stack)
	}
}

func TestModelHeaderMouseClickUsesTypedNavigation(t *testing.T) {
	model := NewModel(Route{Kind: RouteWorkspaces})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 32})
	model = updated.(Model)
	view := model.View()
	if view.OnMouse == nil || view.MouseMode == tea.MouseModeNone {
		t.Fatal("mouse support is not enabled")
	}
	_, targets := model.render()
	var target *component.MouseTarget
	for index := range targets {
		if targets[index].ID == "app.header.config" {
			target = &targets[index]
			break
		}
	}
	if target == nil {
		t.Fatal("config header hitbox not found")
	}
	cmd := view.OnMouse(tea.MouseClickMsg(tea.Mouse{X: target.Rect.X, Y: target.Rect.Y, Button: tea.MouseLeft}))
	if cmd == nil {
		t.Fatal("header click produced no command")
	}
	message := cmd()
	navigation, ok := message.(navigateMsg)
	if !ok || navigation.route.Kind != RouteConfig || !navigation.sibling {
		t.Fatalf("header click message=%#v", message)
	}
	updated, _ = model.Update(message)
	model = updated.(Model)
	if model.router.Current().Kind != RouteConfig {
		t.Fatalf("route=%#v", model.router.Current())
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

func TestModelRoutesKeysToActiveDialogBeforeGlobalShortcuts(t *testing.T) {
	model := NewModel(Route{Kind: RouteHome})
	page := &captureOverlayPage{overlay: true}
	model.currentPage = page
	for _, message := range []tea.KeyPressMsg{
		{Code: 'p', Mod: tea.ModCtrl},
		{Code: 'o', Mod: tea.ModCtrl},
		{Code: 'c', Mod: tea.ModCtrl},
		{Code: 'q', Text: "q"},
		{Code: tea.KeyRight, Mod: tea.ModAlt},
		{Code: 'e', Text: "e"},
	} {
		updated, cmd := model.Update(message)
		model = updated.(Model)
		if cmd != nil {
			t.Fatalf("overlay key %q escaped to global command", message.String())
		}
		if model.palette != nil || model.router.Current().Kind != RouteHome {
			t.Fatalf("overlay key %q changed global UI palette=%v route=%s", message.String(), model.palette != nil, model.router.Current().Kind)
		}
	}
	want := []string{"ctrl+p", "ctrl+o", "ctrl+c", "q", "alt+right", "e"}
	if strings.Join(page.keys, ",") != strings.Join(want, ",") {
		t.Fatalf("captured keys=%v want=%v", page.keys, want)
	}
}

func TestModelRoutesKeysToActiveInputBeforeGlobalShortcuts(t *testing.T) {
	model := NewModel(Route{Kind: RouteHome})
	page := &captureOverlayPage{input: true}
	model.currentPage = page
	for _, message := range []tea.KeyPressMsg{
		{Code: 'p', Mod: tea.ModCtrl},
		{Code: 'o', Mod: tea.ModCtrl},
		{Code: 'c', Mod: tea.ModCtrl},
		{Code: 'q', Text: "q"},
		{Code: tea.KeyRight, Mod: tea.ModAlt},
		{Code: 'e', Text: "e"},
	} {
		updated, cmd := model.Update(message)
		model = updated.(Model)
		if cmd != nil || model.palette != nil || model.router.Current().Kind != RouteHome {
			t.Fatalf("input key %q escaped capture: cmd=%v palette=%v route=%s", message.String(), cmd, model.palette != nil, model.router.Current().Kind)
		}
	}
	want := []string{"ctrl+p", "ctrl+o", "ctrl+c", "q", "alt+right", "e"}
	if strings.Join(page.keys, ",") != strings.Join(want, ",") {
		t.Fatalf("captured keys=%v want=%v", page.keys, want)
	}
}

func TestModelSingleEscapeOpensExitConfirmation(t *testing.T) {
	model := NewModel(Route{Kind: RouteHome})
	updated, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(Model)
	if cmd != nil || model.overlay != overlayExitConfirm {
		t.Fatalf("escape overlay=%d cmd=%v", model.overlay, cmd)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(Model)
	if model.overlay != overlayNone {
		t.Fatalf("escape did not close exit dialog: overlay=%d", model.overlay)
	}
}

type captureOverlayPage struct {
	overlay bool
	input   bool
	keys    []string
}

func (page *captureOverlayPage) Init() tea.Cmd { return nil }

func (page *captureOverlayPage) Update(message tea.Msg) (tuipage.Model, tea.Cmd) {
	if key, ok := message.(tea.KeyPressMsg); ok {
		page.keys = append(page.keys, key.String())
	}
	return page, nil
}

func (page *captureOverlayPage) View(width, height int) string { return "" }
func (page *captureOverlayPage) OverlayActive() bool           { return page.overlay }
func (page *captureOverlayPage) InputActive() bool             { return page.input }
