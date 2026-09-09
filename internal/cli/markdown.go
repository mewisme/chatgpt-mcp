package cli

import (
	"io"
	"os"
	"strings"

	"charm.land/glamour/v2"
	"golang.org/x/term"
)

const defaultMarkdownWidth = 100

func renderMarkdown(writer io.Writer, source string) error {
	width := markdownWidth(writer)
	options := []glamour.TermRendererOption{glamour.WithWordWrap(width)}
	if os.Getenv("NO_COLOR") != "" || !markdownTerminal(writer) {
		options = append(options, glamour.WithStandardStyle("ascii"))
	} else {
		options = append(options, glamour.WithEnvironmentConfig())
	}
	renderer, err := glamour.NewTermRenderer(options...)
	if err != nil {
		return err
	}
	output, err := renderer.Render(strings.TrimSpace(source) + "\n")
	if err != nil {
		return err
	}
	_, err = io.WriteString(writer, output)
	return err
}

func markdownWidth(writer io.Writer) int {
	if !markdownTerminal(writer) {
		return defaultMarkdownWidth
	}
	value := writer.(interface{ Fd() uintptr })
	if width, _, err := term.GetSize(int(value.Fd())); err == nil && width > 0 {
		return width
	}
	return defaultMarkdownWidth
}

func markdownTerminal(writer io.Writer) bool {
	type fdWriter interface{ Fd() uintptr }
	value, ok := writer.(fdWriter)
	return ok && term.IsTerminal(int(value.Fd()))
}
