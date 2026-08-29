package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/fatih/color"
	"go.mewis.me/chatgpt-mcp/internal/activity"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/tools"
)

func TestAttachToolsPublishesTunnelActivityAndLogs(t *testing.T) {
	previous := color.NoColor
	color.NoColor = true
	defer func() { color.NoColor = previous }()

	registry := tools.NewRegistry()
	registry.MustRegister("echo", tools.Schema{Name: "echo", InputSchema: json.RawMessage(`{"type":"object"}`)}, func(context.Context, map[string]any) (tools.Result, error) {
		return tools.TextResult("ok"), nil
	})
	runtime := &tools.Runtime{Registry: registry}
	stream := activity.NewStream()
	var output bytes.Buffer
	log := logger.NewWithWriter(logger.Info, &output)
	AttachTools(runtime, stream, log)

	ctx := tools.WithCallSource(context.Background(), "tunnel")
	result, err := runtime.Call(ctx, "echo", map[string]any{"workspace_id": "ws_test"})
	if err != nil || result.IsError {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	events := stream.Recent(10)
	if len(events) != 1 {
		t.Fatalf("events=%#v", events)
	}
	event := events[0]
	if event.Kind != "tool_call" || event.Source != "tunnel" || event.Tool != "echo" || event.WorkspaceID != "ws_test" || event.Status != "ok" {
		t.Fatalf("event=%#v", event)
	}
	text := output.String()
	for _, expected := range []string{"TOOL", "call started", "call completed", "tool=echo", "source=tunnel", "workspace=ws_test"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("output %q missing %q", text, expected)
		}
	}
}
