package pluginhost

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	"go.mewis.me/chatgpt-mcp/internal/runtimeplugin"
	"go.mewis.me/chatgpt-mcp/internal/toolprovider"
	"go.mewis.me/chatgpt-mcp/internal/tools"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

var installOnce sync.Once

func Install() {
	installOnce.Do(func() {
		pluginpkg.SetCompiledBuiltins(Builtins())
		tools.SyncCompiledPlugins = SyncTools
	})
}

func SyncTools(runtime *tools.Runtime) error {
	Install()
	if runtime == nil {
		return nil
	}
	Attach(runtime.PluginStore)
	return syncToolProviders(runtime)
}

func syncToolProviders(runtime *tools.Runtime) error {
	if runtime.Registry == nil || runtime.Workspaces == nil {
		return nil
	}
	replacements := map[string]map[string]tools.Entry{}
	seen := map[string]string{}
	for _, provider := range enabledToolProviders(runtime.PluginStore) {
		entries, err := providerTools(runtime.Workspaces, runtime.PluginStore, provider, seen)
		if err != nil || len(entries) == 0 {
			continue
		}
		replacements["plugin:"+string(provider.manifest.ID)] = entries
	}
	return runtime.Registry.ReplaceOwnedPrefix("plugin:", replacements)
}

type toolProvider struct {
	manifest   pluginpkg.Manifest
	version    pluginpkg.Version
	entrypoint string
	settings   map[string]any
}

func enabledToolProviders(store *pluginpkg.Store) []toolProvider {
	if store == nil {
		return nil
	}
	lock, err := pluginpkg.LoadLock(store.Layout().LockPath())
	if err != nil {
		return nil
	}
	settings := pluginpkg.SettingsStore{Layout: store.Layout()}
	ids := make([]string, 0, len(lock.Plugins))
	for id := range lock.Plugins {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)
	providers := make([]toolProvider, 0)
	for _, idValue := range ids {
		id := pluginpkg.PluginID(idValue)
		entry := lock.Plugins[id]
		if !entry.Enabled {
			continue
		}
		installed, err := store.Installed(id, entry.Version)
		if err != nil || installed.Manifest.Type != "tool-provider" {
			continue
		}
		path := strings.TrimSpace(installed.Entrypoint)
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(installed.Root, path)
		}
		values, _ := settings.Get(installed.Manifest.Config, id)
		providers = append(providers, toolProvider{manifest: installed.Manifest, version: entry.Version, entrypoint: path, settings: values})
	}
	return providers
}

func providerTools(workspaces *workspace.Manager, store *pluginpkg.Store, provider toolProvider, seen map[string]string) (map[string]tools.Entry, error) {
	session, err := RuntimeHost.Ensure(context.Background(), runtimeplugin.Spec{
		ID: string(provider.manifest.ID), Version: string(provider.version), Entrypoint: provider.entrypoint,
		WorkDir: filepath.Dir(provider.entrypoint), DataDir: store.Layout().PluginRuntimeDataDir(provider.manifest.ID),
	})
	if err != nil {
		return nil, err
	}
	if _, err := session.Call(context.Background(), runtimeplugin.MethodReconcile, toolprovider.ReconcileParams{Settings: provider.settings}); err != nil {
		return nil, err
	}
	raw, err := session.Call(context.Background(), runtimeplugin.MethodDescribe, nil)
	if err != nil {
		return nil, err
	}
	var described toolprovider.DescribeResult
	if err := json.Unmarshal(raw, &described); err != nil {
		return nil, err
	}
	if err := toolprovider.ValidateDescribe(described); err != nil {
		return nil, err
	}
	perms := make([]string, 0, len(provider.manifest.Permissions))
	for _, permission := range provider.manifest.Permissions {
		perms = append(perms, string(permission))
	}
	entries := map[string]tools.Entry{}
	for _, tool := range described.Tools {
		if owner, exists := seen[tool.Name]; exists && owner != string(provider.manifest.ID) {
			continue
		}
		risk := toolprovider.FloorRisk(tool.Risk, perms)
		entries[tool.Name] = hostedTool(workspaces, session, tool, risk)
		seen[tool.Name] = string(provider.manifest.ID)
	}
	return entries, nil
}

func hostedTool(workspaces *workspace.Manager, session *runtimeplugin.Session, tool toolprovider.Tool, risk string) tools.Entry {
	return tools.Entry{Schema: tools.Schema{
		Name: tool.Name, Title: tool.Title, Description: tool.Description,
		InputSchema: tool.InputSchema, OutputSchema: tool.OutputSchema,
		Annotations: tools.ToolAnnotations(tools.Risk(risk)),
	}, Handler: func(ctx context.Context, args map[string]any) (tools.Result, error) {
		if _, ok := args["workspace_id"]; ok {
			if _, err := workspaces.Get(strings.TrimSpace(fmtString(args["workspace_id"]))); err != nil {
				return tools.Result{}, err
			}
		}
		raw, err := session.Call(ctx, runtimeplugin.MethodInvoke, toolprovider.InvokeParams{Tool: tool.Name, Args: args})
		if err != nil {
			return tools.Result{}, err
		}
		var value any
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &value); err != nil {
				return tools.Result{}, err
			}
		}
		return tools.JSONResult(value), nil
	}}
}

func fmtString(value any) string {
	text, _ := value.(string)
	return text
}
