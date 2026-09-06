package component

import (
	"testing"

	"charm.land/huh/v2"
)

func TestFormHelpersBindValuesAndPasswordMode(t *testing.T) {
	name, secret, mode := "demo", "token", "http"
	nameField := Input("Name", &name)
	password := PasswordInput("Secret", &secret)
	selectField := Select("Mode", &mode, huh.NewOption("HTTP", "http"), huh.NewOption("stdio", "stdio"))
	form := NewForm(huh.NewGroup(nameField, password, selectField))
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
