package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/checkpoint"
	shellruntime "go.mewis.me/chatgpt-mcp/internal/shell"
	"go.mewis.me/chatgpt-mcp/internal/version"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

type ReadFilesResult struct {
	Files []ReadFile `json:"files"`
	Count int        `json:"count"`
}

type VersionResult struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildTime string `json:"build_time"`
}

func RegisterCore(registry *Registry, workspaces *workspace.Manager, checkpoints *checkpoint.Store, shells ...*shellruntime.Manager) {
	registry.MustRegister("get_version", coreSchema("get_version", "Get the running chatgpt-mcp binary version, commit, and build time.", `{"type":"object","properties":{},"additionalProperties":false}`, `{"type":"object","properties":{"version":{"type":"string"},"commit":{"type":"string"},"build_time":{"type":"string"}},"required":["version","commit","build_time"],"additionalProperties":false}`, RiskRead), func(context.Context, map[string]any) (Result, error) {
		return JSONResult(VersionResult{Version: version.Version, Commit: version.Commit, BuildTime: version.Date}), nil
	})
	RegisterFilesystemTools(registry, workspaces, checkpoints)
	shell := shellruntime.NewManager(workspaces, shellruntime.DefaultStateRoot())
	if len(shells) > 0 && shells[0] != nil {
		shell = shells[0]
	}
	processes := shellruntime.NewProcessManager(workspaces, shell)
	RegisterShellTools(registry, workspaces, shell, processes)
	RegisterGitTools(registry, workspaces)
	RegisterContextTools(registry, workspaces, checkpoints)
	RegisterRewindTools(registry, workspaces, checkpoints)
	RegisterAdvancedTools(registry, workspaces)
	registry.MustRegister("read_files", coreSchema("read_files", "Read multiple text files", `{"type":"object","properties":{"workspace_id":{"type":"string"},"paths":{"type":"array","items":{"type":"string"},"minItems":1}},"required":["workspace_id","paths"],"additionalProperties":false}`, `{"type":"object","properties":{"files":{"type":"array","items":{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"],"additionalProperties":false}},"count":{"type":"integer"}},"required":["files","count"],"additionalProperties":false}`, RiskRead), handleReadFiles(workspaces))
}

func coreSchema(name, description, input, output string, risk Risk) Schema {
	return Schema{Name: name, Description: description, InputSchema: json.RawMessage(input), OutputSchema: json.RawMessage(output), Annotations: ToolAnnotations(risk)}
}

func handleReadFiles(workspaces *workspace.Manager) Handler {
	return func(_ context.Context, args map[string]any) (Result, error) {
		item, cwd, err := workspaceContext(workspaces, args)
		if err != nil {
			return Result{}, err
		}
		paths, err := requiredStrings(args, "paths")
		if err != nil {
			return Result{}, err
		}
		files := make([]ReadFile, 0, len(paths))
		for _, value := range paths {
			file, err := workspaces.ResolvePath(item.ID, cwd, value, true)
			if err != nil {
				return Result{}, fmt.Errorf("path %q: %w", value, err)
			}
			data, err := os.ReadFile(file)
			if err != nil {
				return Result{}, err
			}
			files = append(files, ReadFile{Path: file, Content: string(data)})
		}
		return JSONResult(ReadFilesResult{Files: files, Count: len(files)}), nil
	}
}

func requiredString(args map[string]any, key string) (string, error) {
	value, ok := args[key].(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", key)
	}
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return value, nil
}

func requiredStrings(args map[string]any, key string) ([]string, error) {
	switch values := args[key].(type) {
	case []string:
		if len(values) == 0 {
			return nil, fmt.Errorf("%s must not be empty", key)
		}
		for i, value := range values {
			if strings.TrimSpace(value) == "" {
				return nil, fmt.Errorf("%s[%d] must not be empty", key, i)
			}
		}
		return values, nil
	case []any:
		if len(values) == 0 {
			return nil, fmt.Errorf("%s must not be empty", key)
		}
		result := make([]string, len(values))
		for i, value := range values {
			text, ok := value.(string)
			if !ok || strings.TrimSpace(text) == "" {
				return nil, fmt.Errorf("%s[%d] must be a non-empty string", key, i)
			}
			result[i] = text
		}
		return result, nil
	default:
		return nil, fmt.Errorf("%s must be an array of strings", key)
	}
}
