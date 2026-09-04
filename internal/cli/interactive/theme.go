package interactive

import (
	"strings"

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

var (
	accentColor  = lipgloss.Color("6")
	successColor = lipgloss.Color("2")
	warningColor = lipgloss.Color("3")
	dangerColor  = lipgloss.Color("1")
	mutedColor   = lipgloss.Color("8")

	titleStyle   = lipgloss.NewStyle().Bold(true)
	accentStyle  = lipgloss.NewStyle().Foreground(accentColor).Bold(true)
	successStyle = lipgloss.NewStyle().Foreground(successColor).Bold(true)
	warningStyle = lipgloss.NewStyle().Foreground(warningColor).Bold(true)
	dangerStyle  = lipgloss.NewStyle().Foreground(dangerColor).Bold(true)
	mutedStyle   = lipgloss.NewStyle().Foreground(mutedColor)
	labelStyle   = lipgloss.NewStyle().Foreground(mutedColor)
)

func Title(value string) string  { return titleStyle.Render(value) }
func Accent(value string) string { return accentStyle.Render(value) }
func Muted(value string) string  { return mutedStyle.Render(value) }
func Label(value string) string  { return labelStyle.Render(value) }

func ToneText(value string, tone Tone) string {
	switch tone {
	case ToneAccent:
		return accentStyle.Render(value)
	case ToneSuccess:
		return successStyle.Render(value)
	case ToneWarning:
		return warningStyle.Render(value)
	case ToneDanger:
		return dangerStyle.Render(value)
	default:
		return value
	}
}

func Header(title, meta string, width int) string {
	left, right := Title(title), Muted(meta)
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
	return Muted(strings.Repeat("─", width))
}

func Filter(value string, active bool) string {
	cursor := ""
	if active {
		cursor = Accent("▌")
	}
	if value == "" && !active {
		return ""
	}
	return Accent("/ ") + value + cursor
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
		return Accent("›") + " "
	}
	return "  "
}

func Secondary(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return Muted(value)
}

func Panel(body string, width int) string {
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(mutedColor).Padding(0, 1)
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
		chunk := Accent(item.Key) + " " + Muted(item.Desc)
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

func KeyValue(label, value string) string {
	return Label(label) + "  " + value
}
