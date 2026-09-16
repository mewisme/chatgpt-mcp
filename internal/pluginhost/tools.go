package pluginhost

import (
	"sync"

	"go.mewis.me/chatgpt-mcp/internal/caveman"
	"go.mewis.me/chatgpt-mcp/internal/features"
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	"go.mewis.me/chatgpt-mcp/internal/ponytail"
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

func SyncTools(runtime *tools.Runtime, raw any) error {
	Install()
	if runtime == nil {
		return nil
	}
	Attach(runtime.PluginStore)
	runtime.EnsurePluginSessions(compiledSessions)
	feat, _ := raw.(features.Config)
	return runtime.ApplyPluginSettings(map[string]map[string]any{
		"ponytail": {"default_active": feat.Ponytail.Active, "default_mode": feat.Ponytail.Mode},
		"caveman":  {"default_active": feat.Caveman.Active, "default_mode": feat.Caveman.Mode},
	})
}

func compiledSessions() []tools.PluginSession {
	pony := ponytail.NewManager(true, ponytail.Full)
	cave := caveman.NewManager(true, caveman.Full)
	return []tools.PluginSession{
		{
			Owner: "ponytail",
			Apply: func(values map[string]any) {
				pony.SetDefaults(boolSetting(values, "default_active", true), ponytail.Mode(stringSetting(values, "default_mode", "full")))
			},
			Tools: func(workspaces *workspace.Manager) map[string]tools.Entry {
				return map[string]tools.Entry{"ponytail_turn": tools.TurnControllerTool(workspaces, "ponytail_turn", "Ponytail Turn Controller", "Built-in Ponytail controller. Call before each user-facing coding response; configured active/mode values seed each workspace state. Pass the exact current user prompt.", `"off","lite","full","ultra","review"`, func(workspaceID, prompt, action string) (any, error) {
					return pony.Turn(workspaceID, prompt, action)
				})}
			},
		},
		{
			Owner: "caveman",
			Apply: func(values map[string]any) {
				cave.SetDefaults(boolSetting(values, "default_active", true), caveman.Mode(stringSetting(values, "default_mode", "full")))
			},
			Tools: func(workspaces *workspace.Manager) map[string]tools.Entry {
				return map[string]tools.Entry{"caveman_turn": tools.TurnControllerTool(workspaces, "caveman_turn", "Caveman Turn Controller", "Built-in Caveman controller. Call before each user-facing response; configured active/mode values seed each workspace state. Pass the exact current user prompt.", `"off","lite","full","ultra","wenyan-lite","wenyan-full","wenyan-ultra"`, func(workspaceID, prompt, action string) (any, error) {
					return cave.Turn(workspaceID, prompt, action)
				})}
			},
		},
	}
}

func boolSetting(values map[string]any, key string, fallback bool) bool {
	raw, ok := values[key]
	if !ok {
		return fallback
	}
	value, ok := raw.(bool)
	if !ok {
		return fallback
	}
	return value
}

func stringSetting(values map[string]any, key, fallback string) string {
	raw, ok := values[key]
	if !ok {
		return fallback
	}
	value, ok := raw.(string)
	if !ok || value == "" {
		return fallback
	}
	return value
}
