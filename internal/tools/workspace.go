package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func RegisterWorkspaceTools(registry *Registry, manager *workspace.Manager) {
	registry.MustRegister("workspace_register", Schema{
		Name:         "workspace_register",
		Title:        "Register Workspace",
		Description:  "Register a workspace root before using local coding tools. Re-registering the same canonical path returns the same workspace_id.",
		InputSchema:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}`),
		OutputSchema: ToolResultOutputSchema,
		Annotations:  ToolAnnotations(RiskRead),
	}, func(_ context.Context, args map[string]any) (Result, error) {
		path, err := requiredString(args, "path")
		if err != nil {
			return Result{}, err
		}
		item, err := manager.Register(path)
		if err != nil {
			return Result{}, err
		}
		return ToolResult("workspace_register", map[string]any{"workspace_id": item.ID, "workspace_root": item.Path}, "workspace registered: "+item.Path), nil
	})

	registry.MustRegister("workspace_status", Schema{
		Name:         "workspace_status",
		Title:        "Workspace Status",
		Description:  "Resolve a registered workspace and optional working_directory.",
		InputSchema:  json.RawMessage(`{"type":"object","properties":{"workspace_id":{"type":"string"},"working_directory":{"type":"string"}},"required":["workspace_id"],"additionalProperties":false}`),
		OutputSchema: ToolResultOutputSchema,
		Annotations:  ToolAnnotations(RiskRead),
	}, func(_ context.Context, args map[string]any) (Result, error) {
		id, err := requiredString(args, "workspace_id")
		if err != nil {
			return Result{}, err
		}
		workingDirectory, err := optionalString(args, "working_directory")
		if err != nil {
			return Result{}, err
		}
		item, cwd, err := manager.ResolveWorkingDirectory(id, workingDirectory)
		if err != nil {
			return Result{}, err
		}
		return ToolResult("workspace_status", map[string]any{
			"workspace_id":      item.ID,
			"workspace_root":    item.Path,
			"working_directory": cwd,
		}, "workspace cwd: "+cwd), nil
	})
}

func workspaceContext(manager *workspace.Manager, args map[string]any) (workspace.Workspace, string, error) {
	id, err := requiredString(args, "workspace_id")
	if err != nil {
		return workspace.Workspace{}, "", err
	}
	workingDirectory, err := requiredString(args, "working_directory")
	if err != nil {
		return workspace.Workspace{}, "", err
	}
	return manager.ResolveWorkingDirectory(id, workingDirectory)
}

func workspacePath(manager *workspace.Manager, args map[string]any, key string, mustExist bool) (workspace.Workspace, string, string, error) {
	item, cwd, err := workspaceContext(manager, args)
	if err != nil {
		return workspace.Workspace{}, "", "", err
	}
	value, err := requiredString(args, key)
	if err != nil {
		return workspace.Workspace{}, "", "", err
	}
	resolved, err := manager.ResolvePath(item.ID, cwd, value, mustExist)
	if err != nil {
		return workspace.Workspace{}, "", "", fmt.Errorf("%s: %w", key, err)
	}
	return item, cwd, resolved, nil
}

func optionalString(args map[string]any, key string) (string, error) {
	value, exists := args[key]
	if !exists {
		return "", nil
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", key)
	}
	return text, nil
}
