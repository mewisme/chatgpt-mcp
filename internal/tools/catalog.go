package tools

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

func Catalog(defs []Schema) []Schema { return defs }

func CatalogHash(defs []Schema) (string, error) {
	ordered := append([]Schema(nil), defs...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Name < ordered[j].Name })
	data, err := json.Marshal(struct {
		Version int      `json:"version"`
		Tools   []Schema `json:"tools"`
	}{Version: 1, Tools: ordered})
	if err != nil {
		return "", fmt.Errorf("encode tool catalog: %w", err)
	}
	sum := sha256.Sum256(data)
	return "cat_" + hex.EncodeToString(sum[:]), nil
}

func DefaultSchema(name, description string) Schema {
	return Schema{Name: name, Description: description, InputSchema: json.RawMessage(`{"type":"object"}`), Annotations: map[string]any{"readOnly": false}}
}
