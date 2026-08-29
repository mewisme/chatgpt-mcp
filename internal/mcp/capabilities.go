package mcp

type Capabilities struct {
	Tools ToolsCapability `json:"tools"`
}

type ToolsCapability struct {
	ListChanged bool `json:"listChanged"`
}

func DefaultCapabilities() Capabilities {
	return Capabilities{Tools: ToolsCapability{ListChanged: true}}
}
