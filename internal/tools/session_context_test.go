package tools

import (
	"context"
	"testing"
)

func TestMCPSessionIDContextIsolation(t *testing.T) {
	first := WithMCPSessionID(context.Background(), "session-a")
	second := WithMCPSessionID(context.Background(), "session-b")
	if got := MCPSessionID(first); got != "session-a" {
		t.Fatalf("first session = %q", got)
	}
	if got := MCPSessionID(second); got != "session-b" {
		t.Fatalf("second session = %q", got)
	}
	if got := MCPSessionID(context.Background()); got != "" {
		t.Fatalf("empty context session = %q", got)
	}
}
