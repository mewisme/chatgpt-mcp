package cli

import (
	"context"
	"io"
	"os"

	"golang.org/x/term"

	"go.mewis.me/chatgpt-mcp/internal/application"
)

const defaultMarkdownWidth = 100

func renderMarkdown(ctx context.Context, writer io.Writer, source string) error {
	width := markdownWidth(writer)
	terminal := markdownTerminal(writer)
	style := "environment"
	if os.Getenv("NO_COLOR") != "" || !terminal {
		style = "ascii"
	}
	_, err := io.WriteString(writer, application.FormatMarkdown(ctx, source, width, terminal, style))
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
