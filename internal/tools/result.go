package tools

import (
	"encoding/json"
	"fmt"
)

type Content struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type Result struct {
	Content           []Content      `json:"content"`
	StructuredContent any            `json:"structuredContent,omitempty"`
	IsError           bool           `json:"isError,omitempty"`
	Meta              map[string]any `json:"_meta,omitempty"`
}

func TextResult(text string) Result {
	return Result{Content: []Content{{Type: "text", Text: text}}, StructuredContent: text}
}

func JSONResult(value any) Result {
	text, err := resultText(value)
	if err != nil {
		return ErrorResult(fmt.Errorf("encode tool result: %w", err))
	}
	return Result{Content: []Content{{Type: "text", Text: text}}, StructuredContent: value}
}

func ErrorResult(err error) Result {
	message := "tool execution failed"
	if err != nil {
		message = err.Error()
	}
	return Result{Content: []Content{{Type: "text", Text: message}}, IsError: true}
}

func resultText(value any) (string, error) {
	if text, ok := value.(string); ok {
		return text, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
