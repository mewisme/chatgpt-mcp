package toolprovider

import "encoding/json"

const (
	MaxTools        = 32
	MaxSchemaBytes  = 16 << 10
	MaxNameBytes    = 64
	Prefix          = "tool-provider/"
	RiskRead        = "read"
	RiskEdit        = "edit"
	RiskCommand     = "command"
	RiskDestructive = "destructive"
)

type Tool struct {
	Name         string          `json:"name"`
	Title        string          `json:"title"`
	Description  string          `json:"description"`
	InputSchema  json.RawMessage `json:"input_schema"`
	OutputSchema json.RawMessage `json:"output_schema"`
	Risk         string          `json:"risk"`
}

type DescribeResult struct {
	Tools []Tool `json:"tools"`
}

type InvokeParams struct {
	Tool string         `json:"tool"`
	Args map[string]any `json:"args"`
}

type ReconcileParams struct {
	Settings map[string]any `json:"settings"`
}
