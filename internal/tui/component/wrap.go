package component

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const contentWrapBreakpoints = " \t/\\,.;:_?&=#"

func WrapContent(value string, width int) string {
	if width <= 0 || value == "" {
		return value
	}
	lines := strings.Split(value, "\n")
	for index, line := range lines {
		if line != "" {
			lines[index] = ansi.Hardwrap(ansi.Wrap(line, width, contentWrapBreakpoints), width, true)
		}
	}
	return strings.Join(lines, "\n")
}

func WrapKeyValue(label, value string, width int) string {
	prefix := Label(label) + "  "
	if width <= 0 {
		return prefix + value
	}
	prefixWidth := lipgloss.Width(prefix)
	if prefixWidth >= width {
		return WrapContent(Label(label), width) + "\n" + WrapContent(value, width)
	}
	lines := strings.Split(WrapContent(value, width-prefixWidth), "\n")
	if len(lines) == 0 {
		return prefix
	}
	indent := strings.Repeat(" ", prefixWidth)
	for index := range lines {
		if index == 0 {
			lines[index] = prefix + lines[index]
		} else {
			lines[index] = indent + lines[index]
		}
	}
	return strings.Join(lines, "\n")
}
func ModalContentWidth(width int) int {
	if width <= 0 {
		return width
	}
	return max(1, width-6)
}

func WrapModalBody(value string, width int) string {
	return WrapContent(value, ModalContentWidth(width))
}
