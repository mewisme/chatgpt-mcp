package tools

import "encoding/json"

func Catalog(defs []Schema) []Schema { return defs }

func DefaultSchema(name, description string) Schema {
	return Schema{Name: name, Description: description, InputSchema: json.RawMessage(`{"type":"object"}`), Annotations: map[string]any{"readOnly": false}}
}
