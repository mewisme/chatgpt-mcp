package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/checkpoint"
	"go.mewis.me/chatgpt-mcp/internal/instructionpolicy"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func newContextToolRuntime(t *testing.T) (*Runtime, string, string, *checkpoint.Store) {
	t.Helper()
	t.Setenv("CHATGPT_MCP_CONFIG_DIR", t.TempDir())
	testHome := t.TempDir()
	t.Setenv("HOME", testHome)
	t.Setenv("USERPROFILE", testHome)
	root := t.TempDir()
	workspaces := workspace.NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	item, err := workspaces.Register(root)
	if err != nil {
		t.Fatal(err)
	}
	checkpoints := checkpoint.NewStore(filepath.Join(t.TempDir(), "state"))
	registry := NewRegistry()
	RegisterWorkspaceTools(registry, workspaces)
	RegisterContextTools(registry, workspaces, checkpoints)
	RegisterRewindTools(registry, workspaces, checkpoints)
	return &Runtime{Registry: registry, Workspaces: workspaces, Checkpoints: checkpoints}, item.ID, item.Path, checkpoints
}

func TestContextSkillsRulesAndRemember(t *testing.T) {
	runtime, workspaceID, root, _ := newContextToolRuntime(t)
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("instructions"), 0644); err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(root, ".agents", "skills", "test")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: test\ndescription: test skill\n---\nbody"), 0644); err != nil {
		t.Fatal(err)
	}
	ruleDir := filepath.Join(root, ".cursor", "rules")
	if err := os.MkdirAll(ruleDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ruleDir, "ts.mdc"), []byte("---\nglobs: [\"**/*.ts\"]\n---\nTS rule"), 0644); err != nil {
		t.Fatal(err)
	}
	globalRuleDir := filepath.Join(root, ".agents", "rules")
	if err := os.MkdirAll(globalRuleDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalRuleDir, "global.md"), []byte("Global rule"), 0644); err != nil {
		t.Fatal(err)
	}

	ctxResult, err := runtime.Call(context.Background(), "project_context", map[string]any{"workspace_id": workspaceID})
	if err != nil || ctxResult.IsError {
		t.Fatalf("project_context failed: %#v %v", ctxResult, err)
	}
	if ctxResult.StructuredContent == nil {
		t.Fatal("missing project context")
	}
	project := ctxResult.StructuredContent.(ProjectContextResult)
	if project.Root != root || project.WorkspaceID != workspaceID || project.Summary.MemoryBytes == 0 || project.Summary.InstructionBytes != project.InstructionContext.InstructionBytes {
		t.Fatalf("project context = %#v", project)
	}
	if project.Summary.Rules != 1 || project.Summary.Skills != 1 || len(project.Summary.MemoryFiles) != 1 {
		t.Fatalf("project summary = %#v", project.Summary)
	}
	if !strings.Contains(project.InstructionContext.InstructionsText, "instructions") || !strings.Contains(project.InstructionContext.InstructionsText, "Global rule") || !strings.Contains(project.InstructionContext.InstructionsText, "test skill") {
		t.Fatalf("instructions = %q", project.InstructionContext.InstructionsText)
	}
	if strings.Contains(project.InstructionContext.InstructionsText, "\nbody") {
		t.Fatalf("skill body leaked into project context: %q", project.InstructionContext.InstructionsText)
	}

	listResult, err := runtime.Call(context.Background(), "list_skills", map[string]any{"workspace_id": workspaceID})
	if err != nil || listResult.IsError {
		t.Fatalf("list_skills failed: %#v %v", listResult, err)
	}
	if listResult.StructuredContent.(SkillsListResult).Count != 1 {
		t.Fatalf("skills = %#v", listResult.StructuredContent)
	}

	loadResult, err := runtime.Call(context.Background(), "load_skill", map[string]any{"workspace_id": workspaceID, "name": "test"})
	if err != nil || loadResult.IsError || !strings.Contains(loadResult.Content[0].Text, "body") {
		t.Fatalf("load_skill failed: %#v %v", loadResult, err)
	}

	rulesResult, err := runtime.Call(context.Background(), "load_path_rules", map[string]any{"workspace_id": workspaceID, "path": "src/app.ts"})
	if err != nil || rulesResult.IsError {
		t.Fatalf("load_path_rules failed: %#v %v", rulesResult, err)
	}
	if rulesResult.StructuredContent.(PathRulesResult).Count != 1 {
		t.Fatalf("rules = %#v", rulesResult.StructuredContent)
	}

	rememberResult, err := runtime.Call(context.Background(), "remember", map[string]any{"workspace_id": workspaceID, "scope": "coding-style", "note": "use compact imports"})
	if err != nil || rememberResult.IsError {
		t.Fatalf("remember failed: %#v %v", rememberResult, err)
	}
	remembered := rememberResult.StructuredContent.(RememberResult)
	if remembered.Scope != "coding-style" || remembered.Note != "use compact imports" {
		t.Fatalf("remember result = %#v", remembered)
	}
	rememberResult, err = runtime.Call(context.Background(), "remember", map[string]any{"workspace_id": workspaceID, "scope": "coding-style", "note": "use compact imports and keep imports contiguous"})
	if err != nil || rememberResult.IsError {
		t.Fatalf("remember update failed: %#v %v", rememberResult, err)
	}
	ctxAfterRemember, err := runtime.Call(context.Background(), "project_context", map[string]any{"workspace_id": workspaceID})
	if err != nil || ctxAfterRemember.IsError {
		t.Fatalf("project_context after remember failed: %#v %v", ctxAfterRemember, err)
	}
	after := ctxAfterRemember.StructuredContent.(ProjectContextResult)
	if !after.InstructionContext.AutoMemory.Loaded || !strings.Contains(after.InstructionContext.InstructionsText, "## coding-style\n- use compact imports and keep imports contiguous") || strings.Count(after.InstructionContext.AutoMemory.Content, "## coding-style") != 1 {
		t.Fatalf("auto memory not included: %#v", after.InstructionContext.AutoMemory)
	}
}

func TestContextToolsApplyManagedGlobalPolicyToUserSources(t *testing.T) {
	runtime, workspaceID, _, _ := newContextToolRuntime(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".claude", "rules"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude", "CLAUDE.md"), []byte("USER CLAUDE CONTEXT"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude", "rules", "ts.md"), []byte("---\nglobs: [\"**/*.ts\"]\n---\nUSER CLAUDE RULE"), 0644); err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(home, ".claude", "skills", "user-review")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: user-review\ndescription: USER CLAUDE SKILL\n---\nsecret body"), 0644); err != nil {
		t.Fatal(err)
	}
	disabled := false
	policy := instructionpolicy.DefaultConfig()
	policy.Context = "MANAGED GLOBAL CONTEXT"
	policy.Rules = []instructionpolicy.GlobalRule{{ID: "managed", Enabled: true, Content: "MANAGED GLOBAL RULE"}}
	policy.Sources["claude"] = instructionpolicy.SourcePolicy{Context: &disabled, Rules: &disabled, Skills: &disabled}
	if err := instructionpolicy.DefaultStore().Save(policy); err != nil {
		t.Fatal(err)
	}

	ctxResult, err := runtime.Call(context.Background(), "project_context", map[string]any{"workspace_id": workspaceID, "include_git": false})
	if err != nil || ctxResult.IsError {
		t.Fatalf("project_context failed: %#v %v", ctxResult, err)
	}
	project := ctxResult.StructuredContent.(ProjectContextResult)
	for _, expected := range []string{"MANAGED GLOBAL CONTEXT", "MANAGED GLOBAL RULE"} {
		if !strings.Contains(project.InstructionContext.InstructionsText, expected) {
			t.Fatalf("missing %q in instructions: %s", expected, project.InstructionContext.InstructionsText)
		}
	}
	for _, unexpected := range []string{"USER CLAUDE CONTEXT", "USER CLAUDE RULE", "USER CLAUDE SKILL"} {
		if strings.Contains(project.InstructionContext.InstructionsText, unexpected) {
			t.Fatalf("disabled source leaked %q: %s", unexpected, project.InstructionContext.InstructionsText)
		}
	}
	if len(project.InstructionContext.Sources) != 3 {
		t.Fatalf("sources = %#v", project.InstructionContext.Sources)
	}
	for _, source := range project.InstructionContext.Sources {
		if source.Provider != "claude" || source.Enabled || source.Loaded {
			t.Fatalf("disabled source snapshot = %#v", source)
		}
	}

	listResult, err := runtime.Call(context.Background(), "list_skills", map[string]any{"workspace_id": workspaceID})
	if err != nil || listResult.IsError {
		t.Fatalf("list_skills failed: %#v %v", listResult, err)
	}
	if listResult.StructuredContent.(SkillsListResult).Count != 0 {
		t.Fatalf("disabled user skill leaked: %#v", listResult.StructuredContent)
	}
	loadResult, err := runtime.Call(context.Background(), "load_skill", map[string]any{"workspace_id": workspaceID, "name": "user-review"})
	if err != nil || !loadResult.IsError {
		t.Fatalf("disabled load_skill bypassed policy: %#v %v", loadResult, err)
	}
	rulesResult, err := runtime.Call(context.Background(), "load_path_rules", map[string]any{"workspace_id": workspaceID, "path": "src/app.ts"})
	if err != nil || rulesResult.IsError || rulesResult.StructuredContent.(PathRulesResult).Count != 0 {
		t.Fatalf("disabled user rule leaked: %#v %v", rulesResult, err)
	}
}

func TestProjectContextOutputSchemaUsesInstructionBundle(t *testing.T) {
	runtime, _, _, _ := newContextToolRuntime(t)
	schema, ok := runtime.Registry.Schema("project_context")
	if !ok {
		t.Fatal("missing project_context schema")
	}
	input := string(schema.InputSchema)
	for _, expected := range []string{"\"max_instruction_bytes\"", "\"max_section_bytes\"", "\"max_lines_per_section\"", "\"include_git\"", "\"include_memory\"", "\"include_skills\""} {
		if !strings.Contains(input, expected) {
			t.Fatalf("input schema missing %s: %s", expected, input)
		}
	}
	for _, legacy := range []string{"\"max_depth\"", "\"max_bytes_per_file\""} {
		if strings.Contains(input, legacy) {
			t.Fatalf("legacy project_context input remains %s: %s", legacy, input)
		}
	}
	output := string(schema.OutputSchema)
	for _, expected := range []string{"\"root\"", "\"workspace_id\"", "\"instruction_context\"", "\"summary\""} {
		if !strings.Contains(output, expected) {
			t.Fatalf("output schema missing %s: %s", expected, output)
		}
	}
	if strings.Contains(output, "\"count\"") || strings.Contains(output, "\"files\":") {
		t.Fatalf("legacy project_context output remains: %s", output)
	}
}

func TestRememberSchemaRequiresScopeAndNote(t *testing.T) {
	runtime, _, _, _ := newContextToolRuntime(t)
	schema, ok := runtime.Registry.Schema("remember")
	if !ok {
		t.Fatal("missing remember schema")
	}
	input := string(schema.InputSchema)
	for _, expected := range []string{`"workspace_id"`, `"scope"`, `"note"`, `"required":["workspace_id","scope","note"]`} {
		if !strings.Contains(input, expected) {
			t.Fatalf("remember input schema missing %s: %s", expected, input)
		}
	}
}

func TestProjectContextInputControlsCollectorsAndLimits(t *testing.T) {
	runtime, workspaceID, root, _ := newContextToolRuntime(t)
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("first line\nsecond line with more content"), 0644); err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(root, ".agents", "skills", "test")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: test\ndescription: test skill\n---\nbody"), 0644); err != nil {
		t.Fatal(err)
	}

	skippedResult, err := runtime.Call(context.Background(), "project_context", map[string]any{
		"workspace_id": workspaceID, "include_git": false, "include_memory": false, "include_skills": false,
	})
	if err != nil || skippedResult.IsError {
		t.Fatalf("project_context skipped collectors failed: %#v %v", skippedResult, err)
	}
	skipped := skippedResult.StructuredContent.(ProjectContextResult)
	if !skipped.InstructionContext.Git.Skipped || !skipped.Summary.Git.Skipped || len(skipped.InstructionContext.ProjectMemory.Sections) != 0 || len(skipped.InstructionContext.Skills) != 0 {
		t.Fatalf("skipped context = %#v", skipped)
	}
	for _, heading := range []string{"## Git", "## Project instructions", "## User instructions", "## Skills"} {
		if strings.Contains(skipped.InstructionContext.InstructionsText, heading) {
			t.Fatalf("skipped heading %q rendered: %s", heading, skipped.InstructionContext.InstructionsText)
		}
	}

	limitedResult, err := runtime.Call(context.Background(), "project_context", map[string]any{
		"workspace_id": workspaceID, "max_instruction_bytes": 256, "max_section_bytes": 8, "max_lines_per_section": 1,
	})
	if err != nil || limitedResult.IsError {
		t.Fatalf("project_context limits failed: %#v %v", limitedResult, err)
	}
	limited := limitedResult.StructuredContent.(ProjectContextResult)
	if len(limited.InstructionContext.ProjectMemory.Sections) != 1 {
		t.Fatalf("memory = %#v", limited.InstructionContext.ProjectMemory)
	}
	section := limited.InstructionContext.ProjectMemory.Sections[0]
	if !section.Truncated || section.LoadedBytes > 8 || strings.Contains(section.Content, "second line") {
		t.Fatalf("section = %#v", section)
	}
	if !limited.InstructionContext.InstructionTruncated || limited.InstructionContext.InstructionBytes > 256 {
		t.Fatalf("instruction limit = %#v", limited.InstructionContext)
	}
}

func TestRewindPreviewAndRestore(t *testing.T) {
	runtime, workspaceID, root, checkpoints := newContextToolRuntime(t)
	file := filepath.Join(root, "file.txt")
	if err := os.WriteFile(file, []byte("before"), 0644); err != nil {
		t.Fatal(err)
	}
	id, err := checkpoints.Before(workspaceID, root, "edit_file", []string{file}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("after"), 0644); err != nil {
		t.Fatal(err)
	}

	preview, err := runtime.Call(context.Background(), "rewind", map[string]any{"workspace_id": workspaceID, "action": "preview", "checkpoint_id": id})
	if err != nil || preview.IsError {
		t.Fatalf("preview failed: %#v %v", preview, err)
	}

	restore, err := runtime.Call(context.Background(), "rewind", map[string]any{"workspace_id": workspaceID, "action": "restore", "checkpoint_id": id})
	if err != nil || restore.IsError {
		t.Fatalf("restore failed: %#v %v", restore, err)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "before" {
		t.Fatalf("content = %q", data)
	}
}

func TestProjectContextUsesSelectedSubprojectRoot(t *testing.T) {
	runtime, workspaceID, root, _ := newContextToolRuntime(t)
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("root instruction must stay out"), 0644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "packages", "app")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "AGENTS.md"), []byte("subproject instruction"), 0644); err != nil {
		t.Fatal(err)
	}
	ruleDir := filepath.Join(sub, ".agents", "rules")
	if err := os.MkdirAll(ruleDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ruleDir, "global.md"), []byte("subproject global rule"), 0644); err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(sub, ".agents", "skills", "subskill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: subskill\ndescription: subproject skill\n---\nbody must stay out"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := runtime.Call(context.Background(), "project_context", map[string]any{"workspace_id": workspaceID, "path": "packages/app", "include_git": false})
	if err != nil || result.IsError {
		t.Fatalf("project_context subproject failed: %#v %v", result, err)
	}
	project := result.StructuredContent.(ProjectContextResult)
	if project.Root != sub || project.InstructionContext.Root != sub || project.InstructionContext.Environment.WorkspaceRoot != root {
		t.Fatalf("subproject roots = %#v", project)
	}
	if len(project.InstructionContext.ProjectMemory.Sections) != 1 || project.InstructionContext.ProjectMemory.Sections[0].Path != filepath.Join(sub, "AGENTS.md") {
		t.Fatalf("subproject memory = %#v", project.InstructionContext.ProjectMemory)
	}
	for _, expected := range []string{"subproject instruction", "subproject global rule", "subproject skill"} {
		if !strings.Contains(project.InstructionContext.InstructionsText, expected) {
			t.Fatalf("subproject instructions missing %q: %s", expected, project.InstructionContext.InstructionsText)
		}
	}
	for _, unexpected := range []string{"root instruction must stay out", "body must stay out"} {
		if strings.Contains(project.InstructionContext.InstructionsText, unexpected) {
			t.Fatalf("subproject instructions leaked %q: %s", unexpected, project.InstructionContext.InstructionsText)
		}
	}
}

func TestProjectContextRejectsPathOutsideWorkspace(t *testing.T) {
	runtime, workspaceID, root, _ := newContextToolRuntime(t)
	outside := filepath.Join(filepath.Dir(root), "outside")
	if err := os.MkdirAll(outside, 0755); err != nil {
		t.Fatal(err)
	}
	result, err := runtime.Call(context.Background(), "project_context", map[string]any{"workspace_id": workspaceID, "path": outside})
	if err == nil && !result.IsError {
		t.Fatalf("outside project_context path was accepted: %#v", result)
	}
}
