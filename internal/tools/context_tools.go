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
	"go.mewis.me/chatgpt-mcp/internal/instructioncontext"
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

type ProjectContextMemoryFile struct {
	Path      string                         `json:"path"`
	Kind      instructioncontext.SectionKind `json:"kind"`
	Source    string                         `json:"source,omitempty"`
	Truncated bool                           `json:"truncated"`
}

type ProjectContextGitSummary struct {
	Skipped bool   `json:"skipped,omitempty"`
	IsRepo  bool   `json:"is_repo"`
	Branch  string `json:"branch,omitempty"`
	Commits int    `json:"commits"`
}

type ProjectContextSummary struct {
	MemoryFiles      []ProjectContextMemoryFile `json:"memory_files"`
	MemoryBytes      int                        `json:"memory_bytes"`
	InstructionBytes int                        `json:"instruction_bytes"`
	Git              ProjectContextGitSummary   `json:"git"`
	Rules            int                        `json:"rules"`
	Skills           int                        `json:"skills"`
}

type ProjectContextResult struct {
	Root               string                                `json:"root"`
	WorkspaceID        string                                `json:"workspace_id"`
	InstructionContext instructioncontext.InstructionContext `json:"instruction_context"`
	Summary            ProjectContextSummary                 `json:"summary"`
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

	register("project_context", "Project Context", "Build the complete workspace instruction context with environment, Git, memory, rules, skills, and ready-to-use instructions.", workspaceOnlySchema(`"path":{"type":"string"},"max_instruction_bytes":{"type":"integer","minimum":1,"maximum":1000000,"default":100000},"max_section_bytes":{"type":"integer","minimum":1,"maximum":500000,"default":25000},"max_lines_per_section":{"type":"integer","minimum":1,"maximum":5000,"default":200},"include_git":{"type":"boolean","default":true},"include_memory":{"type":"boolean","default":true},"include_skills":{"type":"boolean","default":true},`), `{"type":"object","properties":{"root":{"type":"string"},"workspace_id":{"type":"string"},"instruction_context":{"type":"object","additionalProperties":true},"summary":{"type":"object","additionalProperties":true}},"required":["root","workspace_id","instruction_context","summary"],"additionalProperties":false}`, RiskRead, func(ctx context.Context, args map[string]any) (Result, error) {
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
		maxInstructionBytes, err := optionalInt(args, "max_instruction_bytes", instructioncontext.DefaultInstructionMaxBytes, 1, 1_000_000)
		if err != nil {
			return Result{}, err
		}
		maxSectionBytes, err := optionalInt(args, "max_section_bytes", instructioncontext.DefaultSectionMaxBytes, 1, 500_000)
		if err != nil {
			return Result{}, err
		}
		maxLinesPerSection, err := optionalInt(args, "max_lines_per_section", instructioncontext.DefaultSectionMaxLines, 1, 5_000)
		if err != nil {
			return Result{}, err
		}
		includeGit, err := optionalBool(args, "include_git", true)
		if err != nil {
			return Result{}, err
		}
		includeMemory, err := optionalBool(args, "include_memory", true)
		if err != nil {
			return Result{}, err
		}
		includeSkills, err := optionalBool(args, "include_skills", true)
		if err != nil {
			return Result{}, err
		}
		roots, err := workspaces.EffectiveRoots(item.ID)
		if err != nil {
			return Result{}, err
		}
		value, err := instructioncontext.Build(ctx, instructioncontext.BuildOptions{
			Root: root, WorkspaceID: item.ID, WorkspaceRoot: item.Path, CWD: cwd, WorkspaceRoots: roots, MemoryStore: memoryStore,
			Memory:      instructioncontext.MemoryLoadOptions{ImportMaxDepth: instructioncontext.DefaultImportMaxDepth, MaxBytesPerSection: maxSectionBytes, MaxLinesPerSection: maxLinesPerSection},
			ToolProfile: instructioncontext.ToolProfile{Name: "full", Count: len(registry.ListSchemas())}, MaxInstructionBytes: maxInstructionBytes,
			SkipGit: !includeGit, SkipMemory: !includeMemory, SkipSkills: !includeSkills,
		})
		if err != nil {
			return Result{}, err
		}
		return JSONResult(projectContextResult(value)), nil
	})

	register("agent_status", "Agent Status", "Show workspace permissions, runtime, rewind config, upstream configuration, and tool runtime status.", workspaceOnlySchema(``), `{"type":"object","additionalProperties":true}`, RiskRead, func(_ context.Context, args map[string]any) (Result, error) {
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
				"Filesystem and git paths resolve from the registered workspace root; shell commands use the persisted shell cwd.",
				"Read before editing; use apply_patch/edit_file for deterministic changes.",
				"Use workspace_status to inspect workspace root, shell cwd, and allowed directories.",
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

	register("load_path_rules", "Load Path Rules", "Load path-scoped rules from .claude/.claudes/.agents/.cursor/.codex rule directories.", workspaceOnlySchema(`"path":{"type":"string"},`), `{"type":"object","properties":{"path":{"type":"string"},"rules":{"type":"array","items":{"type":"object","additionalProperties":true}},"count":{"type":"integer"}},"required":["path","rules","count"],"additionalProperties":false}`, RiskRead, func(_ context.Context, args map[string]any) (Result, error) {
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

func projectContextResult(value instructioncontext.InstructionContext) ProjectContextResult {
	files := make([]ProjectContextMemoryFile, 0, len(value.ProjectMemory.Sections)+len(value.ProjectMemory.Imports))
	appendSection := func(section instructioncontext.Section) {
		files = append(files, ProjectContextMemoryFile{Path: section.Path, Kind: section.Kind, Source: section.Source, Truncated: section.Truncated})
	}
	for _, section := range value.ProjectMemory.Sections {
		appendSection(section)
	}
	for _, section := range value.ProjectMemory.Imports {
		appendSection(section)
	}
	return ProjectContextResult{
		Root: value.Root, WorkspaceID: value.WorkspaceID, InstructionContext: value,
		Summary: ProjectContextSummary{
			MemoryFiles: files, MemoryBytes: value.ProjectMemory.TotalBytes, InstructionBytes: value.InstructionBytes,
			Git:   ProjectContextGitSummary{Skipped: value.Git.Skipped, IsRepo: value.Git.IsRepo, Branch: value.Git.Branch, Commits: len(value.Git.RecentCommits)},
			Rules: len(value.Rules), Skills: len(value.Skills),
		},
	}
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
	return item, item.Path, nil
}
