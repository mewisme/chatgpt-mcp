package tools

import (
	"encoding/json"
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
