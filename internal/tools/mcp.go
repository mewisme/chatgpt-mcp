package tools

import "go.mewis.me/chatgpt-mcp/internal/upstream"

type MCPBridge struct { Manager *upstream.Manager }

func NewMCPBridge(manager *upstream.Manager) MCPBridge { return MCPBridge{Manager: manager} }

func (b MCPBridge) Servers() []upstream.Server { return b.Manager.List() }
