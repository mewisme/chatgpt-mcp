package markdownformatter

import (
	"strings"

	glamour "charm.land/glamour/v2"

	"go.mewis.me/chatgpt-mcp/internal/formatter"
)

func Render(req formatter.Request) (string, error) {
	width := req.Width
	if width <= 0 {
		width = 100
	}
	options := []glamour.TermRendererOption{glamour.WithWordWrap(width)}
	if req.Style == "ascii" || !req.Terminal {
		options = append(options, glamour.WithStandardStyle("ascii"))
	} else {
		options = append(options, glamour.WithEnvironmentConfig())
	}
	renderer, err := glamour.NewTermRenderer(options...)
	if err != nil {
		return "", err
	}
	defer renderer.Close()
	return renderer.Render(strings.TrimSpace(req.Source) + "\n")
}
