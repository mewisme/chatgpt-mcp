package tui

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/state"
)

func TestRunRejectsNonTTYWithoutWritingANSI(t *testing.T) {
	in := bytes.NewBufferString("input")
	out := &bytes.Buffer{}
	if TerminalIO(in, out) {
		t.Fatal("buffers unexpectedly detected as terminals")
	}
	err := Run(context.Background(), Route{Kind: RouteHome}, in, out)
	if err == nil || !strings.Contains(err.Error(), "requires terminal stdin and stdout") {
		t.Fatalf("err=%v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("non-TTY run wrote output: %q", out.String())
	}
	active, err := state.TUIReviewerActive()
	if err != nil || active {
		t.Fatalf("non-TTY run held presence active=%t err=%v", active, err)
	}
}
