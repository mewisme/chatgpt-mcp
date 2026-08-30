package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/checkpoint"
	projectcontext "go.mewis.me/chatgpt-mcp/internal/context"
	"go.mewis.me/chatgpt-mcp/internal/memory"
	"go.mewis.me/chatgpt-mcp/internal/rules"
	"go.mewis.me/chatgpt-mcp/internal/skills"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

type SkillsListResult struct {
	Skills []skills.Skill `json:"skills"`
	Count  int            `json:"count"`
}

type PathRulesResult struct {
	Path  string       `json:"path"`
	Rules []rules.Rule `json:"rules"`
	Count int          `json:"count"`
}

type RememberResult struct {
	SavedTo string `json:"saved_to"`
	Note    string `json:"note"`
}

type AgentStatusResult struct {
	PermissionProfile     string         `json:"permission_profile"`
	PermissionDescription string         `json:"permission_description"`
	FullMachineAccess     bool           `json:"full_machine_access"`
	DefaultCWD            string         `json:"default_cwd"`
	MachineRoots          []string       `json:"machine_roots"`
	WorkspaceID           string         `json:"workspace_id"`
	WorkspaceRoot         string         `json:"workspace_root"`
	PID                   int            `json:"pid"`
	Go                    string         `json:"go"`
	Quickstart            []string       `json:"quickstart"`
	Rewind                map[string]any `json:"rewind"`
	UpstreamMCP           map[string]any `json:"upstream_mcp"`
	ToolProfile           string         `json:"tool_profile"`
	ToolCount             int            `json:"tool_count"`
}

func RegisterContextTools(registry *Registry, workspaces *workspace.Manager, checkpoints *checkpoint.Store) {
	memoryStore := memory.NewStore(memory.DefaultRoot())
	register := func(name, title, description, input, output string, risk Risk, handler Handler) {
		registry.MustRegister(name, Schema{
			Name: name, Title: title, Description: description,
			InputSchema: json.RawMessage(input), OutputSchema: json.RawMessage(output), Annotations: ToolAnnotations(risk),
		}, handler)
	}

	register("list_skills", "List Skills", "List project skills and activation descriptions across supported agent providers.", workspaceOnlySchema(``), `{"type":"object","properties":{"skills":{"type":"array","items":{"type":"object","additionalProperties":true}},"count":{"type":"integer"}},"required":["skills","count"],"additionalProperties":false}`, RiskRead, func(_ context.Context, args map[string]any) (Result, error) {
		item, err := workspaceFromArgs(workspaces, args)
		if err != nil {
			return Result{}, err
		}
		values, err := skills.Discover(item.Path)
		if err != nil {
			return Result{}, err
		}
		return JSONResult(SkillsListResult{Skills: values, Count: len(values)}), nil
	})

	register("load_skill", "Load Skill", "Load one skill's complete instructions by exact name returned from list_skills.", workspaceOnlySchema(`"name":{"type":"string"},"max_bytes":{"type":"integer","minimum":1,"maximum":500000,"default":200000},`), `{"type":"object","properties":{"skill":{"type":"object","additionalProperties":true},"content":{"type":"string"},"truncated":{"type":"boolean"}},"required":["skill","content","truncated"],"additionalProperties":false}`, RiskRead, func(_ context.Context, args map[string]any) (Result, error) {
		item, err := workspaceFromArgs(workspaces, args)
		if err != nil {
			return Result{}, err
		}
		name, err := requiredString(args, "name")
		if err != nil {
			return Result{}, err
		}
		maxBytes, err := optionalInt(args, "max_bytes", 200_000, 1, 500_000)
		if err != nil {
			return Result{}, err
		}
		value, err := skills.Load(item.Path, name, maxBytes)
		if err != nil {
			return Result{}, err
		}
		if _, err := workspaces.ResolvePath(item.ID, item.Path, value.Skill.Path, true); err != nil {
			return Result{}, fmt.Errorf("skill path: %w", err)
		}
		return JSONResult(value), nil
	})

	register("project_context", "Project Context", "Load project instructions, config, and provider rules under a workspace path.", workspaceOnlySchema(`"working_directory":{"type":"string"},"path":{"type":"string"},"max_depth":{"type":"integer","minimum":0,"maximum":5,"default":3},"max_bytes_per_file":{"type":"integer","minimum":1,"maximum":200000,"default":60000},`), `{"type":"object","properties":{"root":{"type":"string"},"files":{"type":"array","items":{"type":"object","additionalProperties":true}},"count":{"type":"integer"}},"required":["root","files","count"],"additionalProperties":false}`, RiskRead, func(_ context.Context, args map[string]any) (Result, error) {
		item, cwd, err := workspaceLocation(workspaces, args)
		if err != nil {
			return Result{}, err
		}
		pathValue, err := optionalString(args, "path")
		if err != nil {
			return Result{}, err
		}
		root := cwd
		if strings.TrimSpace(pathValue) != "" {
			root, err = workspaces.ResolvePath(item.ID, cwd, pathValue, true)
			if err != nil {
				return Result{}, err
			}
			info, err := os.Stat(root)
			if err != nil {
				return Result{}, err
			}
			if !info.IsDir() {
				return Result{}, errors.New("project_context path must be a directory")
			}
		}
		maxDepth, err := optionalInt(args, "max_depth", 3, 0, 5)
		if err != nil {
			return Result{}, err
		}
		maxBytes, err := optionalInt(args, "max_bytes_per_file", 60_000, 1, 200_000)
		if err != nil {
			return Result{}, err
		}
		value, err := projectcontext.Load(root, maxDepth, maxBytes)
		if err != nil {
			return Result{}, err
		}
		filtered := value.Files[:0]
		for _, file := range value.Files {
			if _, err := workspaces.ResolvePath(item.ID, root, file.Path, true); err == nil {
				filtered = append(filtered, file)
			}
		}
		value.Files = filtered
		value.Count = len(filtered)
		return JSONResult(value), nil
	})

	register("agent_status", "Agent Status", "Show workspace permissions, runtime, rewind config, upstream configuration, and tool runtime status.", workspaceOnlySchema(`"working_directory":{"type":"string"},`), `{"type":"object","additionalProperties":true}`, RiskRead, func(_ context.Context, args map[string]any) (Result, error) {
		item, cwd, err := workspaceLocation(workspaces, args)
		if err != nil {
			return Result{}, err
		}
		upstreamManager := upstream.NewManager(upstream.NewStore(upstream.Path()))
		servers := []upstream.Server{}
		if err := upstreamManager.Load(); err == nil {
			servers = upstreamManager.List()
		}
		roots, err := workspaces.EffectiveRoots(item.ID)
		if err != nil {
			return Result{}, err
		}
		return JSONResult(AgentStatusResult{
			PermissionProfile:     "workspace-bound+allow-dirs",
			PermissionDescription: "local tools may operate inside the workspace root plus explicitly allowed global/workspace directories; mutating shell commands remain fail-closed",
			FullMachineAccess:     false,
			DefaultCWD:            cwd,
			MachineRoots:          roots,
			WorkspaceID:           item.ID,
			WorkspaceRoot:         item.Path,
			PID:                   os.Getpid(),
			Go:                    runtime.Version(),
			Quickstart: []string{
				"Register one workspace and pass its workspace_id to local tools.",
				"Read before editing; use apply_patch/edit_file for deterministic changes.",
				"Mutating shell commands require a matching explicit working_directory.",
				"Use rewind to list, preview, or restore automatic file checkpoints.",
			},
			Rewind: checkpoints.Config(item.ID),
			UpstreamMCP: map[string]any{
				"config_path": upstream.Path(),
				"servers":     servers,
			},
			ToolProfile: "full",
			ToolCount:   len(registry.ListSchemas()),
		}), nil
	})

	register("remember", "Remember", "Save a workspace-scoped cross-session note, similar to Claude Code MEMORY.md.", workspaceOnlySchema(`"note":{"type":"string"},`), `{"type":"object","properties":{"saved_to":{"type":"string"},"note":{"type":"string"}},"required":["saved_to","note"],"additionalProperties":false}`, RiskEdit, func(_ context.Context, args map[string]any) (Result, error) {
		item, err := workspaceFromArgs(workspaces, args)
		if err != nil {
			return Result{}, err
		}
		note, err := requiredString(args, "note")
		if err != nil {
			return Result{}, err
		}
		path, err := memoryStore.Append(item.ID, note)
		if err != nil {
			return Result{}, err
		}
		return JSONResult(RememberResult{SavedTo: path, Note: strings.TrimSpace(note)}), nil
	})

	register("load_path_rules", "Load Path Rules", "Load path-scoped rules from .claude/.claudes/.agents/.cursor/.codex rule directories.", workspaceOnlySchema(`"working_directory":{"type":"string"},"path":{"type":"string"},`), `{"type":"object","properties":{"path":{"type":"string"},"rules":{"type":"array","items":{"type":"object","additionalProperties":true}},"count":{"type":"integer"}},"required":["path","rules","count"],"additionalProperties":false}`, RiskRead, func(_ context.Context, args map[string]any) (Result, error) {
		item, cwd, err := workspaceLocation(workspaces, args)
		if err != nil {
			return Result{}, err
		}
		pathValue, err := requiredString(args, "path")
		if err != nil {
			return Result{}, err
		}
		target, err := workspaces.ResolvePath(item.ID, cwd, pathValue, false)
		if err != nil {
			return Result{}, err
		}
		values, err := rules.LoadForFile(item.Path, target)
		if err != nil {
			return Result{}, err
		}
		for _, rule := range values {
			if _, err := workspaces.ResolvePath(item.ID, item.Path, rule.Path, true); err != nil {
				return Result{}, fmt.Errorf("rule path: %w", err)
			}
		}
		return JSONResult(PathRulesResult{Path: target, Rules: values, Count: len(values)}), nil
	})
}

func workspaceOnlySchema(extra string) string {
	extra = strings.TrimSpace(extra)
	if extra != "" {
		extra = "," + strings.TrimSuffix(extra, ",")
	}
	return `{"type":"object","properties":{"workspace_id":{"type":"string"}` + extra + `},"required":["workspace_id"],"additionalProperties":false}`
}

func workspaceFromArgs(workspaces *workspace.Manager, args map[string]any) (workspace.Workspace, error) {
	workspaceID, err := requiredString(args, "workspace_id")
	if err != nil {
		return workspace.Workspace{}, err
	}
	return workspaces.Get(workspaceID)
}

func workspaceLocation(workspaces *workspace.Manager, args map[string]any) (workspace.Workspace, string, error) {
	item, err := workspaceFromArgs(workspaces, args)
	if err != nil {
		return workspace.Workspace{}, "", err
	}
	workingDirectory, err := optionalString(args, "working_directory")
	if err != nil {
		return workspace.Workspace{}, "", err
	}
	if strings.TrimSpace(workingDirectory) == "" {
		return item, item.Path, nil
	}
	_, cwd, err := workspaces.ResolveWorkingDirectory(item.ID, workingDirectory)
	return item, cwd, err
}
