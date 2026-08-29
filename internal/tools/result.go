package tools

import (
	"encoding/json"
	"fmt"
)

type Content struct {
	Type string         `json:"type"`
	Text string         `json:"text,omitempty"`
	Raw  map[string]any `json:"-"`
}

func (c Content) MarshalJSON() ([]byte, error) {
	if c.Raw != nil {
		value := make(map[string]any, len(c.Raw)+2)
		for key, item := range c.Raw {
			value[key] = item
		}
		if _, ok := value["type"]; !ok && c.Type != "" {
			value["type"] = c.Type
		}
		if _, ok := value["text"]; !ok && c.Text != "" {
			value["text"] = c.Text
		}
		return json.Marshal(value)
	}
	type plain Content
	return json.Marshal(plain(c))
}

func (c *Content) UnmarshalJSON(data []byte) error {
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	c.Raw = value
	if item, ok := value["type"].(string); ok {
		c.Type = item
	}
	if item, ok := value["text"].(string); ok {
		c.Text = item
	}
	return nil
}

type Result struct {
	Content           []Content      `json:"content"`
	StructuredContent any            `json:"structuredContent,omitempty"`
	IsError           bool           `json:"isError,omitempty"`
	Meta              map[string]any `json:"_meta,omitempty"`
	ResultType        string         `json:"resultType,omitempty"`
	RequestState      string         `json:"requestState,omitempty"`
	InputRequests     map[string]any `json:"inputRequests,omitempty"`
}

func (r Result) MarshalJSON() ([]byte, error) {
	if r.ResultType == "input_required" {
		return json.Marshal(struct {
			ResultType    string         `json:"resultType"`
			InputRequests map[string]any `json:"inputRequests"`
			RequestState  string         `json:"requestState,omitempty"`
		}{ResultType: r.ResultType, InputRequests: r.InputRequests, RequestState: r.RequestState})
	}
	type wire struct {
		Content           []Content      `json:"content"`
		StructuredContent any            `json:"structuredContent,omitempty"`
		IsError           bool           `json:"isError,omitempty"`
		Meta              map[string]any `json:"_meta,omitempty"`
		ResultType        string         `json:"resultType,omitempty"`
	}
	return json.Marshal(wire{
		Content: r.Content, StructuredContent: r.StructuredContent, IsError: r.IsError, Meta: r.Meta, ResultType: r.ResultType,
	})
}

func TextResult(text string) Result {
	return Result{Content: []Content{{Type: "text", Text: text}}, StructuredContent: text, ResultType: "complete"}
}

func JSONResult(value any) Result {
	text, err := resultText(value)
	if err != nil {
		return ErrorResult(fmt.Errorf("encode tool result: %w", err))
	}
	return Result{Content: []Content{{Type: "text", Text: text}}, StructuredContent: value, ResultType: "complete"}
}

func ErrorResult(err error) Result {
	message := "tool execution failed"
	if err != nil {
		message = err.Error()
	}
	return Result{Content: []Content{{Type: "text", Text: message}}, IsError: true, ResultType: "complete"}
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
