package page

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
)

func overlayWidth(pageWidth, preferred int) int {
	return max(1, min(preferred, pageWidth-4))
}

func mouseBlocker(originX, originY, width, height, z int) component.MouseTarget {
	return component.MouseTarget{ID: "page.overlay", Rect: component.Rect{X: originX, Y: originY, Width: width, Height: height}, Z: z, Handle: func(component.MouseEvent) tea.Msg { return nil }}
}

func formOverlayMouseTargets(form component.Form, modalWidth, pageWidth, pageHeight, originX, originY, z int) []component.MouseTarget {
	formView := form.View()
	modal := component.Modal(formView, modalWidth)
	modalX := max(0, (pageWidth-lipgloss.Width(modal))/2)
	modalY := max(0, (pageHeight-lipgloss.Height(modal))/2)
	rect, ok := component.FindRenderedRect(modal, formView)
	if !ok {
		return []component.MouseTarget{mouseBlocker(originX, originY, pageWidth, pageHeight, z)}
	}
	targets := []component.MouseTarget{mouseBlocker(originX, originY, pageWidth, pageHeight, z)}
	return append(targets, form.MouseTargets(originX+modalX+rect.X, originY+modalY+rect.Y, z+1)...)
}

func confirmOverlayMouseTargets(confirm component.ConfirmButtons, title, description string, modalWidth, pageWidth, pageHeight, originX, originY, z int) []component.MouseTarget {
	body := component.Title(title) + "\n\n" + component.Muted(description) + "\n\n" + confirm.View() + "\n" + component.Muted("Enter confirm · Esc cancel")
	modal := component.Modal(body, modalWidth)
	modalX := max(0, (pageWidth-lipgloss.Width(modal))/2)
	modalY := max(0, (pageHeight-lipgloss.Height(modal))/2)
	rect, ok := component.FindRenderedRect(modal, confirm.View())
	targets := []component.MouseTarget{mouseBlocker(originX, originY, pageWidth, pageHeight, z)}
	if !ok {
		return targets
	}
	return append(targets, confirm.MouseTargets(originX+modalX+rect.X, originY+modalY+rect.Y, z+1)...)
}

func keyHintMouseTargets(view string, bindings map[string]string, originX, originY, z int) []component.MouseTarget {
	targets := make([]component.MouseTarget, 0, len(bindings))
	for label, key := range bindings {
		if strings.TrimSpace(label) == "" || key == "" {
			continue
		}
		rect, ok := component.FindRenderedRect(view, label)
		if !ok {
			continue
		}
		keyValue := key
		targets = append(targets, component.MouseTarget{
			ID: "page.action", Rect: component.Rect{X: originX + rect.X, Y: originY + rect.Y, Width: rect.Width, Height: 1}, Z: z,
			Handle: func(event component.MouseEvent) tea.Msg {
				if event.Button != tea.MouseLeft {
					return nil
				}
				return pageActionKeyMsg(keyValue)
			},
		})
	}
	return targets
}

func pageActionKeyMsg(value string) tea.KeyPressMsg {
	switch value {
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace}
	default:
		runes := []rune(value)
		if len(runes) == 0 {
			return tea.KeyPressMsg{}
		}
		return tea.KeyPressMsg{Code: runes[0]}
	}
}
