package tuiguide

import (
	"strings"
	"testing"
)

func TestTopicsHaveUniqueEmbeddedMarkdown(t *testing.T) {
	seen := map[string]bool{}
	for _, topic := range Topics() {
		if topic.ID == "" || topic.Title == "" || topic.Description == "" || topic.File == "" {
			t.Fatalf("incomplete topic: %#v", topic)
		}
		if seen[topic.ID] {
			t.Fatalf("duplicate topic ID %q", topic.ID)
		}
		seen[topic.ID] = true
		markdown, err := Markdown(topic.ID)
		if err != nil {
			t.Fatalf("Markdown(%q): %v", topic.ID, err)
		}
		if !strings.HasPrefix(strings.TrimSpace(markdown), "# ") {
			t.Fatalf("topic %q does not start with a Markdown heading", topic.ID)
		}
	}
	if len(seen) < 8 {
		t.Fatalf("guide topic coverage too small: %d", len(seen))
	}
}

func TestMarkdownReturnsOnlyRequestedTopic(t *testing.T) {
	mcp, err := Markdown("mcp")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(mcp, "# MCP Servers") || strings.Contains(mcp, "## Shell & Execution") {
		t.Fatalf("MCP topic contains unexpected content: %q", mcp)
	}
	if _, err := Markdown("missing-topic"); err == nil {
		t.Fatal("unknown topic unexpectedly loaded")
	}
}
