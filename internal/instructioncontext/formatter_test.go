package instructioncontext

import (
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/rules"
	"go.mewis.me/chatgpt-mcp/internal/skills"
)

func TestFormatInstructionsStableOrderingAndByteCount(t *testing.T) {
	value := InstructionContext{
		ToolProfile: ToolProfile{Name: "full", Count: 54},
		Environment: EnvironmentSnapshot{
			Platform: "linux", OS: "linux", Arch: "amd64", Go: "go1.27.0", PID: 123,
			WorkspaceID: "ws_test", WorkspaceRoot: "/workspace", CWD: "/workspace/sub", EffectiveRoots: []string{"/workspace", "/shared"},
			Admin: AdminSnapshot{Enabled: true, URL: "http://127.0.0.1:37422/"},
		},
		Git:        GitSnapshot{IsRepo: true, Root: "/workspace", Branch: "main", StatusShort: "## main\n M a.go", RecentCommits: []string{"abc first", "def second"}},
		AutoMemory: AutoMemorySnapshot{Loaded: true, Content: "remember pnpm", Bytes: 13},
		ProjectMemory: ProjectMemoryBundle{Sections: []Section{
			{Path: "/home/user/.agents/AGENTS.md", Kind: SectionUser, Source: "agents", Content: "user instruction"},
			{Path: "/workspace/AGENTS.md", Kind: SectionProject, Source: "agents", Content: "project instruction"},
			{Path: "/workspace/CLAUDE.md", Kind: SectionProject, Source: "claude", Content: "claude fallback", Truncated: true},
		}},
		Rules:  []rules.Rule{{Path: "/workspace/.agents/rules/global.md", Source: ".agents", Content: "global rule"}},
		Skills: []skills.Skill{{Name: "release", Description: "Release workflow", Source: ".agents", Path: "/workspace/.agents/skills/release/SKILL.md"}},
	}
	text, size := FormatInstructions(value)
	if size != len([]byte(text)) {
		t.Fatalf("size = %d, bytes = %d", size, len([]byte(text)))
	}
	ordered := []string{
		"## Agent workflow", "## Tool profile", "## Environment", "## Git", "## Auto memory", "## User instructions", "## Project instructions", "## Always-on rules", "## Skills", "## Quick pointers",
	}
	last := -1
	for _, heading := range ordered {
		index := strings.Index(text, heading)
		if index < 0 || index <= last {
			t.Fatalf("heading %q out of order in:\n%s", heading, text)
		}
		last = index
	}
	for _, expected := range []string{
		"### /workspace/AGENTS.md [agents]\nproject instruction",
		"### /workspace/CLAUDE.md [claude] (truncated)\nclaude fallback",
		"### /workspace/.agents/rules/global.md [.agents]\nglobal rule",
		"- release: Release workflow [.agents] (/workspace/.agents/skills/release/SKILL.md)",
		"Load an applicable skill with load_skill",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("formatted instructions missing %q:\n%s", expected, text)
		}
	}
}

func TestFormatInstructionsDoesNotRenderImportMetadataTwice(t *testing.T) {
	marker := "<!-- @import /workspace/import.md -->\nimported body"
	value := InstructionContext{ProjectMemory: ProjectMemoryBundle{
		Sections: []Section{{Path: "/workspace/AGENTS.md", Kind: SectionProject, Source: "agents", Content: "root\n" + marker}},
		Imports:  []Section{{Path: "/workspace/import.md", Kind: SectionImport, Source: "import", Content: "imported body"}},
	}}
	text, _ := FormatInstructions(value)
	if strings.Count(text, "imported body") != 1 {
		t.Fatalf("import rendered more than once:\n%s", text)
	}
}

func TestFormatInstructionsOmitsEmptyOptionalBlocks(t *testing.T) {
	text, _ := FormatInstructions(InstructionContext{})
	for _, heading := range []string{"## Auto memory", "## User instructions", "## Project instructions", "## Always-on rules", "## Skills"} {
		if strings.Contains(text, heading) {
			t.Fatalf("unexpected empty block %q:\n%s", heading, text)
		}
	}
	for _, heading := range []string{"## Agent workflow", "## Tool profile", "## Environment", "## Git", "## Quick pointers"} {
		if !strings.Contains(text, heading) {
			t.Fatalf("missing required block %q:\n%s", heading, text)
		}
	}
}

func TestFormatInstructionsDetachedAndNonRepoGit(t *testing.T) {
	detached := formatGit(GitSnapshot{IsRepo: true, Root: "/workspace"})
	if !strings.Contains(detached, "- branch: (detached)") {
		t.Fatalf("detached git = %q", detached)
	}
	nonRepo := formatGit(GitSnapshot{Error: "git unavailable"})
	if !strings.Contains(nonRepo, "- repository: false") || !strings.Contains(nonRepo, "- error: git unavailable") {
		t.Fatalf("non repo git = %q", nonRepo)
	}
}

func TestApplyFormattedInstructionsDefaultsWorkflow(t *testing.T) {
	value := InstructionContext{ToolProfile: ToolProfile{Name: "full", Count: 1}}
	ApplyFormattedInstructions(&value)
	if value.AgentWorkflow != AgentWorkflow() || value.InstructionsText == "" || value.InstructionBytes != len([]byte(value.InstructionsText)) {
		t.Fatalf("value = %#v", value)
	}
	ApplyFormattedInstructions(nil)
}
