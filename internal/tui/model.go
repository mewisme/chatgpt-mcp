package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type Model struct {
	router Router
	theme  theme
	width  int
	height int
}

func NewModel(initial Route) Model {
	return Model{router: NewRouter(initial), theme: newTheme(true)}
}

func (model Model) Init() tea.Cmd { return nil }

func (model Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.BackgroundColorMsg:
		model.theme = newTheme(msg.IsDark())
	case tea.WindowSizeMsg:
		model.width, model.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return model, tea.Quit
		case "esc", "backspace":
			model.router.Back()
		case "1":
			model.router.Navigate(Route{Kind: RouteWorkspaces})
		case "2":
			model.router.Navigate(Route{Kind: RouteMCP})
		case "3":
			model.router.Navigate(Route{Kind: RouteTunnel})
		case "4":
			model.router.Navigate(Route{Kind: RouteRequests})
		case "5":
			model.router.Navigate(Route{Kind: RouteLogs})
		case "6":
			model.router.Navigate(Route{Kind: RouteConfig})
		case "7":
			model.router.Navigate(Route{Kind: RouteRuntime})
		}
	}
	return model, nil
}

func (model Model) View() tea.View {
	view := tea.NewView(model.render())
	view.AltScreen = true
	return view
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
		model.theme.muted.Render("1 Workspaces  2 MCP  3 Tunnel  4 Requests  5 Logs  6 Config  7 Runtime  ·  Esc Back  ·  q Quit"),
	}, "\n")
	panel := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(model.theme.border.GetBorderLeftForeground()).Padding(1, 2).Width(contentWidth).Render(body)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, panel)
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
