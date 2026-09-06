package tui

import (
	"fmt"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/tui/quickopen"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func loadQuickOpenResources() ([]quickopen.Resource, error) {
	resources := pageQuickOpenResources()
	manager := workspace.NewManager(workspace.DefaultStorePath())
	workspaces, err := manager.List()
	if err != nil {
		return nil, err
	}
	for _, item := range workspaces {
		resources = append(resources, quickopen.Resource{ID: item.ID, Title: item.ID, Kind: "Workspace", Description: item.Path, Keywords: append([]string{item.Path}, item.LegacyIDs...), Path: []string{"workspace", item.ID}})
	}
	containers, err := manager.ListContainers()
	if err != nil {
		return nil, err
	}
	for _, item := range containers {
		resources = append(resources, quickopen.Resource{ID: item.ID, Title: item.Name, Kind: "Container", Description: fmt.Sprintf("%s · %d workspaces", item.ID, len(item.WorkspaceIDs)), Keywords: append([]string{item.ID}, item.WorkspaceIDs...), Path: []string{"containers", item.ID}})
	}
	upstreams := upstream.NewManager(upstream.NewStore(upstream.Path()))
	if err := upstreams.Load(); err != nil {
		return nil, err
	}
	for _, item := range upstreams.List() {
		endpoint := item.URL
		if item.Transport == "stdio" {
			endpoint = item.Command
		}
		resources = append(resources, quickopen.Resource{ID: item.ID, Title: item.ID, Kind: "MCP server", Description: endpoint, Keywords: []string{item.Name, item.Transport, item.Expose, endpoint}, Path: []string{"mcp", item.ID}})
	}
	source, err := config.Source()
	if err != nil {
		return nil, err
	}
	if source.Exists {
		cfg, err := config.Load()
		if err != nil {
			return nil, err
		}
		if id := strings.TrimSpace(cfg.Tunnel.ID); id != "" {
			title, description := id, "Configured tunnel"
			if metadata, err := config.LoadTunnelMetadata(id); err == nil {
				if strings.TrimSpace(metadata.Name) != "" {
					title = metadata.Name
				}
				description = id
			}
			resources = append(resources, quickopen.Resource{ID: id, Title: title, Kind: "Tunnel", Description: description, Keywords: []string{id}, Path: []string{"tunnel", id}})
		}
	}
	return resources, nil
}

func pageQuickOpenResources() []quickopen.Resource {
	return []quickopen.Resource{
		{ID: "home", Title: "Home", Kind: "Page", Path: []string{"home"}},
		{ID: "workspaces", Title: "Workspaces", Kind: "Page", Path: []string{"workspaces"}},
		{ID: "containers", Title: "Containers", Kind: "Page", Path: []string{"containers"}},
		{ID: "mcp", Title: "MCP Servers", Kind: "Page", Path: []string{"mcp"}},
		{ID: "tunnel", Title: "Tunnel", Kind: "Page", Path: []string{"tunnel"}},
		{ID: "requests", Title: "Requests", Kind: "Page", Path: []string{"requests"}},
		{ID: "logs", Title: "Logs", Kind: "Page", Path: []string{"logs"}},
		{ID: "config", Title: "Config", Kind: "Page", Path: []string{"config"}},
		{ID: "runtime", Title: "Runtime", Kind: "Page", Path: []string{"runtime"}},
		{ID: "about", Title: "About", Kind: "Page", Path: []string{"about"}},
	}
}
