package application

import (
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

type ManagedTunnelResult struct {
	Metadata   tunnel.Metadata
	Configured bool
	Cleared    bool
}

func NormalizeTunnelIDs(values []string) []string {
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
