package interactive

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

type DetailTab struct {
	Title   string
	Content string
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
