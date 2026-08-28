package tools

import (
	"encoding/json"
	"fmt"
)

var ToolResultOutputSchema = json.RawMessage(`{"type":"object","properties":{"ok":{"type":"boolean"},"tool":{"type":"string"},"summary":{"type":"string"},"data":{"type":"object","additionalProperties":true}},"required":["ok","tool","summary","data"],"additionalProperties":false}`)

type Content struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type Payload struct {
	OK      bool           `json:"ok"`
	Tool    string         `json:"tool"`
	Summary string         `json:"summary"`
	Data    map[string]any `json:"data"`
}

type Result struct {
	Content           []Content      `json:"content"`
	StructuredContent any            `json:"structuredContent,omitempty"`
	IsError           bool           `json:"isError,omitempty"`
	Meta              map[string]any `json:"_meta,omitempty"`
}

func ToolResult(tool string, data map[string]any, summary string) Result {
	if data == nil {
		data = map[string]any{}
	}
	if summary == "" {
		summary = defaultSummary(tool, data)
	}
	payload := Payload{OK: true, Tool: tool, Summary: summary, Data: data}
	return payloadResult(payload, false)
}

func ToolErrorResult(tool string, err error, data map[string]any) Result {
	message := "tool execution failed"
	if err != nil {
		message = err.Error()
	}
	if data == nil {
		data = map[string]any{}
	}
	if _, exists := data["error"]; !exists {
		data["error"] = message
	}
	payload := Payload{OK: false, Tool: tool, Summary: message, Data: data}
	return payloadResult(payload, true)
}

func payloadResult(payload Payload, isError bool) Result {
	text, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		fallback := fmt.Sprintf(`{"ok":false,"tool":%q,"summary":"failed to encode tool result","data":{}}`, payload.Tool)
		return Result{Content: []Content{{Type: "text", Text: fallback}}, IsError: true}
	}
	return Result{
		Content:           []Content{{Type: "text", Text: string(text)}},
		StructuredContent: payload,
		IsError:           isError,
	}
}

func defaultSummary(tool string, data map[string]any) string {
	if path, ok := data["path"].(string); ok {
		return tool + ": " + path
	}
	if command, ok := data["command"].(string); ok {
		return tool + ": " + command
	}
	if code, ok := data["exit_code"].(int); ok {
		return fmt.Sprintf("%s: exit %d", tool, code)
	}
	return tool + ": done"
}
