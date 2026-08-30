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

func TestAttachToolsPublishesActivityAndKeepsDefaultLogQuiet(t *testing.T) {
	previous := color.NoColor
	color.NoColor = true
	defer func() { color.NoColor = previous }()
	registry := tools.NewRegistry()
	registry.MustRegister("echo", tools.Schema{Name: "echo", InputSchema: json.RawMessage(`{"type":"object"}`)}, func(context.Context, map[string]any) (tools.Result, error) { return tools.TextResult("ok"), nil })
	runtime := &tools.Runtime{Registry: registry}
	stream := activity.NewStream()
	var output bytes.Buffer
	AttachTools(runtime, stream, logger.NewWithWriter(logger.Info, &output))
	ctx := tools.WithCallSource(context.Background(), "tunnel")
	result, err := runtime.Call(ctx, "echo", map[string]any{"workspace_id": "ws_test"})
	if err != nil || result.IsError {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if output.Len() != 0 {
		t.Fatalf("default tool logging should be quiet: %q", output.String())
	}
	events := stream.Recent(10)
	if len(events) != 1 {
		t.Fatalf("events=%#v", events)
	}
	event := events[0]
	if event.Kind != "tool_call" || event.Source != "tunnel" || event.Tool != "echo" || event.WorkspaceID != "ws_test" || event.Status != "ok" {
		t.Fatalf("event=%#v", event)
	}
}

func TestAttachToolsVerboseLogsCompletionWithoutStartNoise(t *testing.T) {
	previous := color.NoColor
	color.NoColor = true
	defer func() { color.NoColor = previous }()
	registry := tools.NewRegistry()
	registry.MustRegister("echo", tools.Schema{Name: "echo", InputSchema: json.RawMessage(`{"type":"object"}`)}, func(context.Context, map[string]any) (tools.Result, error) { return tools.TextResult("ok"), nil })
	runtime := &tools.Runtime{Registry: registry}
	var output bytes.Buffer
	log := logger.NewWithOptions(logger.Options{Level: logger.Info, Mode: logger.ModeVerbose, Writer: &output})
	AttachTools(runtime, nil, log)
	_, err := runtime.Call(tools.WithCallSource(context.Background(), "tunnel"), "echo", map[string]any{"workspace_id": "ws_test"})
	if err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if strings.Contains(text, "Tool call started") || !strings.Contains(text, "Tool call completed") || !strings.Contains(text, "tool: echo") {
		t.Fatalf("verbose output = %q", text)
	}
}
