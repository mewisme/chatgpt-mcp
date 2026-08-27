package mcp

import "go.mewis.me/chatgpt-mcp/internal/tools"

type ToolCatalog struct{ Items []tools.Schema }

func NewToolCatalog() *ToolCatalog { return &ToolCatalog{} }

func (c *ToolCatalog) List() []tools.Schema { return c.Items }

func (c *ToolCatalog) Add(schema tools.Schema) {
	c.Items = append(c.Items, schema)
}
