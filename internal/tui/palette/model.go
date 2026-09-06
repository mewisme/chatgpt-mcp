package palette

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"go.mewis.me/chatgpt-mcp/internal/tui/action"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
)

const maxVisibleResults = 9

type SelectedMsg struct{ ID string }
type ClosedMsg struct{}
type MouseScrollMsg struct{ Delta int }

type Options struct {
	Title       string
	Hint        string
	Placeholder string
	Footer      string
	Recent      []string
}

type Model struct {
	input    textinput.Model
	actions  []action.Action
	results  []Result
	context  action.Context
	options  Options
	selected int
	isDark   bool
}

func New(actions []action.Action, ctx action.Context) Model {
	return NewWithOptions(actions, ctx, Options{})
}

func NewWithOptions(actions []action.Action, ctx action.Context, options Options) Model {
	if strings.TrimSpace(options.Title) == "" {
		options.Title = "Command Palette"
	}
	if strings.TrimSpace(options.Hint) == "" {
		options.Hint = "Ctrl+P"
	}
	if strings.TrimSpace(options.Placeholder) == "" {
		options.Placeholder = "Type a command"
	}
	if strings.TrimSpace(options.Footer) == "" {
		options.Footer = "↑/↓ navigate  ·  Enter run  ·  Esc close"
	}
	input := textinput.New()
	input.Prompt = "> "
	input.Placeholder = options.Placeholder
	input.CharLimit = 160
	input.SetStyles(textinput.DefaultStyles(true))
	input.Focus()
	model := Model{input: input, actions: append([]action.Action(nil), actions...), context: ctx, options: options, isDark: true}
	model.refresh()
	return model
}

func (model *Model) SetActions(actions []action.Action, ctx action.Context) {
	if model == nil {
		return
	}
	model.actions = append([]action.Action(nil), actions...)
	model.context = ctx
	model.refresh()
}

func (model *Model) SetQuery(value string) {
	if model == nil {
		return
	}
	model.input.SetValue(value)
	model.refresh()
}

func (model Model) Query() string { return model.input.Value() }

func (model Model) SelectedID() string {
	if model.selected < 0 || model.selected >= len(model.results) {
		return ""
	}
	return model.results[model.selected].Action.ID
}

func (model Model) Update(message tea.Msg) (Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.BackgroundColorMsg:
		model.isDark = msg.IsDark()
		model.input.SetStyles(textinput.DefaultStyles(model.isDark))
		return model, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "esc":
			return model, func() tea.Msg { return ClosedMsg{} }
		case "enter":
			if id := model.SelectedID(); id != "" {
				return model, func() tea.Msg { return SelectedMsg{ID: id} }
			}
			return model, nil
		case "up":
			if len(model.results) > 0 {
				model.selected = (model.selected - 1 + len(model.results)) % len(model.results)
			}
			return model, nil
		case "down":
			if len(model.results) > 0 {
				model.selected = (model.selected + 1) % len(model.results)
			}
			return model, nil
		}
	case MouseScrollMsg:
		if len(model.results) > 0 {
			model.selected = (model.selected + msg.Delta + len(model.results)) % len(model.results)
		}
		return model, nil
	}
	previous := model.input.Value()
	updated, cmd := model.input.Update(message)
	model.input = updated
	if model.input.Value() != previous {
		model.selected = 0
		model.refresh()
	}
	return model, cmd
}

func (model Model) MouseTargets(originX, originY, z, width int) []component.MouseTarget {
	view := model.View(width)
	renderedWidth, renderedHeight := lipgloss.Width(view), lipgloss.Height(view)
	targets := []component.MouseTarget{{
		ID: "palette.scroll", Rect: component.Rect{X: originX, Y: originY, Width: renderedWidth, Height: renderedHeight}, Z: z,
		Handle: func(event component.MouseEvent) tea.Msg {
			switch event.Button {
			case tea.MouseWheelUp:
				return MouseScrollMsg{Delta: -1}
			case tea.MouseWheelDown:
				return MouseScrollMsg{Delta: 1}
			default:
				return nil
			}
		},
	}}
	lines := strings.Split(ansi.Strip(view), "\n")
	start := 0
	visible := model.results
	if len(visible) > maxVisibleResults {
		start = max(0, min(model.selected-maxVisibleResults/2, len(visible)-maxVisibleResults))
		visible = visible[start : start+maxVisibleResults]
	}
	searchLine := 0
	for _, result := range visible {
		item := result.Action
		title := item.Title
		if item.Category != "" {
			title = item.Category + ": " + item.Title
		}
		line, column := findPaletteLine(lines, title, searchLine)
		if line < 0 {
			continue
		}
		id := item.ID
		targets = append(targets, component.MouseTarget{
			ID: "palette.result", Rect: component.Rect{X: originX + max(0, column-2), Y: originY + line, Width: max(1, renderedWidth-max(0, column-2)-2), Height: 1}, Z: z + 1,
			Handle: func(event component.MouseEvent) tea.Msg {
				if event.Button != tea.MouseLeft {
					return nil
				}
				return SelectedMsg{ID: id}
			},
		})
		searchLine = line + 1
	}
	return targets
}

func findPaletteLine(lines []string, needle string, start int) (int, int) {
	for index := max(0, start); index < len(lines); index++ {
		if column := strings.Index(lines[index], needle); column >= 0 {
			return index, column
		}
	}
	return -1, -1
}

func (model Model) View(width int) string {
	if width <= 0 {
		width = 72
	}
	width = max(42, min(78, width))
	contentWidth := max(32, width-6)
	input := model.input
	input.SetWidth(contentWidth - 2)
	listStyles := list.DefaultStyles(model.isDark)
	huhStyles := huh.ThemeCharm(model.isDark)
	titleStyle := huhStyles.Focused.Title
	mutedStyle := huhStyles.Focused.Description
	selectedStyle := lipgloss.NewStyle().Foreground(huhStyles.Focused.SelectSelector.GetForeground()).Bold(true)
	borderColor := huhStyles.Focused.Base.GetBorderLeftForeground()
	var builder strings.Builder
	builder.WriteString(titleStyle.Render(model.options.Title))
	builder.WriteString("\n")
	builder.WriteString(mutedStyle.Render(model.options.Hint))
	builder.WriteString("\n\n")
	builder.WriteString(input.View())
	builder.WriteString("\n")
	builder.WriteString(listStyles.NoItems.Render(strings.Repeat("─", contentWidth)))
	builder.WriteString("\n")
	visible := model.results
	if len(visible) > maxVisibleResults {
		start := max(0, min(model.selected-maxVisibleResults/2, len(visible)-maxVisibleResults))
		visible = visible[start : start+maxVisibleResults]
	}
	if len(visible) == 0 {
		builder.WriteString("\n")
		builder.WriteString(listStyles.NoItems.Render("No matching commands"))
		builder.WriteString("\n")
	} else {
		for _, result := range visible {
			item := result.Action
			prefix := "  "
			lineStyle := lipgloss.NewStyle()
			if item.ID == model.SelectedID() {
				prefix = selectedStyle.Render(">") + " "
				lineStyle = selectedStyle
			}
			title := item.Title
			if item.Category != "" {
				title = item.Category + ": " + item.Title
			}
			help := item.Shortcut.Help()
			right := ""
			if help.Key != "" {
				right = mutedStyle.Render(help.Key)
			}
			gap := max(2, contentWidth-lipgloss.Width(prefix)-lipgloss.Width(title)-lipgloss.Width(right))
			builder.WriteString(prefix + lineStyle.Render(title) + strings.Repeat(" ", gap) + right + "\n")
		}
	}
	if id := model.SelectedID(); id != "" {
		for _, result := range model.results {
			if result.Action.ID == id && result.Action.Description != "" {
				builder.WriteString("\n")
				builder.WriteString(mutedStyle.Render(result.Action.Description))
				break
			}
		}
	}
	builder.WriteString("\n\n")
	builder.WriteString(mutedStyle.Render(model.options.Footer))
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(borderColor).Padding(1, 2).Width(width).Render(builder.String())
}

func (model *Model) refresh() {
	model.results = RankWithRecent(model.actions, model.input.Value(), model.context, model.options.Recent)
	if len(model.results) == 0 {
		model.selected = 0
	} else if model.selected >= len(model.results) {
		model.selected = len(model.results) - 1
	}
}

func (model Model) DebugResults() []string {
	result := make([]string, 0, len(model.results))
	for _, item := range model.results {
		result = append(result, fmt.Sprintf("%s:%d", item.Action.ID, item.Score))
	}
	return result
}
