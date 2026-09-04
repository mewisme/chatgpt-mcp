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
	titleStyle = lipgloss.NewStyle().Bold(true)
	keyStyle   = lipgloss.NewStyle().Bold(true)
)

func Title(value string) string            { return titleStyle.Render(value) }
func Accent(value string) string           { return keyStyle.Render(value) }
func Muted(value string) string            { return value }
func Label(value string) string            { return value }
func ToneText(value string, _ Tone) string { return value }

func Header(title, meta string, width int) string {
	left, right := Title(title), meta
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
	return strings.Repeat("─", width)
}

func Filter(value string, active bool) string {
	cursor := ""
	if active {
		cursor = "▌"
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
	return marker + " " + message
}

func CursorMark(selected bool) string {
	if selected {
		return "› "
	}
	return "  "
}

func Secondary(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return value
}

func Panel(body string, width int) string {
	style := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
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
		chunk := Accent(item.Key) + " " + item.Desc
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

func KeyValue(label, value string) string { return label + "  " + value }
