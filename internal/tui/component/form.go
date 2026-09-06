package component

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

type FormSubmittedMsg struct{}
type FormCancelledMsg struct{}

type Form struct {
	model *huh.Form
}

func NewForm(groups ...*huh.Group) Form {
	model := huh.NewForm(groups...).WithTheme(huh.ThemeFunc(func(isDark bool) *huh.Styles { return huh.ThemeCharm(isDark) })).WithShowHelp(true)
	model.SubmitCmd = func() tea.Msg { return FormSubmittedMsg{} }
	model.CancelCmd = func() tea.Msg { return FormCancelledMsg{} }
	return Form{model: model}
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
	updated, cmd := form.model.Update(message)
	if value, ok := updated.(*huh.Form); ok {
		form.model = value
	}
	return form, cmd
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

func Select[T comparable](title string, value *T, options ...huh.Option[T]) *huh.Select[T] {
	return huh.NewSelect[T]().Title(strings.TrimSpace(title)).Options(options...).Value(value)
}

func MultiSelect[T comparable](title string, value *[]T, options ...huh.Option[T]) *huh.MultiSelect[T] {
	return huh.NewMultiSelect[T]().Title(strings.TrimSpace(title)).Options(options...).Value(value)
}

func Confirm(title string, value *bool) *huh.Confirm {
	return huh.NewConfirm().Title(strings.TrimSpace(title)).Value(value)
}
