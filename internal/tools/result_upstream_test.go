package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestContentPreservesRawMCPBlock(t *testing.T) {
	value := Result{Content: []Content{{
		Type: "image",
		Raw:  map[string]any{"type": "image", "data": "abc", "mimeType": "image/png"},
	}}}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	content := decoded["content"].([]any)[0].(map[string]any)
	if content["type"] != "image" || content["data"] != "abc" || content["mimeType"] != "image/png" {
		t.Fatalf("content = %#v", content)
	}
}

func TestRuntimeAppliesGlobalResultBudget(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegister("huge", DefaultSchema("huge", "huge result"), func(context.Context, map[string]any) (Result, error) {
		return Result{Content: []Content{{Type: "text", Text: strings.Repeat("x", maxToolResultBytes+1)}}, ResultType: "complete"}, nil
	})
	runtime := &Runtime{Registry: registry}
	result, err := runtime.Call(context.Background(), "huge", map[string]any{})
	if err != nil || result.IsError {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) >= maxToolResultBytes || result.StructuredContent != nil || result.Meta == nil {
		t.Fatalf("bounded bytes=%d structured=%#v meta=%#v", len(data), result.StructuredContent, result.Meta)
	}
}
