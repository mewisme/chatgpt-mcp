package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/testutil"
)

func TestRenderMarkdownFallsBackToRawWithoutPlugin(t *testing.T) {
	testutil.UseConfigRoot(t, t.TempDir())
	var out bytes.Buffer
	source := "# Title\n\nReadable markdown."
	if err := renderMarkdown(context.Background(), &out, source); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "# Title") || !strings.Contains(out.String(), "Readable markdown.") {
		t.Fatalf("got %q", out.String())
	}
}
