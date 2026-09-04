package interactive

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestConfirmButtonsToggleAndRender(t *testing.T) {
	buttons := NewConfirmButtons("Allow", "Deny", true)
	buttons.SetWidth(40)
	if !buttons.AffirmativeSelected() {
		t.Fatal("affirmative should be selected initially")
	}
	view := buttons.View()
	if !strings.Contains(view, "Allow") || !strings.Contains(view, "Deny") {
		t.Fatalf("view=%q", view)
	}
	buttons.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyRight}))
	if buttons.AffirmativeSelected() {
		t.Fatal("right did not select negative button")
	}
	buttons.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	if !buttons.AffirmativeSelected() {
		t.Fatal("tab did not toggle button selection")
	}
}
