package mcp

const SupportedProtocolVersion = "2026-07-28"

func Initialize(params map[string]any) map[string]any {
	clientInfo, _ := params["clientInfo"].(map[string]any)
	clientCapabilities, _ := params["capabilities"].(map[string]any)
	return map[string]any{
		"protocolVersion": SupportedProtocolVersion,
		"capabilities":    DefaultCapabilities(),
		"serverInfo": map[string]any{
			"name":    "chatgpt-mcp",
			"version": "0.1.0",
		},
		"client": map[string]any{
			"info":         clientInfo,
			"capabilities": clientCapabilities,
		},
	}
}
