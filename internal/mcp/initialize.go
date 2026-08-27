package mcp

type InitializeResult struct {
	ProtocolVersion string            `json:"protocolVersion"`
	ServerInfo      map[string]string `json:"serverInfo"`
}

func Initialize() InitializeResult {
	return InitializeResult{ProtocolVersion: "2026-07-28", ServerInfo: map[string]string{"name": "chatgpt-mcp", "version": "dev"}}
}
