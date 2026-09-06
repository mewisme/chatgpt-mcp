package component

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestFormHelpersBindValuesAndPasswordMode(t *testing.T) {
	name, secret, mode := "demo", "token", "http"
	nameField := Input("Name", &name)
	password := PasswordInput("Secret", &secret)
	selectField := Select("Mode", &mode, huh.NewOption("HTTP", "http"), huh.NewOption("stdio", "stdio"))
	form := NewForm(Group(nameField, password, selectField))
	if form.State() != huh.StateNormal || form.View() == "" {
		t.Fatalf("form state=%v view=%q", form.State(), form.View())
	}
	if nameField.GetValue() != "demo" || password.GetValue() != "token" || selectField.GetValue() != "http" {
		t.Fatalf("bound values changed: %v %v %v", nameField.GetValue(), password.GetValue(), selectField.GetValue())
	}
}

func TestFormMessagesAreDistinct(t *testing.T) {
	if _, ok := any(FormSubmittedMsg{}).(FormCancelledMsg); ok {
		t.Fatal("form messages unexpectedly overlap")
	}
}

func TestFormMouseSelectAndConfirmUseHuhState(t *testing.T) {
	name, mode, confirmed := "demo", "a", false
	nameField := Input("Name", &name)
	selectField := Select("Mode", &mode, huh.NewOption("Alpha", "a"), huh.NewOption("Beta", "b"), huh.NewOption("Gamma", "c"))
	confirmField := Confirm("Proceed", &confirmed)
	form := NewForm(Group(nameField, selectField, confirmField))
	form = runFormCmd(t, form, form.Init())
	targets := form.MouseTargets(10, 5, 3)

	selectTarget := formFieldTarget(t, targets, 1)
	betaLine, _ := findRenderedLine(strings.Split(ansi.Strip(selectField.View()), "\n"), "Beta", 0)
	message := selectTarget.Handle(MouseEvent{Y: betaLine, Button: tea.MouseLeft})
	updated, _ := form.Update(message)
	form = updated
	if mode != "b" || form.model.GetFocusedField() == nameField {
		t.Fatalf("mode=%q focused=%T", mode, form.model.GetFocusedField())
	}

	targets = form.MouseTargets(10, 5, 3)
	yes := formConfirmTarget(t, targets, 1)
	updated, _ = form.Update(yes.Handle(MouseEvent{Button: tea.MouseLeft}))
	form = updated
	if !confirmed {
		t.Fatal("Yes click did not set confirmation true")
	}
	targets = form.MouseTargets(10, 5, 3)
	no := formConfirmTarget(t, targets, 2)
	updated, _ = form.Update(no.Handle(MouseEvent{Button: tea.MouseLeft}))
	form = updated
	if confirmed {
		t.Fatal("No click did not set confirmation false")
	}
}

func TestFormMouseMultiSelectTogglesClickedOption(t *testing.T) {
	values := []string{}
	field := MultiSelect("Tools", &values, huh.NewOption("Alpha", "a"), huh.NewOption("Beta", "b"), huh.NewOption("Gamma", "c"))
	form := NewForm(Group(field))
	form = runFormCmd(t, form, form.Init())
	target := formFieldTarget(t, form.MouseTargets(0, 0, 1), 0)
	betaLine, _ := findRenderedLine(strings.Split(ansi.Strip(field.View()), "\n"), "Beta", 0)
	_, _ = form.Update(target.Handle(MouseEvent{Y: betaLine, Button: tea.MouseLeft}))
	if len(values) != 1 || values[0] != "b" {
		t.Fatalf("multi-select values=%v", values)
	}
}

func formFieldTarget(t *testing.T, targets []MouseTarget, field int) MouseTarget {
	t.Helper()
	for _, target := range targets {
		if target.ID != "form.field" {
			continue
		}
		msg, ok := target.Handle(MouseEvent{Button: tea.MouseLeft}).(FormMouseMsg)
		if ok && msg.Field == field {
			return target
		}
	}
	t.Fatalf("form field target %d not found", field)
	return MouseTarget{}
}

func formConfirmTarget(t *testing.T, targets []MouseTarget, choice int) MouseTarget {
	t.Helper()
	for _, target := range targets {
		if target.ID != "form.confirm" {
			continue
		}
		msg, ok := target.Handle(MouseEvent{Button: tea.MouseLeft}).(FormMouseMsg)
		if ok && msg.Choice == choice {
			return target
		}
	}
	t.Fatalf("form confirm choice %d not found", choice)
	return MouseTarget{}
}

func runFormCmd(t *testing.T, form Form, cmd tea.Cmd) Form {
	t.Helper()
	if cmd == nil {
		return form
	}
	message := cmd()
	if batch, ok := message.(tea.BatchMsg); ok {
		for _, next := range batch {
			form = runFormCmd(t, form, next)
		}
		return form
	}
	updated, next := form.Update(message)
	return runFormCmd(t, updated, next)
}
