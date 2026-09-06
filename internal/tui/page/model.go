package page

import tea "charm.land/bubbletea/v2"

type Model interface {
	Init() tea.Cmd
	Update(tea.Msg) (Model, tea.Cmd)
	View(width, height int) string
	OverlayActive() bool
}

type NavigateMsg struct {
	Path []string
}
