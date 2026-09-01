package mcp

import (
	"testing"
	"time"
)

func TestRequestActivityToolCall(t *testing.T) {
	event := requestActivity("tools/call", map[string]any{
		"name":           "read_text_file",
		"arguments":      map[string]any{"workspace_id": "ws_123", "path": "README.md"},
		"requestState":   "state_1",
		"inputResponses": map[string]any{"confirm": true},
		"_meta":          map[string]any{"trace": "trace_1"},
	}, "ok", "", 12*time.Millisecond)
	if event.Kind != "tool_call" || event.Method != "tools/call" || event.Tool != "read_text_file" || event.WorkspaceID != "ws_123" || event.Status != "ok" || event.DurationMS != 12 {
		t.Fatalf("event = %#v", event)
	}
	params, ok := event.Raw["params"].(map[string]any)
	if !ok || params["requestState"] != "state_1" || params["_meta"].(map[string]any)["trace"] != "trace_1" {
		t.Fatalf("raw = %#v", event.Raw)
	}
	args, ok := event.Raw["arguments"].(map[string]any)
	if !ok || args["path"] != "README.md" {
		t.Fatalf("raw arguments = %#v", event.Raw)
	}
}

func TestRequestActivityNonTool(t *testing.T) {
	event := requestActivity("tools/list", nil, "error", "failed", time.Millisecond)
	if event.Kind != "mcp_request" || event.Tool != "" || event.WorkspaceID != "" || event.Status != "error" || event.Message != "failed" {
		t.Fatalf("event = %#v", event)
	}
}
