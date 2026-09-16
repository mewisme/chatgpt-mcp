package tools

import (
	"context"
	"fmt"

	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

var SyncCompiledPlugins func(runtime *Runtime, featureConfig any) error

type PluginSession struct {
	Owner string
	Apply func(map[string]any)
	Tools func(*workspace.Manager) map[string]Entry
}

func (r *Runtime) EnsurePluginSessions(build func() []PluginSession) {
	if r == nil || r.pluginSessions != nil || build == nil {
		return
	}
	r.pluginSessions = build()
}

func (r *Runtime) ApplyPluginSettings(settings map[string]map[string]any) error {
	if r == nil || r.Registry == nil || r.Workspaces == nil {
		return fmt.Errorf("tool runtime is unavailable")
	}
	replacements := map[string]map[string]Entry{}
	for _, session := range r.pluginSessions {
		if values, ok := settings[session.Owner]; ok && session.Apply != nil {
			session.Apply(values)
		}
		if session.Tools == nil {
			continue
		}
		replacements["plugin:"+session.Owner] = session.Tools(r.Workspaces)
	}
	return r.Registry.ReplaceOwnedPrefix("plugin:", replacements)
}

func TurnControllerTool(workspaces *workspace.Manager, name, title, description, outputModes string, turn func(workspaceID, prompt, action string) (any, error)) Entry {
	return Entry{Schema: Schema{
		Name: name, Title: title, Description: description,
		InputSchema:  []byte(`{"type":"object","properties":{"workspace_id":{"type":"string"},"prompt":{"type":"string"},"action":{"type":"string","enum":["turn","refresh","status"],"default":"turn"}},"required":["workspace_id","prompt"],"additionalProperties":false}`),
		OutputSchema: []byte(fmt.Sprintf(`{"type":"object","properties":{"available":{"type":"boolean"},"mode":{"type":"string","enum":[%s]},"active":{"type":"boolean"},"active_instructions":{"type":"string"},"refresh_hint":{"type":"string"}},"required":["available","mode","active"],"additionalProperties":false}`, outputModes)),
		Annotations:  ToolAnnotations(RiskRead),
	}, Handler: func(_ context.Context, args map[string]any) (Result, error) {
		item, err := workspaceFromArgs(workspaces, args)
		if err != nil {
			return Result{}, err
		}
		prompt, err := requiredString(args, "prompt")
		if err != nil {
			return Result{}, err
		}
		action, err := optionalEnum(args, "action", "turn", "turn", "refresh", "status")
		if err != nil {
			return Result{}, err
		}
		value, err := turn(item.ID, prompt, action)
		if err != nil {
			return Result{}, err
		}
		return JSONResult(value), nil
	}}
}
