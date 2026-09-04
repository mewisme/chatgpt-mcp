package interactive

import (
	"strings"

	"charm.land/bubbles/v2/list"
	lipgloss "charm.land/lipgloss/v2"
)

type Tone int

const (
	ToneNeutral Tone = iota
	ToneAccent
	ToneSuccess
	ToneWarning
	ToneDanger
)

type charmTheme struct {
	item        list.DefaultItemStyles
	title       lipgloss.Style
	accent      lipgloss.Style
	muted       lipgloss.Style
	subtle      lipgloss.Style
	success     lipgloss.Style
	panelBorder lipgloss.Style
}

var currentTheme = newCharmTheme(true)

func newCharmTheme(isDark bool) charmTheme {
	listStyles := list.DefaultStyles(isDark)
	itemStyles := list.NewDefaultItemStyles(isDark)
	return charmTheme{
		item:        itemStyles,
		title:       lipgloss.NewStyle().Foreground(itemStyles.NormalTitle.GetForeground()).Bold(true),
		accent:      lipgloss.NewStyle().Foreground(itemStyles.SelectedTitle.GetForeground()).Bold(true),
		muted:       lipgloss.NewStyle().Foreground(itemStyles.NormalDesc.GetForeground()),
		subtle:      lipgloss.NewStyle().Foreground(listStyles.NoItems.GetForeground()),
		success:     lipgloss.NewStyle().Foreground(listStyles.Filter.Focused.Prompt.GetForeground()).Bold(true),
		panelBorder: lipgloss.NewStyle().Foreground(itemStyles.SelectedTitle.GetBorderLeftForeground()),
	}
}

func SetDarkBackground(isDark bool) { currentTheme = newCharmTheme(isDark) }

func Title(value string) string { return currentTheme.title.Render(value) }
func Muted(value string) string { return currentTheme.muted.Render(value) }
func Label(value string) string { return currentTheme.muted.Render(value) }

func ToneText(value string, tone Tone) string {
	switch tone {
	case ToneAccent, ToneWarning, ToneDanger:
		return currentTheme.accent.Render(value)
	case ToneSuccess:
		return currentTheme.success.Render(value)
	default:
		return value
	}
}

func Banner(message string, tone Tone) string {
	if strings.TrimSpace(message) == "" {
		return ""
	}
	marker := "·"
	if tone == ToneDanger {
		marker = "×"
	} else if tone == ToneWarning {
		marker = "!"
	} else if tone == ToneSuccess {
		marker = "✓"
	}
	return ToneText(marker, tone) + " " + message
}

func Secondary(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return Muted(value)
}

func Panel(body string, width int) string {
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(currentTheme.panelBorder.GetForeground()).Padding(0, 1)
	if width > 0 {
		style = style.MaxWidth(max(12, width))
	}
	return style.Render(body)
}

func Modal(body string, width int) string {
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(currentTheme.panelBorder.GetForeground()).Padding(1, 2)
	if width > 0 {
		style = style.Width(max(24, width))
	}
	return style.Render(body)
}

func ActionButton(key, label string) string {
	content := strings.TrimSpace(label)
	if key = strings.TrimSpace(key); key != "" {
		content = Muted(key) + " " + content
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder(), false, true, false, true).BorderForeground(currentTheme.panelBorder.GetForeground()).Padding(0, 1).Render(content)
}

func CenterOverlay(background, foreground string, width, height int) string {
	if width <= 0 {
		width = max(80, lipgloss.Width(background))
	}
	if height <= 0 {
		height = max(20, lipgloss.Height(background))
	}
	x := max(0, (width-lipgloss.Width(foreground))/2)
	y := max(0, (height-lipgloss.Height(foreground))/2)
	canvas := lipgloss.NewCanvas(width, height)
	base := lipgloss.NewLayer(background).X(0).Y(0).Z(0)
	dialog := lipgloss.NewLayer(foreground).X(x).Y(y).Z(1)
	canvas.Compose(lipgloss.NewCompositor(base, dialog))
	return canvas.Render()
}

func Divider(width int) string {
	if width <= 0 {
		return ""
	}
	return currentTheme.subtle.Render(strings.Repeat("─", width))
}

func TwoColumn(left, right string, width int) string {
	if right == "" || width <= 0 {
		if right == "" {
			return left
		}
		return left + "  " + right
	}
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 2 {
		return left + "  " + right
	}
	return left + strings.Repeat(" ", gap) + right
}

func KeyValue(label, value string) string { return Label(label) + "  " + value }
