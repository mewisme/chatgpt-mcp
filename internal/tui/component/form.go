package component

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type FormSubmittedMsg struct{}
type FormCancelledMsg struct{}

type FormMouseMsg struct {
	Group  int
	Field  int
	Line   int
	Choice int
	Click  bool
}

type FormGroup struct {
	group  *huh.Group
	fields []huh.Field
	hide   func() bool
}

func Group(fields ...huh.Field) FormGroup {
	return FormGroup{group: huh.NewGroup(fields...), fields: append([]huh.Field(nil), fields...)}
}

func (group FormGroup) WithHideFunc(hide func() bool) FormGroup {
	group.hide = hide
	group.group.WithHideFunc(hide)
	return group
}

func (group FormGroup) Title(title string) FormGroup {
	group.group.Title(title)
	return group
}

func (group FormGroup) Description(description string) FormGroup {
	group.group.Description(description)
	return group
}

type Form struct {
	model  *huh.Form
	groups []FormGroup
}

func NewForm(groups ...FormGroup) Form {
	huhGroups := make([]*huh.Group, 0, len(groups))
	for _, group := range groups {
		huhGroups = append(huhGroups, group.group)
	}
	model := huh.NewForm(huhGroups...).WithTheme(huh.ThemeFunc(func(isDark bool) *huh.Styles { return huh.ThemeCharm(isDark) })).WithShowHelp(true)
	model.SubmitCmd = func() tea.Msg { return FormSubmittedMsg{} }
	model.CancelCmd = func() tea.Msg { return FormCancelledMsg{} }
	return Form{model: model, groups: append([]FormGroup(nil), groups...)}
}

func (form Form) Init() tea.Cmd {
	if form.model == nil {
		return nil
	}
	return form.model.Init()
}

func (form Form) Update(message tea.Msg) (Form, tea.Cmd) {
	if form.model == nil {
		return form, nil
	}
	if msg, ok := message.(FormMouseMsg); ok {
		return form.updateMouse(msg)
	}
	updated, cmd := form.model.Update(message)
	if value, ok := updated.(*huh.Form); ok {
		form.model = value
	}
	return form, cmd
}

func (form Form) MouseTargets(originX, originY, z int) []MouseTarget {
	if form.model == nil {
		return nil
	}
	activeGroup := form.activeGroup()
	if activeGroup < 0 || activeGroup >= len(form.groups) {
		return nil
	}
	view := ansi.Strip(form.View())
	lines := strings.Split(view, "\n")
	group := form.groups[activeGroup]
	targets := make([]MouseTarget, 0, len(group.fields)+1)
	searchLine := 0
	for fieldIndex, field := range group.fields {
		if field == nil || field.Skip() {
			continue
		}
		fieldView := ansi.Strip(field.View())
		needle := firstNonEmptyLine(fieldView)
		if needle == "" {
			continue
		}
		lineIndex, column := findRenderedLine(lines, needle, searchLine)
		if lineIndex < 0 {
			continue
		}
		height := max(1, lipgloss.Height(fieldView))
		width := max(1, lipgloss.Width(fieldView))
		groupIndex, index := activeGroup, fieldIndex
		targets = append(targets, MouseTarget{
			ID:   "form.field",
			Rect: Rect{X: originX + column, Y: originY + lineIndex, Width: width, Height: height},
			Z:    z,
			Handle: func(event MouseEvent) tea.Msg {
				if event.Button != tea.MouseLeft {
					return nil
				}
				return FormMouseMsg{Group: groupIndex, Field: index, Line: event.Y, Click: true}
			},
		})
		if _, ok := field.(*huh.Confirm); ok {
			for choice, label := range map[int]string{1: "Yes", 2: "No"} {
				rect, ok := FindRenderedRect(fieldView, label)
				if !ok {
					continue
				}
				selectedChoice := choice
				targets = append(targets, MouseTarget{
					ID: "form.confirm", Rect: Rect{X: originX + column + rect.X, Y: originY + lineIndex + rect.Y, Width: rect.Width, Height: 1}, Z: z + 1,
					Handle: func(event MouseEvent) tea.Msg {
						if event.Button != tea.MouseLeft {
							return nil
						}
						return FormMouseMsg{Group: groupIndex, Field: index, Choice: selectedChoice, Click: true}
					},
				})
			}
		}
		searchLine = lineIndex
	}
	return targets
}

func (form Form) activeGroup() int {
	if form.model == nil {
		return -1
	}
	focused := form.model.GetFocusedField()
	for groupIndex, group := range form.groups {
		if group.hide != nil && group.hide() {
			continue
		}
		for _, field := range group.fields {
			if field == focused {
				return groupIndex
			}
		}
	}
	for groupIndex, group := range form.groups {
		if group.hide == nil || !group.hide() {
			return groupIndex
		}
	}
	return -1
}

func (form Form) updateMouse(msg FormMouseMsg) (Form, tea.Cmd) {
	if msg.Group < 0 || msg.Group >= len(form.groups) || msg.Field < 0 || msg.Field >= len(form.groups[msg.Group].fields) {
		return form, nil
	}
	target := form.groups[msg.Group].fields[msg.Field]
	for steps := 0; steps < 128 && form.model.GetFocusedField() != target; steps++ {
		updated, cmd := form.model.Update(huh.NextField())
		if value, ok := updated.(*huh.Form); ok {
			form.model = value
		}
		if cmd != nil {
			return form, cmd
		}
	}
	if !msg.Click {
		return form, nil
	}
	if confirmField, ok := target.(*huh.Confirm); ok && msg.Choice != 0 {
		want := msg.Choice == 1
		current, _ := confirmField.GetValue().(bool)
		if current == want {
			return form, nil
		}
		updated, cmd := form.model.Update(tea.KeyPressMsg{Code: tea.KeyRight})
		if value, ok := updated.(*huh.Form); ok {
			form.model = value
		}
		return form, cmd
	}
	if selectField, ok := target.(*huh.Select[string]); ok {
		return form, form.clickSelect(selectField, msg.Line, false)
	}
	if multiField, ok := target.(*huh.MultiSelect[string]); ok {
		return form, form.clickMultiSelect(multiField, msg.Line)
	}
	if _, ok := target.(*huh.Confirm); ok {
		updated, cmd := form.model.Update(tea.KeyPressMsg{Code: tea.KeyRight})
		if value, ok := updated.(*huh.Form); ok {
			form.model = value
		}
		return form, cmd
	}
	return form, nil
}

func (form *Form) clickSelect(field *huh.Select[string], line int, multi bool) tea.Cmd {
	current := hoveredLine(field.View())
	if current < 0 || line <= 0 {
		return nil
	}
	for step := current; step != line; {
		code := tea.KeyDown
		if line < step {
			code = tea.KeyUp
			step--
		} else {
			step++
		}
		updated, _ := form.model.Update(tea.KeyPressMsg{Code: code})
		if value, ok := updated.(*huh.Form); ok {
			form.model = value
		}
	}
	code := tea.KeyEnter
	if multi {
		code = tea.KeySpace
	}
	updated, cmd := form.model.Update(tea.KeyPressMsg{Code: code})
	if value, ok := updated.(*huh.Form); ok {
		form.model = value
	}
	return cmd
}

func (form *Form) clickMultiSelect(field *huh.MultiSelect[string], line int) tea.Cmd {
	current := hoveredLine(field.View())
	if current < 0 || line <= 0 {
		return nil
	}
	for step := current; step != line; {
		code := tea.KeyDown
		if line < step {
			code = tea.KeyUp
			step--
		} else {
			step++
		}
		updated, _ := form.model.Update(tea.KeyPressMsg{Code: code})
		if value, ok := updated.(*huh.Form); ok {
			form.model = value
		}
	}
	updated, cmd := form.model.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if value, ok := updated.(*huh.Form); ok {
		form.model = value
	}
	return cmd
}

func hoveredLine(value string) int {
	for index, line := range strings.Split(ansi.Strip(value), "\n") {
		if strings.Contains(line, "> ") {
			return index
		}
	}
	return -1
}

func firstNonEmptyLine(value string) string {
	for _, line := range strings.Split(value, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}

func findRenderedLine(lines []string, needle string, start int) (int, int) {
	for index := max(0, start); index < len(lines); index++ {
		if column := strings.Index(lines[index], needle); column >= 0 {
			return index, lipgloss.Width(lines[index][:column])
		}
	}
	return -1, -1
}

func (form Form) View() string {
	if form.model == nil {
		return ""
	}
	return form.model.View()
}

func (form Form) State() huh.FormState {
	if form.model == nil {
		return huh.StateAborted
	}
	return form.model.State
}

func Input(title string, value *string) *huh.Input {
	return huh.NewInput().Title(strings.TrimSpace(title)).Value(value)
}

func PasswordInput(title string, value *string) *huh.Input {
	return Input(title, value).EchoMode(huh.EchoModePassword)
}

func Text(title string, value *string) *huh.Text {
	return huh.NewText().Title(strings.TrimSpace(title)).Value(value).Lines(4)
}

func Select[T comparable](title string, value *T, options ...huh.Option[T]) *huh.Select[T] {
	return huh.NewSelect[T]().Title(strings.TrimSpace(title)).Options(options...).Value(value)
}

func MultiSelect[T comparable](title string, value *[]T, options ...huh.Option[T]) *huh.MultiSelect[T] {
	return huh.NewMultiSelect[T]().Title(strings.TrimSpace(title)).Options(options...).Value(value)
}

func Confirm(title string, value *bool) *huh.Confirm {
	return huh.NewConfirm().Title(strings.TrimSpace(title)).Value(value)
}
