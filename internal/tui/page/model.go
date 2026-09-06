package page

import (
	tea "charm.land/bubbletea/v2"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
)

type Model interface {
	Init() tea.Cmd
	Update(tea.Msg) (Model, tea.Cmd)
	View(width, height int) string
	OverlayActive() bool
	InputActive() bool
}

type NavigateMsg struct {
	Path []string
}

type ToastMsg struct {
	Title   string
	Message string
	Tone    component.Tone
}
