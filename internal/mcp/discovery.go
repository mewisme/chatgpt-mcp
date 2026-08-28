package mcp

import "go.mewis.me/chatgpt-mcp/internal/version"

const (
	SupportedProtocolVersion = "2026-07-28"
	defaultCacheTTLMS        = 0
	defaultCacheScope        = "private"
)

func Discover() map[string]any {
	return map[string]any{
		"supportedVersions": []string{SupportedProtocolVersion},
		"capabilities":      DefaultCapabilities(),
		"ttlMs":             defaultCacheTTLMS,
		"cacheScope":        defaultCacheScope,
		"_meta": map[string]any{
			"io.modelcontextprotocol/serverInfo": map[string]any{
				"name":    "chatgpt-mcp",
				"version": version.Version,
			},
		},
	}
}
