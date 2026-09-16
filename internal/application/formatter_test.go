package application

import (
	"context"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/testutil"
)

func TestFormatMarkdownFallsBackWithoutPlugin(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	source := "# Title\n\nHello."
	got := FormatMarkdown(context.Background(), source, 80, false, "ascii")
	if strings.TrimSpace(got) != strings.TrimSpace(source) {
		t.Fatalf("got %q", got)
	}
}
