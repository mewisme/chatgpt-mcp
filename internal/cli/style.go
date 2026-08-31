package cli

import (
	"fmt"
	"os"

	"github.com/fatih/color"
)

func cliStyled(attrs ...color.Attribute) *color.Color {
	value := color.New(attrs...)
	if os.Getenv("NO_COLOR") != "" {
		value.DisableColor()
	}
	return value
}

func cliDim(value any) string        { return cliStyled(color.Faint).Sprint(value) }
func cliHeading(value string) string { return cliStyled(color.Bold).Sprint(value) }

func cliState(value any) string {
	text := fmt.Sprint(value)
	switch text {
	case "connected", "ready", "running":
		return cliStyled(color.FgHiGreen, color.Bold).Sprint(text)
	case "connecting", "reconnecting":
		return cliStyled(color.FgHiCyan, color.Bold).Sprint(text)
	case "unreachable", "failed", "error":
		return cliStyled(color.FgHiRed, color.Bold).Sprint(text)
	case "stopped", "offline", "disabled", "not configured":
		return cliDim(text)
	default:
		return text
	}
}
