package component

import (
	"errors"
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

func TestFormEscapeClosesCleanFormAndConfirmsDirtyForm(t *testing.T) {
	name := "demo"
	form := NewForm(Group(Input("Name", &name)))
	form = runFormCmd(t, form, form.Init())
	updated, cmd := form.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if updated.ConfirmingExit() || cmd == nil {
		t.Fatalf("clean escape confirming=%t cmd=%v", updated.ConfirmingExit(), cmd)
	}
	if _, ok := cmd().(FormCancelledMsg); !ok {
		t.Fatalf("clean escape message=%T", cmd())
	}

	name = "changed"
	updated, cmd = form.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	form = updated
	if cmd != nil || !form.ConfirmingExit() || !strings.Contains(ansi.Strip(form.View()), "Discard changes?") {
		t.Fatalf("dirty escape confirming=%t cmd=%v view=%q", form.ConfirmingExit(), cmd, ansi.Strip(form.View()))
	}
	form, _ = form.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if form.ConfirmingExit() || name != "changed" {
		t.Fatalf("escape from discard confirm confirming=%t name=%q", form.ConfirmingExit(), name)
	}
	form, _ = form.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	form.exitConfirm.Select(true)
	form, cmd = form.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("discard confirmation returned no cancellation command")
	}
	if _, ok := cmd().(FormCancelledMsg); !ok {
		t.Fatalf("discard confirmation message=%T", cmd())
	}
}

func TestFormValidationKeepsFocusOnInvalidField(t *testing.T) {
	value := ""
	field := Input("Required", &value).Validate(func(value string) error {
		if strings.TrimSpace(value) == "" {
			return errors.New("value is required")
		}
		return nil
	})
	form := NewForm(Group(field))
	form = runFormCmd(t, form, form.Init())
	updated, cmd := form.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	form = updated
	form = runFormCmd(t, form, cmd)
	if form.State() == huh.StateCompleted {
		t.Fatal("invalid form completed")
	}
	if form.model.GetFocusedField() != field {
		t.Fatalf("validation moved focus to %T", form.model.GetFocusedField())
	}
	if !strings.Contains(ansi.Strip(form.View()), "value is required") {
		t.Fatalf("validation error not visible: %q", ansi.Strip(form.View()))
	}
}

func TestSwitchUsesCompactBooleanStateAndSpaceWithoutBlockingNavigation(t *testing.T) {
	enabled, id := true, ""
	switchField := Switch("Enabled", &enabled)
	idField := Input("Tunnel ID", &id)
	form := NewForm(Group(switchField, idField))
	form = runFormCmd(t, form, form.Init())
	plain := ansi.Strip(form.View())
	if !strings.Contains(plain, "Enabled [ TRUE ]") || strings.Contains(plain, "[ FALSE ]") || strings.Contains(plain, "y Yes") || strings.Contains(plain, "n No") {
		t.Fatalf("initial switch view=%q", plain)
	}
	form, _ = form.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	plain = ansi.Strip(form.View())
	if enabled || !strings.Contains(plain, "Enabled [ FALSE ]") || strings.Contains(plain, "[ TRUE ]") {
		t.Fatalf("toggled enabled=%t view=%q", enabled, plain)
	}
	updated, cmd := form.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	form = updated
	queue := []tea.Cmd{cmd}
	for steps := 0; steps < 32 && form.model.GetFocusedField() != idField && len(queue) > 0; steps++ {
		next := queue[0]
		queue = queue[1:]
		if next == nil {
			continue
		}
		message := next()
		if batch, ok := message.(tea.BatchMsg); ok {
			queue = append(queue, batch...)
			continue
		}
		form, cmd = form.Update(message)
		if cmd != nil {
			queue = append(queue, cmd)
		}
	}
	if form.model.GetFocusedField() != idField {
		t.Fatalf("switch blocked navigation, focused=%T", form.model.GetFocusedField())
	}
}

func TestPageActionBarWrapsByGroupWithoutDroppingDisabledActions(t *testing.T) {
	groups := [][]ActionHint{
		{{Key: "e", Label: "Configure", Enabled: true}, {Key: "space", Label: "Toggle", Enabled: true}, {Key: "s", Label: "Sync", Enabled: true}},
		{{Key: "a", Label: "Admin key", Enabled: true}, {Key: "v", Label: "Verify", Enabled: false}, {Key: "d", Label: "Remove admin", Enabled: false, Danger: true}},
		{{Key: "m", Label: "Managed tunnels", Enabled: true}},
	}
	wide := ansi.Strip(PageActionBar(120, groups...))
	if !strings.Contains(wide, "│") {
		t.Fatalf("wide action bar has no group separator: %q", wide)
	}
	narrow := ansi.Strip(PageActionBar(54, groups...))
	for _, action := range []string{"e Configure", "space Toggle", "s Sync", "a Admin key", "v Verify", "d Remove admin", "m Managed tunnels"} {
		if !strings.Contains(narrow, action) {
			t.Fatalf("narrow action bar dropped %q: %q", action, narrow)
		}
	}
	lines := strings.Split(narrow, "\n")
	if len(lines) < 3 || !strings.Contains(lines[0], "e Configure") || !strings.Contains(lines[0], "s Sync") || !strings.Contains(lines[len(lines)-1], "m Managed tunnels") {
		t.Fatalf("narrow groups were not wrapped intact: %q", lines)
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
