package mcp

type Capabilities struct {
	Tools ToolsCapability `json:"tools"`
}

type ToolsCapability struct{}

func DefaultCapabilities() Capabilities {
	return Capabilities{Tools: ToolsCapability{}}
}
