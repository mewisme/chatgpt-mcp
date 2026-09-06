package component

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type Rect struct {
	X      int
	Y      int
	Width  int
	Height int
}

func (rect Rect) Contains(x, y int) bool {
	return rect.Width > 0 && rect.Height > 0 && x >= rect.X && x < rect.X+rect.Width && y >= rect.Y && y < rect.Y+rect.Height
}

type MouseEvent struct {
	X      int
	Y      int
	Button tea.MouseButton
}

type MouseTarget struct {
	ID     string
	Rect   Rect
	Z      int
	Handle func(MouseEvent) tea.Msg
}

func DispatchMouse(targets []MouseTarget, message tea.MouseMsg) tea.Cmd {
	mouse := message.Mouse()
	if mouse.Button != tea.MouseLeft && mouse.Button != tea.MouseWheelUp && mouse.Button != tea.MouseWheelDown {
		return nil
	}
	index := -1
	for i := range targets {
		if !targets[i].Rect.Contains(mouse.X, mouse.Y) {
			continue
		}
		if index < 0 || targets[i].Z >= targets[index].Z {
			index = i
		}
	}
	if index < 0 || targets[index].Handle == nil {
		return nil
	}
	target := targets[index]
	event := MouseEvent{X: mouse.X - target.Rect.X, Y: mouse.Y - target.Rect.Y, Button: mouse.Button}
	result := target.Handle(event)
	if result == nil {
		return nil
	}
	return func() tea.Msg { return result }
}

func OffsetMouseTargets(targets []MouseTarget, x, y, z int) []MouseTarget {
	result := make([]MouseTarget, len(targets))
	for index, target := range targets {
		target.Rect.X += x
		target.Rect.Y += y
		target.Z += z
		result[index] = target
	}
	return result
}

func FindRenderedRect(container, child string) (Rect, bool) {
	containerLines := strings.Split(ansi.Strip(container), "\n")
	childPlain := ansi.Strip(child)
	needle := firstRenderedLine(childPlain)
	if needle == "" {
		return Rect{}, false
	}
	for y, line := range containerLines {
		if x := strings.Index(line, needle); x >= 0 {
			return Rect{X: lipgloss.Width(line[:x]), Y: y, Width: max(1, lipgloss.Width(childPlain)), Height: max(1, lipgloss.Height(childPlain))}, true
		}
	}
	return Rect{}, false
}

func firstRenderedLine(value string) string {
	for _, line := range strings.Split(value, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}
