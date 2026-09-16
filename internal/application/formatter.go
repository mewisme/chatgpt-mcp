package application

import (
	"context"
	"fmt"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/formatter"
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
)

const markdownFormatterCapability = pluginpkg.Capability("formatter/markdown")

func FormatMarkdown(ctx context.Context, source string, width int, terminal bool, style string) string {
	fallback := formatter.Fallback(source)
	provider, err := LookupMarkdownFormatter()
	if err != nil {
		return fallback
	}
	text, err := formatter.Run(ctx, provider.Path, formatter.Request{Source: source, Width: width, Terminal: terminal, Style: style})
	if err != nil || strings.TrimSpace(text) == "" {
		return fallback
	}
	return text
}

func LookupMarkdownFormatter() (pluginpkg.CapabilityProvider, error) {
	service, err := NewPluginService()
	if err != nil {
		return pluginpkg.CapabilityProvider{}, err
	}
	resolver, err := pluginpkg.NewResolver(service.Manager.Store)
	if err != nil {
		return pluginpkg.CapabilityProvider{}, err
	}
	provider, err := resolver.Resolve(markdownFormatterCapability)
	if err != nil {
		return pluginpkg.CapabilityProvider{}, err
	}
	if strings.TrimSpace(provider.Path) == "" {
		return pluginpkg.CapabilityProvider{}, fmt.Errorf("markdown formatter plugin path is empty")
	}
	return provider, nil
}
