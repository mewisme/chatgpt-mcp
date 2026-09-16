package config

import "go.mewis.me/chatgpt-mcp/internal/secretstore"

var MCPTokenSecretName = secretstore.Name("auth", "mcp-token")

func GetMCPToken() (string, error) {
	return secretstore.New(RootPath()).Get(MCPTokenSecretName)
}

func SetMCPToken(token string) error {
	return secretstore.New(RootPath()).Set(MCPTokenSecretName, token)
}
