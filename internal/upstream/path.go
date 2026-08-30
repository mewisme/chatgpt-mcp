package upstream

import (
	"go.mewis.me/chatgpt-mcp/internal/configformat"
)

func Path() string {
	return configformat.StructuredPath(configformat.RootPath(), "upstream")
}
