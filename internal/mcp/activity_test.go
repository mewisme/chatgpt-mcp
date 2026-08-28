package mcp

import (
	"testing"
	"time"
)

func TestRequestActivityToolCall(t *testing.T) {
	event := requestActivity("tools/call", map[string]any{
		"name":      "read_text_file",
		"arguments": map[string]any{"workspace_id": "ws_123"},
	}, "ok", "", 12*time.Millisecond)
	if event.Kind != "tool_call" || event.Method != "tools/call" || event.Tool != "read_text_file" || event.WorkspaceID != "ws_123" || event.Status != "ok" || event.DurationMS != 12 {
		t.Fatalf("event = %#v", event)
	}
}

func TestRequestActivityNonTool(t *testing.T) {
	event := requestActivity("tools/list", nil, "error", "failed", time.Millisecond)
	if event.Kind != "mcp_request" || event.Tool != "" || event.WorkspaceID != "" || event.Status != "error" || event.Message != "failed" {
		t.Fatalf("event = %#v", event)
	}
}
