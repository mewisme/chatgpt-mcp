package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"go.mewis.me/chatgpt-mcp/internal/tui/action"
	"go.mewis.me/chatgpt-mcp/internal/tui/palette"
)

type Model struct {
	ctx     context.Context
	router  Router
	actions *action.Registry
	palette *palette.Model
	theme   theme
	width   int
	height  int
}

func NewModel(initial Route) Model {
	return NewModelWithContext(context.Background(), initial)
}

func NewModelWithContext(ctx context.Context, initial Route) Model {
	if ctx == nil {
		ctx = context.Background()
	}
	return Model{ctx: ctx, router: NewRouter(initial), actions: defaultActionRegistry(), theme: newTheme(true)}
}

func (model Model) Init() tea.Cmd { return nil }

func (model Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.BackgroundColorMsg:
		model.theme = newTheme(msg.IsDark())
		if model.palette != nil {
			updated, cmd := model.palette.Update(msg)
			model.palette = &updated
			return model, cmd
		}
	case tea.WindowSizeMsg:
		model.width, model.height = msg.Width, msg.Height
	case palette.ClosedMsg:
		model.palette = nil
		return model, nil
	case palette.SelectedMsg:
		model.palette = nil
		cmd, err := model.actions.Execute(model.ctx, msg.ID, actionContext(model.router.Current()))
		if err != nil {
			return model, nil
		}
		return model, cmd
	case navigateMsg:
		model.router.Navigate(msg.route)
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
		switch msg.String() {
		case "ctrl+c", "q":
			return model, tea.Quit
		case "esc", "backspace":
			model.router.Back()
		default:
			if selected, ok := model.actions.MatchShortcut(msg, actionContext(model.router.Current())); ok {
				cmd, err := model.actions.Execute(model.ctx, selected.ID, actionContext(model.router.Current()))
				if err == nil {
					return model, cmd
				}
			}
		}
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
	value := palette.New(model.actions.Actions(context), context)
	model.palette = &value
}

func isPaletteKey(message tea.KeyPressMsg) bool {
	value := message.String()
	return value == "ctrl+shift+p" || value == "ctrl+p" || value == ":"
}

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
	return strings.Join(parts, "  ·  ")
}

func (model Model) header(width int) string {
	left := model.theme.title.Render("ChatGPT MCP")
	right := model.theme.muted.Render(model.router.Current().Title())
	gap := max(2, width-lipgloss.Width(left)-lipgloss.Width(right)-4)
	return left + strings.Repeat(" ", gap) + right
}

func (model Model) page(width int) string {
	route := model.router.Current()
	title := model.theme.current.Render(route.Title())
	description := routeDescription(route)
	return "\n" + title + "\n\n" + model.theme.muted.Render(description) + "\n\n" + model.theme.subtle.Render("Command Center shell is ready. Domain actions will be added through the shared action registry.") + "\n"
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
