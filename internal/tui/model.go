package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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

func (model Model) Init() tea.Cmd { return nil }

func (model Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.BackgroundColorMsg:
		model.theme = newTheme(msg.IsDark())
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
			updated, cmd := model.currentPage.Update(tea.WindowSizeMsg{Width: max(20, msg.Width-8), Height: max(10, msg.Height-10)})
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
			return model, nil
		}
		model.closeOverlay()
		model.recordRecent(msg.ID)
		cmd, err := model.actions.Execute(model.ctx, msg.ID, actionContext(model.router.Current()))
		if err != nil {
			model.notice = err.Error()
			return model, nil
		}
		return model, cmd
	case navigateMsg:
		model.navigate(msg.route)
	case tuipage.NavigateMsg:
		route, err := ParseRoute(msg.Path)
		if err != nil {
			model.notice = err.Error()
			return model, nil
		}
		model.navigate(route)
		return model, nil
	case tuipage.WorkspaceCommandMsg:
		if err := model.ensureWorkspacePage(msg.Command, msg.ResourceID); err != nil {
			model.notice = err.Error()
			return model, nil
		}
		return model.updatePage(msg)
	case component.FormSubmittedMsg, component.FormCancelledMsg:
		return model.updatePage(msg)
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
		case "esc", "backspace":
			if model.router.Back() {
				model.loadPage(model.router.Current())
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
	content := model.render()
	if model.palette != nil {
		content = centerOverlay(content, model.palette.View(min(78, max(48, model.width-8))), model.width, model.height)
	}
	view := tea.NewView(content)
	view.AltScreen = true
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
	value := message.String()
	return value == "ctrl+shift+p" || value == "ctrl+p" || value == ":"
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
	}
	if err != nil {
		model.notice = err.Error()
		return
	}
	model.currentPage = value
	if model.currentPage != nil && model.width > 0 && model.height > 0 {
		updated, _ := model.currentPage.Update(tea.WindowSizeMsg{Width: max(20, model.width-8), Height: max(10, model.height-10)})
		model.currentPage = updated
	}
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

func (model Model) render() string {
	width, height := model.width, model.height
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	contentWidth := min(92, max(48, width-8))
	body := strings.Join([]string{
		model.header(contentWidth),
		model.divider(contentWidth),
		model.page(contentWidth),
		model.divider(contentWidth),
		model.theme.muted.Render(model.shortcutFooter()),
	}, "\n")
	panel := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(model.theme.border.GetBorderLeftForeground()).Padding(1, 2).Width(contentWidth).Render(body)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, panel)
}

func (model Model) shortcutFooter() string {
	parts := make([]string, 0, 10)
	for _, item := range model.actions.Actions(actionContext(model.router.Current())) {
		help := item.Shortcut.Help()
		if help.Key != "" && help.Desc != "" {
			parts = append(parts, help.Key+" "+help.Desc)
		}
	}
	sort.Strings(parts)
	parts = append(parts, "Esc Back", "q Quit")
	parts = append([]string{"Ctrl+Shift+P Commands", "Ctrl+O Open"}, parts...)
	return strings.Join(parts, "  ·  ")
}

func (model Model) header(width int) string {
	left := model.theme.title.Render("ChatGPT MCP")
	right := model.theme.muted.Render(model.router.Current().Title())
	gap := max(2, width-lipgloss.Width(left)-lipgloss.Width(right)-4)
	return left + strings.Repeat(" ", gap) + right
}

func (model Model) page(width int) string {
	if model.currentPage != nil {
		return model.currentPage.View(width, max(10, model.height-10))
	}
	route := model.router.Current()
	title := model.theme.current.Render(route.Title())
	description := routeDescription(route)
	notice := ""
	if model.notice != "" {
		notice = "\n\n" + model.theme.muted.Render(model.notice)
	}
	return "\n" + title + "\n\n" + model.theme.muted.Render(description) + notice + "\n\n" + model.theme.subtle.Render("Command Center shell is ready. Domain actions will be added through the shared action registry.") + "\n"
}

func (model Model) divider(width int) string {
	return model.theme.subtle.Render(strings.Repeat("─", max(1, width-4)))
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
