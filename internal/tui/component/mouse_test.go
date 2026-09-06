package component

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestDispatchMouseUsesHighestZAndLocalCoordinates(t *testing.T) {
	lowCalled := false
	targets := []MouseTarget{
		{ID: "low", Rect: Rect{X: 10, Y: 4, Width: 10, Height: 5}, Z: 1, Handle: func(MouseEvent) tea.Msg { lowCalled = true; return "low" }},
		{ID: "high", Rect: Rect{X: 12, Y: 5, Width: 4, Height: 2}, Z: 9, Handle: func(event MouseEvent) tea.Msg { return event }},
	}
	cmd := DispatchMouse(targets, tea.MouseClickMsg(tea.Mouse{X: 14, Y: 6, Button: tea.MouseLeft}))
	if cmd == nil {
		t.Fatal("dispatch returned no command")
	}
	event, ok := cmd().(MouseEvent)
	if !ok || event.X != 2 || event.Y != 1 || event.Button != tea.MouseLeft || lowCalled {
		t.Fatalf("event=%#v ok=%t lowCalled=%t", event, ok, lowCalled)
	}
}

func TestDispatchMouseIgnoresUnsupportedButtonsAndOutsideTargets(t *testing.T) {
	target := MouseTarget{Rect: Rect{X: 1, Y: 1, Width: 2, Height: 2}, Z: 1, Handle: func(MouseEvent) tea.Msg { return "hit" }}
	if cmd := DispatchMouse([]MouseTarget{target}, tea.MouseClickMsg(tea.Mouse{X: 10, Y: 10, Button: tea.MouseLeft})); cmd != nil {
		t.Fatal("outside click unexpectedly dispatched")
	}
	if cmd := DispatchMouse([]MouseTarget{target}, tea.MouseClickMsg(tea.Mouse{X: 1, Y: 1, Button: tea.MouseRight})); cmd != nil {
		t.Fatal("right click unexpectedly dispatched")
	}
}
