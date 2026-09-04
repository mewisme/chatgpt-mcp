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

type HelpItem struct {
	Key  string
	Desc string
}

type charmTheme struct {
	list        list.Styles
	item        list.DefaultItemStyles
	title       lipgloss.Style
	accent      lipgloss.Style
	muted       lipgloss.Style
	subtle      lipgloss.Style
	success     lipgloss.Style
	border      lipgloss.Style
	panelBorder lipgloss.Style
}

var currentTheme = newCharmTheme(true)

func newCharmTheme(isDark bool) charmTheme {
	listStyles := list.DefaultStyles(isDark)
	itemStyles := list.NewDefaultItemStyles(isDark)
	return charmTheme{
		list:        listStyles,
		item:        itemStyles,
		title:       lipgloss.NewStyle().Foreground(itemStyles.NormalTitle.GetForeground()).Bold(true),
		accent:      lipgloss.NewStyle().Foreground(itemStyles.SelectedTitle.GetForeground()).Bold(true),
		muted:       lipgloss.NewStyle().Foreground(itemStyles.NormalDesc.GetForeground()),
		subtle:      lipgloss.NewStyle().Foreground(listStyles.NoItems.GetForeground()),
		success:     lipgloss.NewStyle().Foreground(listStyles.Filter.Focused.Prompt.GetForeground()).Bold(true),
		border:      lipgloss.NewStyle().Foreground(itemStyles.SelectedTitle.GetBorderLeftForeground()),
		panelBorder: lipgloss.NewStyle().Foreground(itemStyles.SelectedTitle.GetBorderLeftForeground()),
	}
}

func SetDarkBackground(isDark bool) { currentTheme = newCharmTheme(isDark) }

func Title(value string) string  { return currentTheme.title.Render(value) }
func Accent(value string) string { return currentTheme.accent.Render(value) }
func Muted(value string) string  { return currentTheme.muted.Render(value) }
func Label(value string) string  { return currentTheme.muted.Render(value) }

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

func Header(title, meta string, width int) string {
	left, right := currentTheme.list.Title.Render(title), Muted(meta)
	line := left
	if right != "" {
		gap := 2
		if width > 0 {
			gap = width - lipgloss.Width(left) - lipgloss.Width(right)
			if gap < 2 {
				gap = 2
			}
		}
		line += strings.Repeat(" ", gap) + right
	}
	return line + "\n" + Divider(width)
}

func Divider(width int) string {
	if width <= 0 {
		width = 72
	}
	if width < 12 {
		width = 12
	}
	return currentTheme.subtle.Render(strings.Repeat("─", width))
}

func Filter(value string, active bool) string {
	cursor := ""
	if active {
		cursor = Accent("▌")
	}
	if value == "" && !active {
		return ""
	}
	return currentTheme.list.Filter.Focused.Prompt.Render("/ ") + value + cursor
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

func CursorMark(selected bool) string {
	if selected {
		return currentTheme.border.Render("│") + " "
	}
	return "  "
}

func Primary(value string, selected bool) string {
	if selected {
		return Accent(value)
	}
	return Title(value)
}

func Secondary(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return Muted(value)
}

func SelectedSecondary(value string, selected bool) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	if selected {
		style := lipgloss.NewStyle().Foreground(currentTheme.item.SelectedDesc.GetForeground())
		return style.Render(value)
	}
	return Secondary(value)
}

func Panel(body string, width int) string {
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(currentTheme.panelBorder.GetForeground()).Padding(0, 1)
	if width > 0 {
		style = style.MaxWidth(max(12, width))
	}
	return style.Render(body)
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

func Help(width int, items ...HelpItem) string {
	if len(items) == 0 {
		return ""
	}
	var lines []string
	line := ""
	for _, item := range items {
		chunk := item.Key + " " + Muted(item.Desc)
		separator := "   "
		candidate := chunk
		if line != "" {
			candidate = line + separator + chunk
		}
		if line != "" && width > 0 && lipgloss.Width(candidate) > width {
			lines = append(lines, line)
			line = chunk
			continue
		}
		line = candidate
	}
	if line != "" {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func KeyValue(label, value string) string { return Label(label) + "  " + value }
