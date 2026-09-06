package tui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"go.mewis.me/chatgpt-mcp/internal/tui/action"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
	tuipage "go.mewis.me/chatgpt-mcp/internal/tui/page"
	"go.mewis.me/chatgpt-mcp/internal/tui/palette"
	"go.mewis.me/chatgpt-mcp/internal/tui/quickopen"
	tuistate "go.mewis.me/chatgpt-mcp/internal/tui/state"
)

type overlayKind uint8

const (
	overlayNone overlayKind = iota
	overlayCommands
	overlayQuickOpen
)

const minTerminalWidth = 56
const minTerminalHeight = 24

type Model struct {
	ctx            context.Context
	router         Router
	actions        *action.Registry
	palette        *palette.Model
	overlay        overlayKind
	quickResources map[string]quickopen.Resource
	stateRoot      string
	state          tuistate.State
	notice         string
	currentPage    tuipage.Model
	theme          theme
	width          int
	height         int
}

func NewModel(initial Route) Model {
	return NewModelWithContext(context.Background(), initial)
}

func NewModelWithContext(ctx context.Context, initial Route) Model {
	return NewModelWithState(ctx, initial, "")
}

func NewModelWithState(ctx context.Context, initial Route, root string) Model {
	if ctx == nil {
		ctx = context.Background()
	}
	state := tuistate.Default()
	if root != "" {
		if loaded, err := tuistate.Load(root); err == nil {
			state = loaded
		}
	}
	model := Model{ctx: ctx, router: NewRouter(initial), actions: defaultActionRegistry(), stateRoot: root, state: state, theme: newTheme(true)}
	model.loadPage(initial)
	return model
}

func (model Model) Init() tea.Cmd {
	if model.currentPage != nil {
		return model.currentPage.Init()
	}
	return nil
}

func (model Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.BackgroundColorMsg:
		model.theme = newTheme(msg.IsDark())
		component.SetDarkBackground(msg.IsDark())
		if model.currentPage != nil {
			updated, cmd := model.currentPage.Update(msg)
			model.currentPage = updated
			if model.palette == nil && cmd != nil {
				return model, cmd
			}
		}
		if model.palette != nil {
			updated, cmd := model.palette.Update(msg)
			model.palette = &updated
			return model, cmd
		}
	case tea.WindowSizeMsg:
		model.width, model.height = msg.Width, msg.Height
		if model.currentPage != nil {
			updated, cmd := model.currentPage.Update(tea.WindowSizeMsg{Width: max(20, msg.Width-4), Height: max(10, msg.Height-6)})
			model.currentPage = updated
			if cmd != nil {
				return model, cmd
			}
		}
	case palette.ClosedMsg:
		model.closeOverlay()
		return model, nil
	case palette.SelectedMsg:
		if model.overlay == overlayQuickOpen {
			resource, ok := model.quickResources[msg.ID]
			model.closeOverlay()
			if !ok {
				model.notice = "Quick Open resource is no longer available"
				return model, nil
			}
			route, err := ParseRoute(resource.Path)
			if err != nil {
				model.notice = err.Error()
				return model, nil
			}
			model.router.Navigate(route)
			model.loadPage(route)
			return model, model.initCurrentPage()
		}
		model.closeOverlay()
		model.recordRecent(msg.ID)
		cmd, err := model.actions.Execute(model.ctx, msg.ID, actionContext(model.router.Current()))
		if err != nil {
			model.notice = err.Error()
			return model, nil
		}
		return model, cmd
	case palette.MouseScrollMsg:
		if model.palette != nil {
			updated, cmd := model.palette.Update(msg)
			model.palette = &updated
			return model, cmd
		}
		return model, nil
	case navigateMsg:
		if msg.sibling {
			model.switchPage(msg.route)
		} else {
			model.navigate(msg.route)
		}
		return model, model.initCurrentPage()
	case tuipage.NavigateMsg:
		route, err := ParseRoute(msg.Path)
		if err != nil {
			model.notice = err.Error()
			return model, nil
		}
		model.navigate(route)
		return model, model.initCurrentPage()
	case tuipage.WorkspaceCommandMsg:
		if err := model.ensureWorkspacePage(msg.Command, msg.ResourceID); err != nil {
			model.notice = err.Error()
			return model, nil
		}
		return model.updatePage(msg)
	case tuipage.MCPCommandMsg:
		if err := model.ensureMCPPage(msg.ResourceID); err != nil {
			model.notice = err.Error()
			return model, nil
		}
		return model.updatePage(msg)
	case tuipage.TunnelCommandMsg:
		if err := model.ensureTunnelPage(msg.Command, msg.ResourceID); err != nil {
			model.notice = err.Error()
			return model, nil
		}
		return model.updatePage(msg)
	case tuipage.RequestCommandMsg:
		if err := model.ensureRequestPage(msg.ResourceID); err != nil {
			model.notice = err.Error()
			return model, nil
		}
		return model.updatePage(msg)
	case tuipage.ConfigCommandMsg:
		if err := model.ensureConfigPage(); err != nil {
			model.notice = err.Error()
			return model, nil
		}
		return model.updatePage(msg)
	case component.FormSubmittedMsg, component.FormCancelledMsg:
		return model.updatePage(msg)
	case tea.MouseClickMsg, tea.MouseReleaseMsg, tea.MouseWheelMsg, tea.MouseMotionMsg:
		return model, nil
	case tea.KeyPressMsg:
		if model.palette != nil {
			updated, cmd := model.palette.Update(msg)
			model.palette = &updated
			return model, cmd
		}
		if isPaletteKey(msg) {
			model.openPalette()
			return model, nil
		}
		if isQuickOpenKey(msg) {
			model.openQuickOpen()
			return model, nil
		}
		if model.currentPage != nil && model.currentPage.OverlayActive() && msg.String() != "ctrl+c" {
			return model.updatePage(msg)
		}
		switch msg.String() {
		case "ctrl+c", "q":
			return model, tea.Quit
		case "alt+left":
			model.switchPage(cycleHeaderRoute(model.router.Current(), -1))
			return model, model.initCurrentPage()
		case "alt+right":
			model.switchPage(cycleHeaderRoute(model.router.Current(), 1))
			return model, model.initCurrentPage()
		case "esc", "backspace":
			if model.router.Back() {
				model.loadPage(model.router.Current())
				return model, model.initCurrentPage()
			}
		default:
			if selected, ok := model.actions.MatchShortcut(msg, actionContext(model.router.Current())); ok {
				cmd, err := model.actions.Execute(model.ctx, selected.ID, actionContext(model.router.Current()))
				if err == nil {
					return model, cmd
				}
			}
		}
	}
	if model.currentPage != nil {
		return model.updatePage(message)
	}
	return model, nil
}

func (model Model) View() tea.View {
	content, targets := model.render()
	if model.palette != nil {
		width, height := model.layoutSize()
		paletteWidth := min(78, max(48, width-8))
		foreground := model.palette.View(paletteWidth)
		x, y := max(0, (width-lipgloss.Width(foreground))/2), max(0, (height-lipgloss.Height(foreground))/2)
		content = centerOverlay(content, foreground, width, height)
		targets = append(targets, model.palette.MouseTargets(x, y, 100, paletteWidth)...)
	}
	view := tea.NewView(content)
	view.AltScreen = true
	view.MouseMode = tea.MouseModeCellMotion
	view.OnMouse = func(message tea.MouseMsg) tea.Cmd { return component.DispatchMouse(targets, message) }
	return view
}

func (model *Model) openPalette() {
	if model == nil {
		return
	}
	context := actionContext(model.router.Current())
	value := palette.NewWithOptions(model.actions.Actions(context), context, palette.Options{Recent: model.state.RecentActions})
	model.palette = &value
	model.overlay = overlayCommands
	model.quickResources = nil
}

func isPaletteKey(message tea.KeyPressMsg) bool {
	return message.String() == "ctrl+p"
}

func (model *Model) openQuickOpen() {
	if model == nil {
		return
	}
	resources, err := loadQuickOpenResources()
	if err != nil {
		model.notice = err.Error()
		return
	}
	actions, index := quickopen.Actions(resources)
	context := actionContext(model.router.Current())
	value := palette.NewWithOptions(actions, context, palette.Options{Title: "Quick Open", Hint: "Ctrl+O", Placeholder: "Search pages and resources", Footer: "↑/↓ navigate  ·  Enter open  ·  Esc close"})
	model.palette = &value
	model.overlay = overlayQuickOpen
	model.quickResources = index
}

func (model *Model) closeOverlay() {
	if model == nil {
		return
	}
	model.palette = nil
	model.overlay = overlayNone
	model.quickResources = nil
}

func (model *Model) recordRecent(id string) {
	if model == nil {
		return
	}
	tuistate.RecordRecent(&model.state, id)
	if model.stateRoot != "" {
		if err := tuistate.Save(model.stateRoot, model.state); err != nil {
			model.notice = "TUI state: " + err.Error()
		}
	}
}

func (model *Model) navigate(route Route) {
	if model == nil {
		return
	}
	model.router.Navigate(route)
	model.loadPage(route)
}

func (model *Model) switchPage(route Route) {
	if model == nil {
		return
	}
	model.router.Switch(route)
	model.loadPage(route)
}

func (model *Model) loadPage(route Route) {
	if model == nil {
		return
	}
	model.currentPage = nil
	var value tuipage.Model
	var err error
	switch route.Kind {
	case RouteWorkspaces:
		value, err = tuipage.NewWorkspaces(model.ctx, route.ResourceID)
	case RouteContainers:
		value, err = tuipage.NewContainers(model.ctx, route.ResourceID)
	case RouteMCP:
		value, err = tuipage.NewMCP(model.ctx, route.ResourceID)
	case RouteTunnel:
		value, err = tuipage.NewTunnelDashboard(model.ctx)
	case RouteTunnels:
		value, err = tuipage.NewManagedTunnels(model.ctx, route.ResourceID)
	case RouteRequests:
		value, err = tuipage.NewRequests(model.ctx, route.ResourceID)
	case RouteConfig:
		value, err = tuipage.NewConfig(model.ctx)
	}
	if err != nil {
		model.notice = err.Error()
		return
	}
	model.currentPage = value
	if model.currentPage != nil && model.width > 0 && model.height > 0 {
		updated, _ := model.currentPage.Update(tea.WindowSizeMsg{Width: max(20, model.width-4), Height: max(10, model.height-6)})
		model.currentPage = updated
	}
}

func (model Model) initCurrentPage() tea.Cmd {
	if model.currentPage == nil {
		return nil
	}
	return model.currentPage.Init()
}

func (model *Model) ensureMCPPage(resourceID string) error {
	if model.router.Current().Kind != RouteMCP || (resourceID != "" && model.router.Current().ResourceID != resourceID) {
		model.navigate(Route{Kind: RouteMCP, ResourceID: resourceID})
	}
	if model.currentPage == nil {
		return fmt.Errorf("MCP page is unavailable")
	}
	return nil
}

func (model *Model) ensureTunnelPage(command tuipage.TunnelCommand, resourceID string) error {
	managed := command == tuipage.TunnelManagedRefresh || command == tuipage.TunnelManagedCreate || command == tuipage.TunnelManagedUpdate || command == tuipage.TunnelManagedConfigure || command == tuipage.TunnelManagedDelete
	kind := RouteTunnel
	if managed {
		kind = RouteTunnels
	}
	if model.router.Current().Kind != kind || (resourceID != "" && model.router.Current().ResourceID != resourceID) {
		model.navigate(Route{Kind: kind, ResourceID: resourceID})
	}
	if model.currentPage == nil {
		return fmt.Errorf("tunnel page is unavailable")
	}
	return nil
}

func (model *Model) ensureRequestPage(resourceID string) error {
	if model.router.Current().Kind != RouteRequests || (resourceID != "" && model.router.Current().ResourceID != resourceID) {
		model.navigate(Route{Kind: RouteRequests, ResourceID: resourceID})
	}
	if model.currentPage == nil {
		return fmt.Errorf("approval inbox is unavailable")
	}
	return nil
}

func (model *Model) ensureConfigPage() error {
	if model.router.Current().Kind != RouteConfig {
		model.navigate(Route{Kind: RouteConfig})
	}
	if model.currentPage == nil {
		return fmt.Errorf("config center is unavailable")
	}
	return nil
}

func (model Model) updatePage(message tea.Msg) (tea.Model, tea.Cmd) {
	if model.currentPage == nil {
		return model, nil
	}
	updated, cmd := model.currentPage.Update(message)
	model.currentPage = updated
	return model, cmd
}

func (model *Model) ensureWorkspacePage(command tuipage.WorkspaceCommand, resourceID string) error {
	container := command == tuipage.WorkspaceContainerCreate || command == tuipage.WorkspaceContainerRename || command == tuipage.WorkspaceContainerDelete || command == tuipage.WorkspaceContainerMembers
	kind := RouteWorkspaces
	if container {
		kind = RouteContainers
	}
	if model.router.Current().Kind != kind || (resourceID != "" && model.router.Current().ResourceID != resourceID) {
		model.navigate(Route{Kind: kind, ResourceID: resourceID})
	}
	if model.currentPage == nil {
		return fmt.Errorf("workspace page is unavailable")
	}
	return nil
}

func isQuickOpenKey(message tea.KeyPressMsg) bool { return message.String() == "ctrl+o" }

type mousePage interface {
	MouseTargets(originX, originY, z int) []component.MouseTarget
}

func (model Model) render() (string, []component.MouseTarget) {
	width, height := model.layoutSize()
	contentWidth := max(48, width-4)
	pageHeight := max(1, height-6)
	header, targets := model.header(contentWidth, 2, 1)
	body := fitFrameContent(model.page(contentWidth, pageHeight), contentWidth, pageHeight)
	footer := fitFrameLine(model.shortcutFooter(), contentWidth)
	border := lipgloss.NewStyle().Foreground(model.theme.border.GetBorderLeftForeground())
	lines := make([]string, 0, height)
	lines = append(lines, model.topBorder(width, border))
	lines = append(lines, frameLine(header, contentWidth, border))
	lines = append(lines, frameDivider(width, border))
	for _, line := range body {
		lines = append(lines, frameLine(line, contentWidth, border))
	}
	lines = append(lines, frameDivider(width, border))
	lines = append(lines, frameLine(footer, contentWidth, border))
	lines = append(lines, border.Render("╰"+strings.Repeat("─", width-2)+"╯"))
	if page, ok := model.currentPage.(mousePage); ok {
		targets = append(targets, page.MouseTargets(2, 3, 10)...)
	}
	return strings.Join(lines, "\n"), targets
}

func (model Model) topBorder(width int, border lipgloss.Style) string {
	label := " " + model.theme.title.Render("ChatGPT MCP") + " "
	used := 2 + lipgloss.Width(label) + 1
	return border.Render("╭─") + label + border.Render(strings.Repeat("─", max(0, width-used))+"╮")
}

func (model Model) shortcutFooter() string {
	width, _ := model.layoutSize()
	width = max(48, width-4)
	bindings := []key.Binding{
		component.Binding([]string{"ctrl+p"}, "ctrl+p", "commands"),
		component.Binding([]string{"ctrl+o"}, "ctrl+o", "open"),
		component.Binding([]string{"alt+left", "alt+right"}, "alt+←/→", "pages"),
	}
	if len(model.router.stack) > 1 {
		bindings = append(bindings, component.Binding([]string{"esc"}, "esc", "back"))
	}
	bindings = append(bindings, component.Binding([]string{"q"}, "q", "quit"))
	return component.DefaultHelp(width, bindings...)
}

func (model Model) header(width, originX, originY int) (string, []component.MouseTarget) {
	owner := headerOwner(model.router.Current().Kind)
	parts := make([]string, 0, len(headerPages))
	targets := make([]component.MouseTarget, 0, len(headerPages))
	x := 0
	for index, page := range headerPages {
		style := model.theme.navInactive
		if page.Kind == owner {
			style = model.theme.navActive
		}
		cellWidth := width / len(headerPages)
		if index < width%len(headerPages) {
			cellWidth++
		}
		label := ansi.Truncate(page.Label, max(1, cellWidth), "")
		button := style.Padding(0).Width(cellWidth).Align(lipgloss.Center).Render(label)
		kind := page.Kind
		targets = append(targets, component.MouseTarget{
			ID: "app.header." + string(kind), Rect: component.Rect{X: originX + x, Y: originY, Width: cellWidth, Height: 1}, Z: 1,
			Handle: func(event component.MouseEvent) tea.Msg {
				if event.Button != tea.MouseLeft {
					return nil
				}
				return navigateMsg{route: Route{Kind: kind}, sibling: true}
			},
		})
		parts = append(parts, button)
		x += cellWidth
	}
	return fitFrameLine(strings.Join(parts, ""), width), targets
}

func (model Model) page(width, height int) string {
	if model.currentPage != nil {
		return model.currentPage.View(width, height)
	}
	route := model.router.Current()
	description := routeDescription(route)
	notice := ""
	if model.notice != "" {
		notice = "\n\n" + model.theme.muted.Render(model.notice)
	}
	return component.PageTitle(route.Title(), width) + "\n" + model.theme.muted.Render(description) + notice + "\n\n" + model.theme.subtle.Render("Command Center shell is ready. Domain actions will be added through the shared action registry.")
}

func (model Model) layoutSize() (int, int) {
	width, height := model.width, model.height
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	return max(minTerminalWidth, width), max(minTerminalHeight, height)
}

func frameLine(content string, width int, border lipgloss.Style) string {
	return border.Render("│") + " " + fitFrameLine(content, width) + " " + border.Render("│")
}

func frameDivider(width int, border lipgloss.Style) string {
	return border.Render("├" + strings.Repeat("─", width-2) + "┤")
}

func fitFrameContent(content string, width, height int) []string {
	lines := strings.Split(content, "\n")
	result := make([]string, height)
	for index := range result {
		if index < len(lines) {
			result[index] = fitFrameLine(lines[index], width)
		} else {
			result[index] = strings.Repeat(" ", width)
		}
	}
	return result
}

func fitFrameLine(line string, width int) string {
	line = ansi.Truncate(line, width, "")
	return line + strings.Repeat(" ", max(0, width-lipgloss.Width(line)))
}

func routeDescription(route Route) string {
	if route.ResourceID != "" {
		return fmt.Sprintf("Deep-linked resource: %s", route.ResourceID)
	}
	switch route.Kind {
	case RouteHome:
		return "Keyboard-first command center for chatgpt-mcp."
	case RouteWorkspaces:
		return "Browse registered workspaces and workspace containers."
	case RouteContainers:
		return "Browse workspace containers and membership."
	case RouteMCP:
		return "Manage configured upstream MCP servers."
	case RouteTunnel:
		return "Manage runtime and OpenAI Secure MCP Tunnel state."
	case RouteTunnels:
		return "Browse and manage tunnels available through the OpenAI Tunnel Management API."
	case RouteRequests:
		return "Review control approval requests."
	case RouteLogs:
		return "Inspect runtime history and live events."
	case RouteConfig:
		return "Browse and manage validated runtime configuration."
	case RouteRuntime:
		return "Inspect and control the local managed runtime."
	case RouteAbout:
		return "Build and runtime information."
	default:
		return ""
	}
}
