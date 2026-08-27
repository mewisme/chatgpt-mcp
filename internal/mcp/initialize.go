package mcp

func Initialize() map[string]any {
	return map[string]any{
		"protocolVersion": "2026-07-28",
		"capabilities":    DefaultCapabilities(),
		"serverInfo": map[string]any{
			"name":    "chatgpt-mcp",
			"version": "0.1.0",
		},
	}
}
