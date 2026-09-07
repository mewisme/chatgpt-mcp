package component

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type DetailTab struct {
	Title   string
	Content string
}

type TabSpan struct {
	Index int
	X     int
	Width int
}

func TabDelta(msg tea.KeyPressMsg) (int, bool) {
	switch msg.String() {
	case "h", "left":
		return -1, true
	case "l", "right":
		return 1, true
	default:
		return 0, false
	}
}

func MoveTab(current, count, delta int) int {
	if count <= 0 {
		return 0
	}
	current = ((current % count) + count) % count
	if delta == 0 {
		return current
	}
	return ((current+delta)%count + count) % count
}

func Tabs(labels []string, active int) string {
	if len(labels) == 0 {
		return ""
	}
	active = MoveTab(active, len(labels), 0)
	parts := make([]string, 0, len(labels))
	for index, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		if index == active {
			parts = append(parts, currentTheme.accent.Render(label))
		} else {
			parts = append(parts, Muted(label))
		}
	}
	return strings.Join(parts, "   ")
}

func PageTabs(labels []string, active, width int) string {
	return PageTabsNotice(labels, active, "", width)
}

func PageTabsNotice(labels []string, active int, notice string, width int) string {
	view, _ := pageTabsLayout(labels, active, notice, width)
	return view
}

func PageTabsLayout(labels []string, active int, notice string, width int) (string, []TabSpan) {
	return pageTabsLayout(labels, active, notice, width)
}

func pageTabsLayout(labels []string, active int, notice string, width int) (string, []TabSpan) {
	if len(labels) == 0 {
		return "", nil
	}
	active = MoveTab(active, len(labels), 0)
	parts := make([]string, 0, len(labels)+1)
	spans := make([]TabSpan, 0, len(labels))
	x := 0
	for index, raw := range labels {
		label := strings.TrimSpace(raw)
		if label == "" {
			continue
		}
		desired := lipgloss.Width(label) + 2
		if width > 0 && x >= width {
			break
		}
		cellWidth := desired
		if width > 0 {
			cellWidth = min(cellWidth, width-x)
		}
		if cellWidth <= 0 {
			break
		}
		labelWidth := max(0, cellWidth-2)
		visible := ansi.Truncate(label, labelWidth, "")
		cell := NavItemStyle(index == active).Padding(0).Width(cellWidth).Align(lipgloss.Center).Render(visible)
		parts = append(parts, cell)
		spans = append(spans, TabSpan{Index: index, X: x, Width: cellWidth})
		x += cellWidth
	}
	if notice = strings.TrimSpace(notice); notice != "" && (width <= 0 || x < width) {
		separator := "  "
		available := 0
		if width > 0 {
			available = max(0, width-x-len(separator))
		}
		text := "· " + notice
		if width > 0 {
			text = ansi.Truncate(text, available, "")
		}
		if text != "" {
			parts = append(parts, separator+Muted(text))
		}
	}
	return strings.Join(parts, ""), spans
}
