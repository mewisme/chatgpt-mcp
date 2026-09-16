package markdownformatter

import (
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/formatter"
)

func TestRenderASCIIIncludesHeadingText(t *testing.T) {
	got, err := Render(formatter.Request{Source: "# Hello\n\nWorld.", Width: 80, Style: "ascii"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Hello") || !strings.Contains(got, "World") {
		t.Fatalf("got %q", got)
	}
}
