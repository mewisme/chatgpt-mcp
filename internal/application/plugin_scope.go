package application

import (
	"fmt"
	"path/filepath"
	"strings"

	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

type PluginScopeOptions struct {
	Scope     string
	Workspace string
}

func ResolvePluginLayout(opts PluginScopeOptions) (pluginpkg.Layout, error) {
	return ResolvePluginLayoutWith(opts, workspace.NewManager(workspace.DefaultStorePath()))
}

func ResolvePluginLayoutWith(opts PluginScopeOptions, workspaces *workspace.Manager) (pluginpkg.Layout, error) {
	scope, err := parsePluginScopeOptions(opts)
	if err != nil {
		return pluginpkg.Layout{}, err
	}
	if scope != pluginpkg.ScopeWorkspace {
		return pluginpkg.DefaultLayout(), nil
	}
	item, err := resolvePluginWorkspace(workspaces, strings.TrimSpace(opts.Workspace))
	if err != nil {
		return pluginpkg.Layout{}, err
	}
	return pluginpkg.WorkspaceLayout(item.Path)
}

func parsePluginScopeOptions(opts PluginScopeOptions) (pluginpkg.PluginScope, error) {
	scopeText := strings.TrimSpace(opts.Scope)
	workspaceRef := strings.TrimSpace(opts.Workspace)
	if scopeText == "" {
		if workspaceRef != "" {
			return pluginpkg.ScopeWorkspace, nil
		}
		return pluginpkg.ScopeGlobal, nil
	}
	scope, err := pluginpkg.ParsePluginScope(scopeText)
	if err != nil {
		return "", fmt.Errorf("plugin --scope must be global or workspace")
	}
	if scope == pluginpkg.ScopeGlobal && workspaceRef != "" {
		return "", fmt.Errorf("--scope global cannot be combined with --workspace")
	}
	return scope, nil
}

func resolvePluginWorkspace(workspaces *workspace.Manager, ref string) (workspace.Workspace, error) {
	if workspaces == nil {
		return workspace.Workspace{}, fmt.Errorf("workspace manager is unavailable")
	}
	if ref == "" {
		items, err := workspaces.List()
		if err != nil {
			return workspace.Workspace{}, err
		}
		available := make([]workspace.Workspace, 0, len(items))
		for _, item := range items {
			if item.Available() {
				available = append(available, item)
			}
		}
		if len(available) == 1 {
			return available[0], nil
		}
		if len(available) == 0 {
			return workspace.Workspace{}, fmt.Errorf("no registered workspace; specify --workspace")
		}
		return workspace.Workspace{}, fmt.Errorf("multiple workspaces; specify --workspace")
	}
	if item, err := workspaces.Get(ref); err == nil {
		if !item.Available() {
			return workspace.Workspace{}, fmt.Errorf("%w: %s", workspace.ErrUnavailable, item.ID)
		}
		return item, nil
	}
	root, err := filepath.Abs(ref)
	if err != nil {
		return workspace.Workspace{}, err
	}
	items, err := workspaces.List()
	if err != nil {
		return workspace.Workspace{}, err
	}
	root = filepath.Clean(root)
	for _, item := range items {
		if filepath.Clean(item.Path) == root {
			if !item.Available() {
				return workspace.Workspace{}, fmt.Errorf("%w: %s", workspace.ErrUnavailable, item.ID)
			}
			return item, nil
		}
	}
	return workspace.Workspace{}, fmt.Errorf("%w: %s", workspace.ErrNotFound, ref)
}
