package tui

import (
	"charm.land/bubbles/v2/list"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

type theme struct {
	title   lipgloss.Style
	accent  lipgloss.Style
	muted   lipgloss.Style
	subtle  lipgloss.Style
	border  lipgloss.Style
	current lipgloss.Style
}

func newTheme(isDark bool) theme {
	listStyles := list.DefaultStyles(isDark)
	huhStyles := huh.ThemeCharm(isDark)
	accent := lipgloss.NewStyle().Foreground(huhStyles.Focused.SelectSelector.GetForeground())
	return theme{
		title: huhStyles.Focused.Title, accent: accent, muted: huhStyles.Focused.Description,
		subtle: lipgloss.NewStyle().Foreground(listStyles.NoItems.GetForeground()), border: huhStyles.Focused.Base,
		current: accent.Bold(true),
	}
}
