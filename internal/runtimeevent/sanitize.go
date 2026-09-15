package runtimeevent

import "go.mewis.me/chatgpt-mcp/internal/redact"

func sanitizeValue(key string, value any) any {
	return redact.Value(key, value)
}

func sanitizeString(value string) string { return redact.Text(value) }
