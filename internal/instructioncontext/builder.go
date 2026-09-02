package instructioncontext

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/memory"
)

type BuildOptions struct {
	Root           string
	WorkspaceID    string
	WorkspaceRoot  string
	CWD            string
	WorkspaceRoots []string
	MemoryStore    memory.Store
	Memory         MemoryLoadOptions
	ToolProfile    ToolProfile
	AdminEnabled   bool
	AdminPort      int
	Now            func() time.Time
}

func Build(ctx context.Context, opts BuildOptions) (InstructionContext, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	workspaceID := strings.TrimSpace(opts.WorkspaceID)
	if workspaceID == "" {
		return InstructionContext{}, errors.New("workspace id is required")
	}
	root, err := canonicalEnvironmentPath(opts.Root)
	if err != nil {
		return InstructionContext{}, err
	}
	workspaceRoot, err := canonicalEnvironmentPath(opts.WorkspaceRoot)
	if err != nil {
		return InstructionContext{}, err
	}
	roots := normalizeEnvironmentRoots(opts.WorkspaceRoots)
	if len(roots) == 0 {
		roots = []string{workspaceRoot}
	}
	if !withinAnyRoot(roots, root) {
		return InstructionContext{}, errors.New("project root is outside effective workspace roots")
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	loadedAt := now().UTC()
	environment, err := LoadEnvironmentSnapshot(EnvironmentOptions{
		WorkspaceID: workspaceID, WorkspaceRoot: workspaceRoot, CWD: opts.CWD, EffectiveRoots: roots,
		AdminEnabled: opts.AdminEnabled, AdminPort: opts.AdminPort,
	})
	if err != nil {
		return InstructionContext{}, err
	}
	memoryOpts := opts.Memory
	memoryOpts.WorkspaceRoots = roots
	memoryOpts.Now = func() time.Time { return loadedAt }
	projectMemory, err := LoadProjectMemory(root, memoryOpts)
	if err != nil {
		return InstructionContext{}, err
	}
	autoMemory, err := LoadAutoMemory(opts.MemoryStore, workspaceID)
	if err != nil {
		return InstructionContext{}, err
	}
	unconditionalRules, err := LoadUnconditionalRules(root)
	if err != nil {
		return InstructionContext{}, err
	}
	skillSummaries, err := LoadSkillSummaries(root)
	if err != nil {
		return InstructionContext{}, err
	}
	value := InstructionContext{
		Root: root, WorkspaceID: workspaceID, WorkspaceRoots: roots, Environment: environment,
		Git: LoadGitSnapshot(ctx, root, GitSnapshotOptions{WorkspaceRoots: roots}), ProjectMemory: projectMemory,
		AutoMemory: autoMemory, Rules: unconditionalRules, Skills: skillSummaries, ToolProfile: opts.ToolProfile,
		AgentWorkflow: AgentWorkflow(), LoadedAt: loadedAt,
	}
	ApplyFormattedInstructions(&value)
	return value, nil
}
