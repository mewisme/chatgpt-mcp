package mcp

import "go.mewis.me/chatgpt-mcp/internal/tools"

type ToolCatalog struct{ Registry *tools.Registry }

func NewToolCatalog(registry *tools.Registry) *ToolCatalog { return &ToolCatalog{Registry: registry} }

func (c *ToolCatalog) List() []tools.Schema {
	return c.Registry.ListSchemas()
}
