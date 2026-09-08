package tui

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"go.mewis.me/chatgpt-mcp/internal/approval"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
	tuipage "go.mewis.me/chatgpt-mcp/internal/tui/page"
)

func TestModelWindowTitleTracksCurrentRoute(t *testing.T) {
	model := NewModel(Route{Kind: RouteHome})
	if got := model.View().WindowTitle; got != "ChatGPT MCP · Home" {
		t.Fatalf("window title=%q", got)
	}
	model.router.Switch(Route{Kind: RouteRuntime})
	if got := model.View().WindowTitle; got != "ChatGPT MCP · Runtime" {
		t.Fatalf("window title after route change=%q", got)
	}
}

func TestModelFillsExactTerminalSizeWithoutMinimumLayout(t *testing.T) {
	defer configformat.SetRootPath("")
	if err := configformat.SetRootPath(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	for _, route := range []Route{{Kind: RouteHome}, {Kind: RouteWorkspaces}, {Kind: RouteMCP}, {Kind: RouteLogs}, {Kind: RouteConfig}, {Kind: RouteInstruction}, {Kind: RouteRuntime}, {Kind: RouteAbout}} {
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

func TestModelResizeAndThemeStressKeepsExactGeometry(t *testing.T) {
	defer configformat.SetRootPath("")
	if err := configformat.SetRootPath(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	model := NewModel(Route{Kind: RouteLogs})
	sizes := [][2]int{{160, 50}, {80, 24}, {40, 10}, {20, 6}, {3, 3}, {1, 1}, {100, 32}}
	for round := range 8 {
		updated, _ := model.Update(tea.BackgroundColorMsg{Color: lipgloss.Color("#ffffff")})
		model = updated.(Model)
		if round%2 == 1 {
			updated, _ = model.Update(tea.BackgroundColorMsg{Color: lipgloss.Color("#000000")})
			model = updated.(Model)
		}
		for _, size := range sizes {
			updated, _ = model.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			model = updated.(Model)
			view := model.View().Content
			if width, height := lipgloss.Width(view), lipgloss.Height(view); width != size[0] || height != size[1] {
				t.Fatalf("round=%d layout=%dx%d want=%dx%d", round, width, height, size[0], size[1])
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
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 40, Height: 20})
	model = updated.(Model)
	if model.frameMetrics(40, 20).showNavbar {
		t.Fatal("narrow terminal kept navbar visible")
	}
	_, targets := model.render()
	for _, target := range targets {
		if strings.HasPrefix(target.ID, "app.header.") {
			t.Fatalf("hidden navbar exposed mouse target %q", target.ID)
		}
	}
	updated, _ = model.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	model = updated.(Model)
	if !model.frameMetrics(60, 20).showNavbar {
		t.Fatal("compact navbar did not restore at usable width")
	}
	header, _ := model.header(56, 2, 1)
	plain := ansi.Strip(header)
	if !strings.Contains(plain, "Instr") || strings.Contains(plain, "Instruction") {
		t.Fatalf("compact header=%q", plain)
	}
	updated, _ = model.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	model = updated.(Model)
	header, _ = model.header(96, 2, 1)
	if plain = ansi.Strip(header); !strings.Contains(plain, "Instruction") {
		t.Fatalf("full header=%q", plain)
	}
}

func TestModelNumberKeysDoNotSwitchHeaderPages(t *testing.T) {
	for _, value := range "1234567" {
		model := NewModel(Route{Kind: RouteHome})
		updated, _ := model.Update(tea.KeyPressMsg{Code: value, Text: string(value)})
		model = updated.(Model)
		query := ""
		if model.homeCommands != nil {
			query = model.homeCommands.Query()
		}
		if model.router.Current().Kind != RouteHome || query != string(value) {
			t.Fatalf("number %q route=%s query=%q", value, model.router.Current().Kind, query)
		}
	}
}

func TestHomeEmbedsCenteredCommandPanel(t *testing.T) {
	model := NewModel(Route{Kind: RouteHome})
	if model.homeCommands == nil || model.palette != nil || model.overlay != overlayNone {
		t.Fatalf("home commands=%v palette=%v overlay=%d", model.homeCommands != nil, model.palette != nil, model.overlay)
	}
	plain := ansi.Strip(model.homeView(100, 28))
	for _, want := range []string{"Commands", "Type a command or resource", "Ctrl+K", "Enter run", "Esc exit", "Alt+←/→ pages"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("home panel missing %q: %q", want, plain)
		}
	}
	lines := strings.Split(plain, "\n")
	first, last := -1, -1
	for index, line := range lines {
		if strings.TrimSpace(line) != "" {
			if first < 0 {
				first = index
			}
			last = index
		}
	}
	if first <= 0 || last >= len(lines)-1 {
		t.Fatalf("command panel is not vertically centered: first=%d last=%d height=%d", first, last, len(lines))
	}
}

func TestHomeCommandPanelKeepsGlobalNavigationAndEscape(t *testing.T) {
	model := NewModel(Route{Kind: RouteHome})
	updated, _ := model.Update(tea.KeyPressMsg{Code: 'l', Text: "l"})
	model = updated.(Model)
	query := "<nil>"
	if model.homeCommands != nil {
		query = model.homeCommands.Query()
	}
	if query != "l" {
		t.Fatalf("home query=%q", query)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModAlt})
	model = updated.(Model)
	if model.router.Current().Kind != RouteWorkspaces || model.homeCommands != nil {
		t.Fatalf("alt+right route=%s homeCommands=%v", model.router.Current().Kind, model.homeCommands != nil)
	}
	model.switchPage(Route{Kind: RouteHome})
	if model.homeCommands == nil {
		t.Fatal("returning home did not restore embedded command panel")
	}
	_, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("home escape did not quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("home escape message=%T", cmd())
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
	updated, _ = model.Update(tea.KeyPressMsg{Text: "q", Code: 'q'})
	model = updated.(Model)
	query := ""
	if model.homeCommands != nil {
		query = model.homeCommands.Query()
	}
	if model.overlay != overlayNone || query != "q" {
		t.Fatalf("q triggered quit flow: overlay=%d query=%q", model.overlay, query)
	}
	updated, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("root escape did not quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("root escape message=%T", cmd())
	}
}

func TestModelReplaceNavigationDropsDeletedResourceRoute(t *testing.T) {
	model := NewModel(Route{Kind: RouteWorkspaces})
	model.router.Navigate(Route{Kind: RouteWorkspaces, ResourceID: "ws_deleted"})
	updated, cmd := model.Update(tuipage.NavigateMsg{Path: []string{"workspaces"}, Replace: true})
	model = updated.(Model)
	if current := model.router.Current(); current != (Route{Kind: RouteWorkspaces}) {
		t.Fatalf("replace current=%#v", current)
	}
	if len(model.router.stack) != 1 || model.router.stack[0] != (Route{Kind: RouteWorkspaces}) {
		t.Fatalf("replace stack=%#v", model.router.stack)
	}
	if cmd != nil {
		updated, _ = model.Update(cmd())
		model = updated.(Model)
	}
}

func TestModelEscBacksToCurrentMainThenHomeThenQuits(t *testing.T) {
	model := NewModel(Route{Kind: RouteWorkspaces, ResourceID: "ws_previous"})
	model.navigate(Route{Kind: RouteLogs})
	model.router.Navigate(Route{Kind: RouteLogs, ResourceID: "event_current"})
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(Model)
	if model.router.Current() != (Route{Kind: RouteLogs}) {
		t.Fatalf("first escape route=%#v stack=%#v", model.router.Current(), model.router.stack)
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(Model)
	if model.router.Current() != (Route{Kind: RouteHome}) {
		t.Fatalf("second escape route=%#v stack=%#v", model.router.Current(), model.router.stack)
	}
	updated, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	model = updated.(Model)
	if cmd == nil {
		t.Fatal("third escape did not quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("third escape message=%T", cmd())
	}
}

func TestModelOpensAndRunsCommands(t *testing.T) {
	model := NewModel(Route{Kind: RouteAbout})
	updated, _ := model.Update(tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	model = updated.(Model)
	if model.palette == nil {
		t.Fatal("Ctrl+K did not open commands")
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

func TestModelCommandsOnlyUsesCtrlK(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{{Code: 'p', Mod: tea.ModCtrl}, {Code: 'o', Mod: tea.ModCtrl}, {Code: 'k', Mod: tea.ModCtrl | tea.ModShift}, {Text: ":", Code: ':'}} {
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
	for _, want := range []string{"ctrl+k commands", "alt+←/→ pages", "esc quit"} {
		if !strings.Contains(footer, want) {
			t.Fatalf("footer missing %q: %q", want, footer)
		}
	}
	if strings.Contains(footer, "ctrl+p") || strings.Contains(footer, "ctrl+o") || strings.Contains(footer, " open") {
		t.Fatalf("footer retained old command/open shortcuts: %q", footer)
	}
	if strings.Contains(footer, "Commands") || strings.Contains(footer, "Quit") || strings.Contains(footer, "esc back") || strings.Contains(footer, "q quit") {
		t.Fatalf("footer=%q", footer)
	}
	model.router.Navigate(Route{Kind: RouteLogs})
	footer = ansi.Strip(model.shortcutFooter())
	if !strings.Contains(footer, "esc home") {
		t.Fatalf("main page footer=%q", footer)
	}
	model.router.Navigate(Route{Kind: RouteLogs, ResourceID: "event_demo"})
	footer = ansi.Strip(model.shortcutFooter())
	if !strings.Contains(footer, "esc back") {
		t.Fatalf("child footer=%q", footer)
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

func TestModelCommandsNavigateResource(t *testing.T) {
	defer configformat.SetRootPath("")
	if err := configformat.SetRootPath(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	model := NewModel(Route{Kind: RouteAbout})
	updated, _ := model.Update(tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl})
	model = updated.(Model)
	if model.palette == nil || model.overlay != overlayCommands {
		t.Fatal("Ctrl+K did not open merged commands")
	}
	model.palette.SetQuery("config")
	updated, command := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(Model)
	if command == nil {
		t.Fatal("Commands enter returned no command")
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
		{Code: 'k', Mod: tea.ModCtrl},
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
	want := []string{"ctrl+k", "ctrl+c", "q", "alt+right", "e"}
	if strings.Join(page.keys, ",") != strings.Join(want, ",") {
		t.Fatalf("captured keys=%v want=%v", page.keys, want)
	}
}

func TestModelRoutesKeysToActiveInputBeforeGlobalShortcuts(t *testing.T) {
	model := NewModel(Route{Kind: RouteHome})
	page := &captureOverlayPage{input: true}
	model.currentPage = page
	for _, message := range []tea.KeyPressMsg{
		{Code: 'k', Mod: tea.ModCtrl},
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
	want := []string{"ctrl+k", "ctrl+c", "q", "alt+right", "e"}
	if strings.Join(page.keys, ",") != strings.Join(want, ",") {
		t.Fatalf("captured keys=%v want=%v", page.keys, want)
	}
}

func TestModelSingleEscapeQuitsFromHome(t *testing.T) {
	model := NewModel(Route{Kind: RouteHome})
	_, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd == nil {
		t.Fatal("escape did not quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("escape message=%T", cmd())
	}
}

type captureOverlayPage struct {
	overlay bool
	input   bool
	keys    []string
}

type confirmCapturePage struct{ affirmative *bool }

func (*confirmCapturePage) Init() tea.Cmd { return nil }
func (page *confirmCapturePage) Update(message tea.Msg) (tuipage.Model, tea.Cmd) {
	if choice, ok := message.(component.ConfirmChoiceMsg); ok {
		value := choice.Affirmative
		page.affirmative = &value
	}
	return page, nil
}
func (*confirmCapturePage) View(width, height int) string { return "" }
func (*confirmCapturePage) OverlayActive() bool           { return true }
func (*confirmCapturePage) InputActive() bool             { return false }

func TestModelRoutesNonApprovalConfirmChoiceToPage(t *testing.T) {
	model := NewModel(Route{Kind: RouteHome})
	page := &confirmCapturePage{}
	model.currentPage = page
	updated, cmd := model.Update(component.ConfirmChoiceMsg{Affirmative: true})
	model = updated.(Model)
	if cmd != nil || page.affirmative == nil || !*page.affirmative {
		t.Fatalf("confirm was not routed to page: cmd=%v affirmative=%v", cmd, page.affirmative)
	}
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

func TestModelPendingApprovalOverlaysEveryRoute(t *testing.T) {
	defer configformat.SetRootPath("")
	if err := configformat.SetRootPath(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	request := testPendingApproval("req_global")
	for _, route := range []Route{{Kind: RouteHome}, {Kind: RouteWorkspaces}, {Kind: RouteContainers}, {Kind: RouteMCP}, {Kind: RouteTunnel}, {Kind: RouteTunnels}, {Kind: RouteRequests}, {Kind: RouteLogs}, {Kind: RouteConfig}, {Kind: RouteInstruction}, {Kind: RouteRuntime}, {Kind: RouteAbout}} {
		t.Run(string(route.Kind), func(t *testing.T) {
			model := NewModel(route)
			updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
			model = updated.(Model)
			model.applyApprovalPoll(approvalPollMsg{requests: []approval.Request{request}})
			plain := ansi.Strip(model.View().Content)
			for _, want := range []string{"Approval request", request.ID, request.WorkspaceID, request.TargetTool, "echo hello"} {
				if !strings.Contains(plain, want) {
					t.Fatalf("route %s approval overlay missing %q: %q", route.Kind, want, plain)
				}
			}
		})
	}
}

func TestModelPendingApprovalSupersedesEveryInteractiveState(t *testing.T) {
	request := testPendingApproval("req_blocking")
	for _, test := range []struct {
		name  string
		setup func(*Model) *captureOverlayPage
	}{
		{name: "plain"},
		{name: "palette", setup: func(model *Model) *captureOverlayPage { _ = model.openCommands(); return nil }},
		{name: "page-overlay", setup: func(model *Model) *captureOverlayPage {
			page := &captureOverlayPage{overlay: true}
			model.currentPage = page
			return page
		}},
		{name: "page-input", setup: func(model *Model) *captureOverlayPage {
			page := &captureOverlayPage{input: true}
			model.currentPage = page
			return page
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			model := NewModel(Route{Kind: RouteHome})
			var page *captureOverlayPage
			if test.setup != nil {
				page = test.setup(&model)
			}
			model.applyApprovalPoll(approvalPollMsg{requests: []approval.Request{request}})
			beforeRoute, beforeOverlay, beforePalette := model.router.Current(), model.overlay, model.palette
			for _, key := range []tea.KeyPressMsg{{Code: 'p', Mod: tea.ModCtrl}, {Code: 'o', Mod: tea.ModCtrl}, {Code: tea.KeyRight, Mod: tea.ModAlt}, {Code: tea.KeyEscape}} {
				updated, cmd := model.Update(key)
				model = updated.(Model)
				if cmd != nil {
					t.Fatalf("approval key %q escaped with command", key.String())
				}
			}
			if model.router.Current() != beforeRoute || model.overlay != beforeOverlay || model.palette != beforePalette {
				t.Fatalf("approval changed underlying state: route=%#v overlay=%d paletteChanged=%t", model.router.Current(), model.overlay, model.palette != beforePalette)
			}
			if page != nil && len(page.keys) != 0 {
				t.Fatalf("approval leaked keys to page: %v", page.keys)
			}
			if !strings.Contains(ansi.Strip(model.View().Content), request.ID) {
				t.Fatal("approval overlay disappeared")
			}
		})
	}
}

func TestModelApprovalResolvesDirectlyAndAdvancesQueue(t *testing.T) {
	first, second := testPendingApproval("req_first"), testPendingApproval("req_second")
	model := NewModel(Route{Kind: RouteHome})
	model.applyApprovalPoll(approvalPollMsg{requests: []approval.Request{first, second}})
	type resolution struct {
		id      string
		approve bool
		reason  string
	}
	var resolutions []resolution
	model.approvalResolve = func(_ context.Context, id string, approve bool, reason string) (approval.Request, error) {
		resolutions = append(resolutions, resolution{id: id, approve: approve, reason: reason})
		return approval.Request{ID: id}, nil
	}
	updated, cmd := model.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	model = updated.(Model)
	if cmd == nil || model.approvalStage != approvalStageResolving || !model.approvalApprove || len(resolutions) != 0 {
		t.Fatalf("approve did not resolve directly: stage=%d approve=%t cmd=%v resolutions=%v", model.approvalStage, model.approvalApprove, cmd, resolutions)
	}
	updated, follow := model.Update(cmd())
	model = updated.(Model)
	if follow == nil || model.activeApprovalID() != second.ID || model.approvalStage != approvalStageChoice || model.toast.message != "Approved "+first.ID {
		t.Fatalf("queue did not advance: active=%q stage=%d follow=%v", model.activeApprovalID(), model.approvalStage, follow)
	}
	if len(resolutions) != 1 || resolutions[0].id != first.ID || !resolutions[0].approve || resolutions[0].reason != "" {
		t.Fatalf("approval resolution=%v", resolutions)
	}
	updated, _ = model.Update(toastCloseMsg{})
	model = updated.(Model)
	updated, cmd = model.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	model = updated.(Model)
	if cmd == nil || model.approvalStage != approvalStageResolving || model.approvalApprove {
		t.Fatalf("deny did not resolve directly: stage=%d approve=%t cmd=%v", model.approvalStage, model.approvalApprove, cmd)
	}
	updated, _ = model.Update(cmd())
	model = updated.(Model)
	if model.approvalActive() || len(model.approvals) != 0 {
		t.Fatalf("approval queue not cleared: %#v", model.approvals)
	}
	if len(resolutions) != 2 || resolutions[1].id != second.ID || resolutions[1].approve {
		t.Fatalf("deny resolution=%v", resolutions)
	}
}

func TestModelApprovalPollFiltersStatusesAndSurvivesErrors(t *testing.T) {
	pending, resolved := testPendingApproval("req_pending"), testPendingApproval("req_resolved")
	resolved.Status = approval.StatusApproved
	model := NewModel(Route{Kind: RouteHome})
	model.applyApprovalPoll(approvalPollMsg{requests: []approval.Request{resolved, pending}})
	if len(model.approvals) != 1 || model.activeApprovalID() != pending.ID || !model.approvalActive() {
		t.Fatalf("pending filter=%#v", model.approvals)
	}
	model.applyApprovalPoll(approvalPollMsg{err: errors.New("runtime temporarily unavailable")})
	if model.activeApprovalID() != pending.ID || !model.approvalActive() {
		t.Fatal("transient poll error cleared pending approval")
	}
	model.applyApprovalPoll(approvalPollMsg{requests: nil})
	if model.approvalActive() || len(model.approvals) != 0 {
		t.Fatalf("empty successful poll did not clear dialog: %#v", model.approvals)
	}
}

func TestModelApprovalDialogShowsLiveExpiryCountdown(t *testing.T) {
	now := time.Date(2026, 9, 8, 11, 0, 0, 0, time.Local)
	request := testPendingApproval("req_countdown")
	request.ExpiresAt = now.Add(65 * time.Second)
	model := NewModel(Route{Kind: RouteHome})
	model.approvalNow = func() time.Time { return now }
	model.applyApprovalPoll(approvalPollMsg{requests: []approval.Request{request}})
	plain := ansi.Strip(model.approvalDialogView(80))
	if !strings.Contains(plain, "Expires in") || !strings.Contains(plain, "00:01:05") || !strings.Contains(plain, request.ExpiresAt.Local().Format("15:04:05")) {
		t.Fatalf("approval countdown view=%q", plain)
	}
	now = now.Add(5 * time.Second)
	plain = ansi.Strip(model.approvalDialogView(80))
	if !strings.Contains(plain, "00:01:00") {
		t.Fatalf("approval countdown did not advance: %q", plain)
	}
}

func TestModelApprovalTickExpiresRequestAndAdvancesQueue(t *testing.T) {
	now := time.Date(2026, 9, 8, 11, 0, 0, 0, time.Local)
	first, second := testPendingApproval("req_expiring"), testPendingApproval("req_next")
	first.ExpiresAt = now.Add(time.Second)
	second.ExpiresAt = now.Add(time.Minute)
	model := NewModel(Route{Kind: RouteHome})
	model.approvalNow = func() time.Time { return now }
	model.approvalList = func(context.Context) ([]approval.Request, error) { return []approval.Request{second}, nil }
	model.applyApprovalPoll(approvalPollMsg{requests: []approval.Request{first, second}})
	if model.activeApprovalID() != first.ID {
		t.Fatalf("active approval=%q", model.activeApprovalID())
	}
	now = now.Add(time.Second)
	updated, poll := model.Update(approvalPollTickMsg{})
	model = updated.(Model)
	if model.activeApprovalID() != second.ID || model.approvalStage != approvalStageChoice {
		t.Fatalf("expired approval did not advance: active=%q stage=%d approvals=%#v", model.activeApprovalID(), model.approvalStage, model.approvals)
	}
	if poll == nil {
		t.Fatal("expiry tick did not continue runtime poll")
	}
}

func TestModelApprovalCannotResolveAfterExpiry(t *testing.T) {
	now := time.Date(2026, 9, 8, 11, 0, 0, 0, time.Local)
	request := testPendingApproval("req_expired_action")
	request.ExpiresAt = now.Add(time.Second)
	model := NewModel(Route{Kind: RouteHome})
	model.approvalNow = func() time.Time { return now }
	model.applyApprovalPoll(approvalPollMsg{requests: []approval.Request{request}})
	resolved := false
	model.approvalResolve = func(context.Context, string, bool, string) (approval.Request, error) {
		resolved = true
		return approval.Request{}, nil
	}
	now = now.Add(time.Second)
	updated, poll := model.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	model = updated.(Model)
	if resolved || model.approvalActive() || len(model.approvals) != 0 {
		t.Fatalf("expired approval resolved=%t active=%t approvals=%#v", resolved, model.approvalActive(), model.approvals)
	}
	if poll == nil {
		t.Fatal("expired action did not refresh approval state")
	}
}

func TestApprovalCountdownRoundsPositiveRemainderUp(t *testing.T) {
	now := time.Unix(0, 0)
	if got := approvalCountdown(now.Add(1500*time.Millisecond), now); got != "00:00:02" {
		t.Fatalf("countdown=%q", got)
	}
	if got := approvalCountdown(now, now); got != "00:00:00" {
		t.Fatalf("expired countdown=%q", got)
	}
}

func TestModelApprovalOverlayKeepsExactGeometry(t *testing.T) {
	model := NewModel(Route{Kind: RouteHome})
	model.applyApprovalPoll(approvalPollMsg{requests: []approval.Request{testPendingApproval("req_geometry")}})
	for _, size := range [][2]int{{120, 40}, {20, 8}, {3, 3}, {1, 1}} {
		updated, _ := model.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		model = updated.(Model)
		view := model.View().Content
		if width, height := lipgloss.Width(view), lipgloss.Height(view); width != size[0] || height != size[1] {
			t.Fatalf("approval layout=%dx%d want=%dx%d", width, height, size[0], size[1])
		}
	}
}

func TestModelApprovalResolutionErrorKeepsRequestVisible(t *testing.T) {
	request := testPendingApproval("req_error")
	model := NewModel(Route{Kind: RouteHome})
	model.applyApprovalPoll(approvalPollMsg{requests: []approval.Request{request}})
	model.approvalResolve = func(context.Context, string, bool, string) (approval.Request, error) {
		return approval.Request{}, errors.New("resolution failed")
	}
	updated, cmd := model.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	model = updated.(Model)
	updated, follow := model.Update(cmd())
	model = updated.(Model)
	if follow != nil || model.activeApprovalID() != request.ID || model.approvalStage != approvalStageChoice || model.approvalErr == nil {
		t.Fatalf("failed resolution state: active=%q stage=%d follow=%v err=%v", model.activeApprovalID(), model.approvalStage, follow, model.approvalErr)
	}
	if !strings.Contains(ansi.Strip(model.View().Content), "resolution failed") {
		t.Fatal("resolution error not rendered")
	}
}

func testPendingApproval(id string) approval.Request {
	return approval.Request{ID: id, Status: approval.StatusPending, WorkspaceID: "ws_demo", Source: "tunnel", TargetTool: "run_command", Title: "Allow command", Arguments: json.RawMessage(`{"command":"echo hello","workspace_id":"ws_demo"}`)}
}

func TestModelInitPollsPendingApprovals(t *testing.T) {
	request := testPendingApproval("req_init")
	model := NewModel(Route{Kind: RouteHome})
	model.approvalList = func(context.Context) ([]approval.Request, error) { return []approval.Request{request}, nil }
	cmd := model.Init()
	if cmd == nil {
		t.Fatal("model init did not start approval poll")
	}
	message, ok := cmd().(approvalPollMsg)
	if !ok {
		t.Fatalf("init message=%T", cmd())
	}
	updated, tick := model.Update(message)
	model = updated.(Model)
	if !model.approvalActive() || model.activeApprovalID() != request.ID || tick == nil {
		t.Fatalf("init approval state active=%t id=%q tick=%v", model.approvalActive(), model.activeApprovalID(), tick)
	}
}

func TestModelApprovalPollKeepsActiveRequestStableAcrossReorder(t *testing.T) {
	first, second := testPendingApproval("req_first"), testPendingApproval("req_second")
	model := NewModel(Route{Kind: RouteHome})
	model.applyApprovalPoll(approvalPollMsg{requests: []approval.Request{first, second}})
	updated, cmd := model.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	model = updated.(Model)
	if cmd == nil || model.approvalStage != approvalStageResolving {
		t.Fatalf("approval did not begin resolving: stage=%d cmd=%v", model.approvalStage, cmd)
	}
	model.applyApprovalPoll(approvalPollMsg{requests: []approval.Request{second, first}})
	if model.activeApprovalID() != first.ID || model.approvalStage != approvalStageResolving || !model.approvalApprove {
		t.Fatalf("active approval changed after reorder: active=%q stage=%d approve=%t", model.activeApprovalID(), model.approvalStage, model.approvalApprove)
	}
}

func TestModelToastRendersAsDialogAcrossRoutes(t *testing.T) {
	defer configformat.SetRootPath("")
	if err := configformat.SetRootPath(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	for _, route := range []Route{{Kind: RouteWorkspaces}, {Kind: RouteContainers}, {Kind: RouteMCP}, {Kind: RouteTunnel}, {Kind: RouteTunnels}, {Kind: RouteRequests}, {Kind: RouteLogs}, {Kind: RouteConfig}, {Kind: RouteInstruction}, {Kind: RouteRuntime}} {
		t.Run(string(route.Kind), func(t *testing.T) {
			model := NewModel(route)
			updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
			model = updated.(Model)
			updated, dismiss := model.Update(tuipage.ToastMsg{Title: "Update", Message: "toast-inline", Tone: component.ToneSuccess})
			model = updated.(Model)
			if dismiss == nil || model.toast.id == 0 {
				t.Fatal("toast did not schedule dismissal")
			}
			plain := ansi.Strip(model.View().Content)
			for _, want := range []string{"✓ Update", "toast-inline", "Close"} {
				if !strings.Contains(plain, want) {
					t.Fatalf("route %s dialog missing %q: %q", route.Kind, want, plain)
				}
			}
			if strings.Contains(plain, "· toast-inline") || strings.Count(plain, "Close") != 1 {
				t.Fatalf("route %s toast retained inline rendering or multiple close actions: %q", route.Kind, plain)
			}
			if width, height := lipgloss.Width(model.View().Content), lipgloss.Height(model.View().Content); width != 120 || height != 40 {
				t.Fatalf("route %s geometry=%dx%d", route.Kind, width, height)
			}
		})
	}
}

func TestModelToastDismissalDoesNotClearNewerToast(t *testing.T) {
	model := NewModel(Route{Kind: RouteRuntime})
	updated, _ := model.Update(tuipage.ToastMsg{Title: "First", Message: "one", Tone: component.ToneSuccess})
	model = updated.(Model)
	firstID, firstTimer := model.toast.id, model.toast.timer
	updated, _ = model.Update(tuipage.ToastMsg{Title: "Second", Message: "two", Tone: component.ToneWarning})
	model = updated.(Model)
	secondID, secondTimer := model.toast.id, model.toast.timer
	updated, _ = model.Update(toastDismissMsg{id: firstID, timer: firstTimer})
	model = updated.(Model)
	if model.toast.id != secondID || model.toast.message != "two" || pageNotice(model.currentPage) != "" {
		t.Fatalf("stale dismiss cleared newer toast: %#v", model.toast)
	}
	updated, _ = model.Update(toastDismissMsg{id: secondID, timer: secondTimer})
	model = updated.(Model)
	if model.toast.id != 0 || pageNotice(model.currentPage) != "" {
		t.Fatalf("matching dismiss did not clear toast: toast=%#v notice=%q", model.toast, pageNotice(model.currentPage))
	}
}

func TestModelToastKeepsExactGeometryAtTinySizes(t *testing.T) {
	model := NewModel(Route{Kind: RouteRuntime})
	updated, _ := model.Update(tuipage.ToastMsg{Title: "Update", Message: "done", Tone: component.ToneSuccess})
	model = updated.(Model)
	for _, size := range [][2]int{{40, 10}, {20, 6}, {3, 3}, {1, 1}} {
		updated, _ = model.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		model = updated.(Model)
		view := model.View().Content
		if width, height := lipgloss.Width(view), lipgloss.Height(view); width != size[0] || height != size[1] {
			t.Fatalf("toast layout=%dx%d want=%dx%d", width, height, size[0], size[1])
		}
	}
}

func TestModelPageToastAutoDismissDuration(t *testing.T) {
	if toastDuration != 3*time.Second {
		t.Fatalf("toast duration=%s", toastDuration)
	}
	model := NewModel(Route{Kind: RouteHome})
	model.currentPage = &noticeTestPage{}
	updated, dismiss := model.updatePage(noticeTestMsg("Created"))
	model = updated.(Model)
	if dismiss == nil || model.toast.id == 0 || model.toast.message != "Created" || pageNotice(model.currentPage) != "" {
		t.Fatalf("page notice did not start toast timer: toast=%#v notice=%q", model.toast, pageNotice(model.currentPage))
	}
	updated, _ = model.Update(toastDismissMsg{id: model.toast.id, timer: model.toast.timer})
	model = updated.(Model)
	if pageNotice(model.currentPage) != "" || model.toast.id != 0 {
		t.Fatalf("toast did not auto-clear state: toast=%#v notice=%q", model.toast, pageNotice(model.currentPage))
	}
}

func TestModelToastHoverPausesAndRestartsAutoDismiss(t *testing.T) {
	model := NewModel(Route{Kind: RouteRuntime})
	updated, _ := model.Update(tuipage.ToastMsg{Title: "Update", Message: "done", Tone: component.ToneSuccess})
	model = updated.(Model)
	id, initialTimer := model.toast.id, model.toast.timer
	updated, cmd := model.Update(toastHoverMsg{id: id, hovered: true})
	model = updated.(Model)
	if cmd != nil || !model.toast.hovered || model.toast.timer == initialTimer {
		t.Fatalf("hover did not pause timer: toast=%#v cmd=%v", model.toast, cmd)
	}
	updated, _ = model.Update(toastDismissMsg{id: id, timer: initialTimer})
	model = updated.(Model)
	if model.toast.id != id {
		t.Fatal("stale pre-hover timer dismissed toast")
	}
	pausedTimer := model.toast.timer
	updated, cmd = model.Update(toastHoverMsg{id: id, hovered: false})
	model = updated.(Model)
	if cmd == nil || model.toast.hovered || model.toast.timer == pausedTimer {
		t.Fatalf("leaving toast did not restart timer: toast=%#v cmd=%v", model.toast, cmd)
	}
	updated, _ = model.Update(toastDismissMsg{id: id, timer: pausedTimer})
	model = updated.(Model)
	if model.toast.id != id {
		t.Fatal("stale paused timer dismissed toast")
	}
	updated, _ = model.Update(toastDismissMsg{id: id, timer: model.toast.timer})
	model = updated.(Model)
	if model.toast.id != 0 {
		t.Fatalf("matching resumed timer did not dismiss toast: %#v", model.toast)
	}
}

func TestModelToastMouseHoverOutsideAndClose(t *testing.T) {
	model := NewModel(Route{Kind: RouteRuntime})
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	model = updated.(Model)
	updated, _ = model.Update(tuipage.ToastMsg{Title: "Update", Message: "done", Tone: component.ToneSuccess})
	model = updated.(Model)
	dialog := component.NewToastDialog(model.toast.title, model.toast.message, model.toast.tone)
	modalWidth := min(72, model.width-4)
	foreground := component.Modal(dialog.ViewWidth(component.ModalContentWidth(modalWidth)), modalWidth)
	_, x, y := component.CenteredOverlayTargets(foreground, model.width, model.height, 0, 0, 299, toastCloseMsg{})
	view := model.View()
	cmd := view.OnMouse(tea.MouseMotionMsg(tea.Mouse{X: x, Y: y}))
	if cmd == nil {
		t.Fatal("motion inside toast returned no command")
	}
	hover, ok := cmd().(toastHoverMsg)
	if !ok || !hover.hovered {
		t.Fatalf("inside motion=%#v", hover)
	}
	updated, _ = model.Update(hover)
	model = updated.(Model)
	view = model.View()
	cmd = view.OnMouse(tea.MouseMotionMsg(tea.Mouse{X: 0, Y: 0}))
	if cmd == nil {
		t.Fatal("motion outside toast returned no command")
	}
	leave, ok := cmd().(toastHoverMsg)
	if !ok || leave.hovered {
		t.Fatalf("outside motion=%#v", leave)
	}
	view = model.View()
	cmd = view.OnMouse(tea.MouseClickMsg(tea.Mouse{X: 0, Y: 0, Button: tea.MouseLeft}))
	if cmd == nil {
		t.Fatal("outside click returned no command")
	}
	closeMsg := cmd()
	if _, ok := closeMsg.(toastCloseMsg); !ok {
		t.Fatalf("outside click=%#v", closeMsg)
	}
	updated, _ = model.Update(closeMsg)
	model = updated.(Model)
	if model.toast.id != 0 {
		t.Fatalf("outside click did not close toast: %#v", model.toast)
	}
	updated, _ = model.Update(tuipage.ToastMsg{Title: "Update", Message: "done", Tone: component.ToneSuccess})
	model = updated.(Model)
	dialog = component.NewToastDialog(model.toast.title, model.toast.message, model.toast.tone)
	modalWidth = min(72, model.width-4)
	foreground = component.Modal(dialog.ViewWidth(component.ModalContentWidth(modalWidth)), modalWidth)
	_, x, y = component.CenteredOverlayTargets(foreground, model.width, model.height, 0, 0, 299, toastCloseMsg{})
	view = model.View()
	if rect, ok := component.FindRenderedRect(foreground, dialog.CloseButtonView()); ok {
		cmd = view.OnMouse(tea.MouseClickMsg(tea.Mouse{X: x + rect.X, Y: y + rect.Y, Button: tea.MouseLeft}))
		if cmd == nil {
			t.Fatal("close button click returned no command")
		}
		closeMsg = cmd()
		if _, ok := closeMsg.(toastCloseMsg); !ok {
			t.Fatalf("close button click=%#v", closeMsg)
		}
		updated, _ = model.Update(closeMsg)
		model = updated.(Model)
		if model.toast.id != 0 {
			t.Fatalf("close button did not close toast: %#v", model.toast)
		}
	} else {
		t.Fatal("close button rect not found")
	}
}

type noticeTestMsg string

type noticeTestPage struct{ notice string }

func (*noticeTestPage) Init() tea.Cmd { return nil }
func (page *noticeTestPage) Update(message tea.Msg) (tuipage.Model, tea.Cmd) {
	if value, ok := message.(noticeTestMsg); ok {
		page.notice = string(value)
	}
	return page, nil
}
func (page *noticeTestPage) View(width, height int) string {
	return component.PageTitleNotice("Test", page.notice, width)
}
func (*noticeTestPage) OverlayActive() bool         { return false }
func (*noticeTestPage) InputActive() bool           { return false }
func (page *noticeTestPage) Notice() string         { return page.notice }
func (page *noticeTestPage) SetNotice(value string) { page.notice = value }

type statusNoticeTestPage struct{ notice string }

func (*statusNoticeTestPage) Init() tea.Cmd { return nil }
func (page *statusNoticeTestPage) Update(message tea.Msg) (tuipage.Model, tea.Cmd) {
	if value, ok := message.(noticeTestMsg); ok {
		page.notice = string(value)
	}
	return page, nil
}
func (page *statusNoticeTestPage) View(width, height int) string {
	return component.PageTitleNotice("Test", page.notice, width)
}
func (*statusNoticeTestPage) OverlayActive() bool         { return false }
func (*statusNoticeTestPage) InputActive() bool           { return false }
func (page *statusNoticeTestPage) Notice() string         { return page.notice }
func (page *statusNoticeTestPage) SetNotice(value string) { page.notice = value }
func (*statusNoticeTestPage) ShouldToastNotice() bool     { return false }

func TestModelKeepsStatusOnlyNoticeOutOfToastDialog(t *testing.T) {
	model := NewModel(Route{Kind: RouteHome})
	model.currentPage = &statusNoticeTestPage{}
	updated, cmd := model.updatePage(noticeTestMsg("Live stream disconnected; reconnecting"))
	model = updated.(Model)
	if cmd != nil || model.toast.id != 0 || pageNotice(model.currentPage) == "" {
		t.Fatalf("status notice promoted to toast: toast=%#v notice=%q cmd=%v", model.toast, pageNotice(model.currentPage), cmd)
	}
}

func TestApprovalDialogWrapsLongArgumentsWithoutTruncation(t *testing.T) {
	token := strings.Repeat("z", 72)
	request := testPendingApproval("req_" + token)
	request.Title = "Run " + token
	request.WorkspaceID = "ws_" + token
	request.Arguments = json.RawMessage(`{"command":"` + token + `","cwd":"/very/long/` + token + `"}`)
	model := NewModel(Route{Kind: RouteHome})
	model.applyApprovalPoll(approvalPollMsg{requests: []approval.Request{request}})
	view := model.approvalDialogView(24)
	for _, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(line); got > 24 {
			t.Fatalf("approval line width=%d want <=24: %q", got, ansi.Strip(line))
		}
	}
	plain := ansi.Strip(view)
	if strings.Count(plain, "z") < len(token)*5 {
		t.Fatalf("approval content was truncated: %q", plain)
	}
}
