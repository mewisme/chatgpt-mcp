package tools

import (
	"encoding/json"
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
