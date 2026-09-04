package interactive

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

type ConfirmButtons struct {
	field *huh.Confirm
	value *bool
}

func NewConfirmButtons(affirmative, negative string, affirmativeSelected bool) ConfirmButtons {
	value := new(bool)
	*value = affirmativeSelected
	field := huh.NewConfirm().Affirmative(strings.TrimSpace(affirmative)).Negative(strings.TrimSpace(negative)).Value(value).Inline(true)
	keymap := huh.NewDefaultKeyMap()
	keymap.Confirm.Toggle = key.NewBinding(key.WithKeys("h", "l", "left", "right", "tab", "shift+tab"), key.WithHelp("←/→", "choose"))
	keymap.Confirm.Next.SetEnabled(false)
	keymap.Confirm.Prev.SetEnabled(false)
	keymap.Confirm.Submit.SetEnabled(false)
	keymap.Confirm.Accept.SetEnabled(false)
	keymap.Confirm.Reject.SetEnabled(false)
	field.WithKeyMap(keymap)
	field.WithTheme(huh.ThemeFunc(compactConfirmTheme))
	field.Focus()
	return ConfirmButtons{field: field, value: value}
}

func (b *ConfirmButtons) Update(msg tea.Msg) tea.Cmd {
	if b == nil || b.field == nil {
		return nil
	}
	updated, cmd := b.field.Update(msg)
	if field, ok := updated.(*huh.Confirm); ok {
		b.field = field
	}
	return cmd
}

func (b *ConfirmButtons) SetWidth(width int) {
	if b == nil || b.field == nil {
		return
	}
	b.field.WithWidth(max(1, width))
}

func (b ConfirmButtons) View() string {
	if b.field == nil {
		return ""
	}
	return b.field.View()
}

func (b ConfirmButtons) AffirmativeSelected() bool { return b.value != nil && *b.value }

func compactConfirmTheme(isDark bool) *huh.Styles {
	styles := huh.ThemeCharm(isDark)
	styles.Focused.Base = lipgloss.NewStyle()
	styles.Blurred.Base = lipgloss.NewStyle()
	return styles
}
