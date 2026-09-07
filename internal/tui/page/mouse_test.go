package page

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
)

func TestFormOverlayMouseTargetsDismissOutsideAndShieldInside(t *testing.T) {
	value := ""
	form := component.NewForm(component.Group(component.Input("Name", &value)))
	form = runMouseForm(form, form.Init())
	targets := formOverlayMouseTargets(form, 60, 100, 30, 2, 3, 20)
	if len(targets) < 3 || targets[0].ID != "overlay.backdrop" || targets[1].ID != "overlay.modal" {
		t.Fatalf("targets=%#v", targets)
	}
	if targets[0].Z >= targets[1].Z || targets[1].Z >= targets[2].Z {
		t.Fatalf("z ordering=%d,%d,%d", targets[0].Z, targets[1].Z, targets[2].Z)
	}
	cmd := component.DispatchMouse(targets, tea.MouseClickMsg(tea.Mouse{X: 2, Y: 3, Button: tea.MouseLeft}))
	if cmd == nil {
		t.Fatal("outside click did not dismiss modal")
	}
	if message, ok := cmd().(tea.KeyPressMsg); !ok || message.String() != "esc" {
		t.Fatalf("outside dismiss message=%#v", cmd())
	}
	insideX, insideY := targets[1].Rect.X, targets[1].Rect.Y
	if cmd := component.DispatchMouse(targets, tea.MouseClickMsg(tea.Mouse{X: insideX, Y: insideY, Button: tea.MouseLeft})); cmd != nil {
		t.Fatal("inside modal click incorrectly dismissed")
	}
}

func runMouseForm(form component.Form, cmd tea.Cmd) component.Form {
	if cmd == nil {
		return form
	}
	message := cmd()
	if batch, ok := message.(tea.BatchMsg); ok {
		for _, next := range batch {
			form = runMouseForm(form, next)
		}
		return form
	}
	updated, next := form.Update(message)
	return runMouseForm(updated, next)
}

func TestKeyHintMouseTargetsKeepSpaceShortcut(t *testing.T) {
	view := "e Configure · Space Enable/Disable · s Sync"
	targets := keyHintMouseTargets(view, map[string]string{"Enable/Disable": " "}, 0, 0, 1)
	if len(targets) != 1 {
		t.Fatalf("targets=%d want=1", len(targets))
	}
	message, ok := targets[0].Handle(component.MouseEvent{Button: tea.MouseLeft}).(tea.KeyPressMsg)
	if !ok || message.String() != "space" {
		t.Fatalf("space mouse message=%#v", message)
	}
}
