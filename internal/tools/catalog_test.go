package tools

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCatalogHashIsOrderIndependentAndSchemaSensitive(t *testing.T) {
	first := Schema{Name: "a", Description: "one", InputSchema: json.RawMessage(`{"type":"object"}`)}
	second := Schema{Name: "b", Description: "two", InputSchema: json.RawMessage(`{"type":"object"}`)}
	left, err := CatalogHash([]Schema{first, second})
	if err != nil {
		t.Fatal(err)
	}
	right, err := CatalogHash([]Schema{second, first})
	if err != nil {
		t.Fatal(err)
	}
	if left != right || left == "" {
		t.Fatalf("hashes = %q / %q", left, right)
	}
	second.Description = "changed"
	changed, err := CatalogHash([]Schema{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if changed == left {
		t.Fatal("catalog hash did not change with schema")
	}
}

func TestRuntimeDoesNotExposeWorkspaceRelocateTool(t *testing.T) {
	runtime := NewRuntime()
	for _, schema := range runtime.ListTools() {
		name := strings.ToLower(strings.TrimSpace(schema.Name))
		if name == "workspace_relocate" || name == "workspace.relocate" || name == "workspace_move" {
			t.Fatalf("control-plane workspace relocate leaked into MCP tool catalog as %q", schema.Name)
		}
	}
}
