package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/checkpoint"
	"go.mewis.me/chatgpt-mcp/internal/instructioncontext"
	"go.mewis.me/chatgpt-mcp/internal/instructionpolicy"
	"go.mewis.me/chatgpt-mcp/internal/memory"
	"go.mewis.me/chatgpt-mcp/internal/projectcontext"
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
	Scope   string `json:"scope"`
	Key     string `json:"key"`
	Note    string `json:"note"`
}

type MemoryGetResult struct {
	Entries []memory.Entry `json:"entries"`
	Count   int            `json:"count"`
}

type ForgetResult struct {
	Removed int    `json:"removed"`
	Scope   string `json:"scope"`
	Key     string `json:"key,omitempty"`
}

type MemorySearchMatch struct {
	Scope string  `json:"scope"`
	Key   string  `json:"key"`
	Note  string  `json:"note"`
	Score float64 `json:"score"`
}

type MemorySearchResult struct {
	Matches []MemorySearchMatch `json:"matches"`
	Count   int                 `json:"count"`
}

type OptimizeMemoryResult struct {
	Groups                  []memory.OptimizationGroup `json:"groups"`
	BeforeBytes             int                        `json:"before_bytes"`
	CandidateSavingsBytes   int                        `json:"candidate_savings_bytes"`
	LegacyFormat            bool                       `json:"legacy_format"`
	OptimizationRecommended bool                       `json:"optimization_recommended"`
	DryRun                  bool                       `json:"dry_run"`
}

type ProjectContextMemoryFile = projectcontext.MemoryFile
type ProjectContextGitSummary = projectcontext.GitSummary
type ProjectContextSummary = projectcontext.Summary
type ProjectContextResult = projectcontext.Result

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
	memoryIndex := memory.NewMemoryIndex()
	policyStore := instructionpolicy.DefaultStore()
	contextService := projectcontext.New(workspaces, func() instructioncontext.ToolProfile {
		return instructioncontext.ToolProfile{Name: "full", Count: len(registry.ListSchemas())}
	})
	contextService.MemoryStore = memoryStore
	contextService.PolicyStore = policyStore
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
		policy, err := policyStore.Load()
		if err != nil {
			return Result{}, err
		}
		home, _ := os.UserHomeDir()
		values, err := skills.DiscoverWithUser(item.Path, home, policy)
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
		policy, err := policyStore.Load()
		if err != nil {
			return Result{}, err
		}
		home, _ := os.UserHomeDir()
		value, err := skills.LoadWithUser(item.Path, home, name, maxBytes, policy)
		if err != nil {
			return Result{}, err
		}
		if _, err := workspaces.ResolvePath(item.ID, item.Path, value.Skill.Path, true); err != nil && !withinDirectory(home, value.Skill.Path) {
			return Result{}, fmt.Errorf("skill path: %w", err)
		}
		return JSONResult(value), nil
	})

	register("project_context", "Project Context", "Build the complete workspace instruction context with environment, Git, selected memory, rules, skills, and ready-to-use instructions.", workspaceOnlySchema(`"path":{"type":"string"},"memory_query":{"type":"string"},"max_memory_entries":{"type":"integer","minimum":1,"maximum":100,"default":12},"max_memory_bytes":{"type":"integer","minimum":256,"maximum":100000,"default":8192},"max_instruction_bytes":{"type":"integer","minimum":1,"maximum":1000000,"default":100000},"max_section_bytes":{"type":"integer","minimum":1,"maximum":500000,"default":25000},"max_lines_per_section":{"type":"integer","minimum":1,"maximum":5000,"default":200},"include_git":{"type":"boolean","default":true},"include_memory":{"type":"boolean","default":true},"include_skills":{"type":"boolean","default":true},`), `{"type":"object","properties":{"root":{"type":"string"},"workspace_id":{"type":"string"},"instruction_context":{"type":"object","additionalProperties":true},"summary":{"type":"object","additionalProperties":true}},"required":["root","workspace_id","instruction_context","summary"],"additionalProperties":false}`, RiskRead, func(ctx context.Context, args map[string]any) (Result, error) {
		item, err := workspaceFromArgs(workspaces, args)
		if err != nil {
			return Result{}, err
		}
		pathValue, err := optionalString(args, "path")
		memoryQuery, err := optionalString(args, "memory_query")
		if err != nil {
			return Result{}, err
		}
		maxMemoryEntries, err := optionalInt(args, "max_memory_entries", 12, 1, 100)
		if err != nil {
			return Result{}, err
		}
		maxMemoryBytes, err := optionalInt(args, "max_memory_bytes", 8192, 256, 100_000)
		if err != nil {
			return Result{}, err
		}
		if err != nil {
			return Result{}, err
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
		value, err := contextService.Build(ctx, item.ID, projectcontext.Options{
			Path: pathValue, MaxInstructionBytes: maxInstructionBytes, MaxSectionBytes: maxSectionBytes, MaxLinesPerSection: maxLinesPerSection,
			MemoryQuery: memoryQuery, MaxMemoryEntries: maxMemoryEntries, MaxMemoryBytes: maxMemoryBytes,
			IncludeGit: includeGit, IncludeMemory: includeMemory, IncludeSkills: includeSkills,
		})
		if err != nil {
			return Result{}, err
		}
		return JSONResult(value), nil
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

	register("remember", "Remember", "Upsert one canonical cross-session memory entry by scope and key. Read existing memory first; when updating an existing key, submit the complete canonical replacement note containing all still-relevant facts.", `{"type":"object","properties":{"workspace_id":{"type":"string"},"scope":{"type":"string"},"key":{"type":"string"},"note":{"type":"string"}},"required":["workspace_id","scope","key","note"],"additionalProperties":false}`, `{"type":"object","properties":{"saved_to":{"type":"string"},"scope":{"type":"string"},"key":{"type":"string"},"note":{"type":"string"}},"required":["saved_to","scope","key","note"],"additionalProperties":false}`, RiskEdit, func(_ context.Context, args map[string]any) (Result, error) {
		item, err := workspaceFromArgs(workspaces, args)
		if err != nil {
			return Result{}, err
		}
		scope, err := requiredString(args, "scope")
		if err != nil {
			return Result{}, err
		}
		key, err := requiredString(args, "key")
		if err != nil {
			return Result{}, err
		}
		note, err := requiredString(args, "note")
		if err != nil {
			return Result{}, err
		}
		path, err := memoryStore.Upsert(item.ID, scope, key, note)
		if err != nil {
			return Result{}, err
		}
		return JSONResult(RememberResult{SavedTo: path, Scope: strings.TrimSpace(scope), Key: strings.TrimSpace(key), Note: strings.TrimSpace(note)}), nil
	})

	register("memory_get", "Memory Get", "Read canonical cross-session memory entries. Omit filters for all entries, set scope for one scope, or set scope and key for one exact entry.", workspaceOnlySchema(`"scope":{"type":"string"},"key":{"type":"string"},`), `{"type":"object","properties":{"entries":{"type":"array","items":{"type":"object","properties":{"scope":{"type":"string"},"key":{"type":"string"},"note":{"type":"string"}},"required":["scope","key","note"],"additionalProperties":false}},"count":{"type":"integer"}},"required":["entries","count"],"additionalProperties":false}`, RiskRead, func(_ context.Context, args map[string]any) (Result, error) {
		item, err := workspaceFromArgs(workspaces, args)
		if err != nil {
			return Result{}, err
		}
		scope, err := optionalString(args, "scope")
		if err != nil {
			return Result{}, err
		}
		key, err := optionalString(args, "key")
		if err != nil {
			return Result{}, err
		}
		entries, err := memoryStore.Get(item.ID, scope, key)
		if err != nil {
			return Result{}, err
		}
		return JSONResult(MemoryGetResult{Entries: entries, Count: len(entries)}), nil
	})

	register("forget", "Forget", "Remove canonical cross-session memory by exact scope and optional key. Scope only removes the entire scope; fuzzy deletion is not supported.", workspaceOnlySchema(`"scope":{"type":"string"},"key":{"type":"string"},`), `{"type":"object","properties":{"removed":{"type":"integer"},"scope":{"type":"string"},"key":{"type":"string"}},"required":["removed","scope"],"additionalProperties":false}`, RiskEdit, func(_ context.Context, args map[string]any) (Result, error) {
		item, err := workspaceFromArgs(workspaces, args)
		if err != nil {
			return Result{}, err
		}
		scope, err := requiredString(args, "scope")
		if err != nil {
			return Result{}, err
		}
		key, err := optionalString(args, "key")
		if err != nil {
			return Result{}, err
		}
		removed, _, err := memoryStore.Remove(item.ID, scope, key)
		if err != nil {
			return Result{}, err
		}
		return JSONResult(ForgetResult{Removed: removed, Scope: strings.TrimSpace(scope), Key: strings.TrimSpace(key)}), nil
	})

	register("memory_search", "Memory Search", "Search canonical cross-session memory by lexical relevance with optional scope filtering. Results rank key matches above scope and note matches.", workspaceOnlySchema(`"query":{"type":"string"},"scope":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":50,"default":5},`), `{"type":"object","properties":{"matches":{"type":"array","items":{"type":"object","properties":{"scope":{"type":"string"},"key":{"type":"string"},"note":{"type":"string"},"score":{"type":"number"}},"required":["scope","key","note","score"],"additionalProperties":false}},"count":{"type":"integer"}},"required":["matches","count"],"additionalProperties":false}`, RiskRead, func(_ context.Context, args map[string]any) (Result, error) {
		item, err := workspaceFromArgs(workspaces, args)
		if err != nil {
			return Result{}, err
		}
		query, err := requiredString(args, "query")
		if err != nil {
			return Result{}, err
		}
		scope, err := optionalString(args, "scope")
		if err != nil {
			return Result{}, err
		}
		limit, err := optionalInt(args, "limit", 5, 1, 50)
		if err != nil {
			return Result{}, err
		}
		document, err := memoryStore.LoadDocument(item.ID)
		if err != nil {
			return Result{}, err
		}
		if err := memoryIndex.Rebuild(item.ID, document.Entries); err != nil {
			return Result{}, err
		}
		found, err := memoryIndex.Search(item.ID, memory.Query{Text: query, Scope: scope, Limit: limit})
		if err != nil {
			return Result{}, err
		}
		matches := make([]MemorySearchMatch, 0, len(found))
		for _, match := range found {
			matches = append(matches, MemorySearchMatch{Scope: match.Entry.Scope, Key: match.Entry.Key, Note: match.Entry.Note, Score: match.Score})
		}
		return JSONResult(MemorySearchResult{Matches: matches, Count: len(matches)}), nil
	})

	register("optimize_memory", "Optimize Memory", "Analyze canonical memory for legacy format, oversized notes, fragmented keys, and high-overlap candidates. This phase is analysis-only and never rewrites semantic memory; reconcile candidates with remember/forget.", workspaceOnlySchema(`"scope":{"type":"string"},"dry_run":{"type":"boolean","default":true},`), `{"type":"object","properties":{"groups":{"type":"array","items":{"type":"object","additionalProperties":true}},"before_bytes":{"type":"integer"},"candidate_savings_bytes":{"type":"integer"},"legacy_format":{"type":"boolean"},"optimization_recommended":{"type":"boolean"},"dry_run":{"type":"boolean"}},"required":["groups","before_bytes","candidate_savings_bytes","legacy_format","optimization_recommended","dry_run"],"additionalProperties":false}`, RiskRead, func(_ context.Context, args map[string]any) (Result, error) {
		item, err := workspaceFromArgs(workspaces, args)
		if err != nil {
			return Result{}, err
		}
		scope, err := optionalString(args, "scope")
		if err != nil {
			return Result{}, err
		}
		if _, err := optionalBool(args, "dry_run", true); err != nil {
			return Result{}, err
		}
		analysis, err := memoryStore.Analyze(item.ID, scope)
		if err != nil {
			return Result{}, err
		}
		return JSONResult(OptimizeMemoryResult{Groups: analysis.Groups, BeforeBytes: analysis.BeforeBytes, CandidateSavingsBytes: analysis.CandidateSavingsBytes, LegacyFormat: analysis.LegacyFormat, OptimizationRecommended: analysis.OptimizationRecommended, DryRun: true}), nil
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
		policy, err := policyStore.Load()
		if err != nil {
			return Result{}, err
		}
		home, _ := os.UserHomeDir()
		values, err := rules.LoadForFileWithUser(item.Path, target, home, policy)
		if err != nil {
			return Result{}, err
		}
		for _, rule := range values {
			if _, err := workspaces.ResolvePath(item.ID, item.Path, rule.Path, true); err != nil && !withinDirectory(home, rule.Path) {
				return Result{}, fmt.Errorf("rule path: %w", err)
			}
		}
		return JSONResult(PathRulesResult{Path: target, Rules: values, Count: len(values)}), nil
	})
}

func withinDirectory(root, path string) bool {
	root, path = strings.TrimSpace(root), strings.TrimSpace(path)
	if root == "" || path == "" {
		return false
	}
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
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
