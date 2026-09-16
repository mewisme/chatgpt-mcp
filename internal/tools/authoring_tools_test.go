package tools

import (
	"encoding/base64"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/nativeinstruction"
	"go.mewis.me/chatgpt-mcp/internal/plugin"
	"go.mewis.me/chatgpt-mcp/internal/skills"
)

func TestCreateSkillAndRuleTools(t *testing.T) {
	runtime, workspaceID, root, _ := newContextToolRuntime(t)
	for _, name := range []string{"create_skill", "create_rule"} {
		schema, ok := runtime.Registry.Schema(name)
		if !ok {
			t.Fatalf("missing %s", name)
		}
		if schema.Annotations["readOnlyHint"] != false || schema.Annotations["destructiveHint"] != false {
			t.Fatalf("%s annotations = %#v", name, schema.Annotations)
		}
		if !strings.Contains(string(schema.InputSchema), `"additionalProperties":false`) {
			t.Fatalf("%s schema allows extra properties", name)
		}
	}

	unknown := callTool(t, runtime, "create_skill", map[string]any{
		"workspace_id": workspaceID, "scope": "workspace", "name": "ok", "description": "d", "instructions": "i", "destination": "/tmp",
	})
	if !unknown.IsError || !strings.Contains(unknown.Content[0].Text, "unknown property") {
		t.Fatalf("extra property = %#v", unknown)
	}
	badName := callTool(t, runtime, "create_skill", map[string]any{
		"workspace_id": workspaceID, "scope": "workspace", "name": "Bad Name", "description": "d", "instructions": "i",
	})
	if !badName.IsError {
		t.Fatalf("malformed name accepted: %#v", badName)
	}

	dry := callTool(t, runtime, "create_skill", map[string]any{
		"workspace_id": workspaceID, "scope": "workspace", "name": "ship-it", "description": "Ship it", "instructions": "Ship.", "dry_run": true,
	})
	if dry.IsError {
		t.Fatalf("dry_run failed: %#v", dry)
	}
	if _, err := os.Stat(filepath.Join(root, ".cgm", "skills", "ship-it")); !os.IsNotExist(err) {
		t.Fatal("dry_run wrote skill")
	}

	created := callTool(t, runtime, "create_skill", map[string]any{
		"workspace_id": workspaceID, "scope": "workspace", "name": "ship-it", "description": "Ship it", "instructions": "Ship after tests.",
		"files": []any{map[string]any{"path": "references/note.md", "content": "keep", "encoding": "utf8"}},
	})
	if created.IsError {
		t.Fatalf("create_skill failed: %#v", created)
	}
	skill := created.StructuredContent.(nativeinstruction.SkillResult)
	if skill.Mode != "create" || !strings.HasPrefix(skill.Directory, root) {
		t.Fatalf("skill result = %#v", skill)
	}
	exists := callTool(t, runtime, "create_skill", map[string]any{
		"workspace_id": workspaceID, "scope": "workspace", "name": "ship-it", "description": "Ship it", "instructions": "no",
	})
	if !exists.IsError {
		t.Fatal("create overwrite succeeded")
	}
	missing := callTool(t, runtime, "create_skill", map[string]any{
		"workspace_id": workspaceID, "scope": "workspace", "mode": "update", "name": "missing", "description": "d", "instructions": "i",
	})
	if !missing.IsError {
		t.Fatal("update missing succeeded")
	}
	updated := callTool(t, runtime, "create_skill", map[string]any{
		"workspace_id": workspaceID, "scope": "workspace", "mode": "update", "name": "ship-it", "description": "Ship it", "instructions": "Ship after tests.",
		"files":        []any{map[string]any{"path": "assets/icon.bin", "content": base64.StdEncoding.EncodeToString([]byte("png")), "encoding": "base64"}},
		"remove_files": []any{"references/note.md"},
	})
	if updated.IsError {
		t.Fatalf("update failed: %#v", updated)
	}
	if _, err := os.Stat(filepath.Join(skill.Directory, "references", "note.md")); !os.IsNotExist(err) {
		t.Fatal("removed file remains")
	}
	if _, err := os.Stat(filepath.Join(skill.Directory, "assets", "icon.bin")); err != nil {
		t.Fatal(err)
	}

	ruleBad := callTool(t, runtime, "create_rule", map[string]any{
		"workspace_id": workspaceID, "scope": "workspace", "name": "ts", "description": "TS", "always_apply": true, "globs": []any{"**/*.ts"}, "content": "types",
	})
	if !ruleBad.IsError {
		t.Fatal("always_apply with globs accepted")
	}
	rule := callTool(t, runtime, "create_rule", map[string]any{
		"workspace_id": workspaceID, "scope": "workspace", "name": "ts", "description": "TS conventions", "always_apply": false, "globs": []any{"**/*.ts"}, "content": "Prefer explicit types.",
	})
	if rule.IsError {
		t.Fatalf("create_rule failed: %#v", rule)
	}
	ruleResult := rule.StructuredContent.(nativeinstruction.RuleResult)
	if !strings.HasPrefix(ruleResult.Path, filepath.Join(root, ".cgm", "rules")) || !strings.HasSuffix(ruleResult.Path, "ts.md") {
		t.Fatalf("rule path = %s", ruleResult.Path)
	}

	global := callTool(t, runtime, "create_rule", map[string]any{
		"workspace_id": workspaceID, "scope": "global", "name": "house", "description": "House style", "always_apply": true, "content": "Be concise.",
	})
	if global.IsError {
		t.Fatalf("global create_rule failed: %#v", global)
	}
	globalResult := global.StructuredContent.(nativeinstruction.RuleResult)
	if !strings.HasPrefix(globalResult.Path, configformat.RootPath()) {
		t.Fatalf("global rule escaped config root: %s", globalResult.Path)
	}

	list := callTool(t, runtime, "list_skills", map[string]any{"workspace_id": workspaceID})
	if list.IsError {
		t.Fatalf("list_skills failed: %#v", list)
	}
	listed := list.StructuredContent.(SkillsListResult)
	byName := map[string]skills.Skill{}
	for _, item := range listed.Skills {
		byName[item.Name] = item
	}
	if byName["create-skill"].Source != skills.BuiltinSource || byName["create-rule"].Source != skills.BuiltinSource {
		t.Fatalf("builtins missing: %#v", listed.Skills)
	}
	loaded := callTool(t, runtime, "load_skill", map[string]any{"workspace_id": workspaceID, "name": "create-skill"})
	if loaded.IsError || !strings.Contains(loaded.Content[0].Text, "create_skill") || !strings.Contains(loaded.Content[0].Text, "Do not fall back") {
		t.Fatalf("load_skill builtin = %#v", loaded)
	}
	ruleSkill := callTool(t, runtime, "load_skill", map[string]any{"workspace_id": workspaceID, "name": "create-rule"})
	if ruleSkill.IsError || !strings.Contains(ruleSkill.Content[0].Text, "create_rule") || strings.Contains(ruleSkill.Content[0].Text, ".cursor/rules") {
		t.Fatalf("load_skill create-rule = %#v", ruleSkill)
	}
}

func TestBuiltinSkillsReserveNames(t *testing.T) {
	runtime, workspaceID, root, _ := newContextToolRuntime(t)
	dir := filepath.Join(root, ".agents", "skills", "create-skill")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: create-skill\ndescription: shadow\n---\nshadow body\n"), 0644); err != nil {
		t.Fatal(err)
	}
	list := callTool(t, runtime, "list_skills", map[string]any{"workspace_id": workspaceID})
	listed := list.StructuredContent.(SkillsListResult)
	for _, skill := range listed.Skills {
		if skill.Name == "create-skill" && skill.Source != skills.BuiltinSource {
			t.Fatalf("builtin shadowed: %#v", skill)
		}
	}
	loaded := callTool(t, runtime, "load_skill", map[string]any{"workspace_id": workspaceID, "name": "create-skill"})
	if loaded.IsError || strings.Contains(loaded.Content[0].Text, "shadow body") {
		t.Fatalf("shadow loaded: %#v", loaded)
	}
}

func TestCreateSkillRejectsPluginOwnedProjection(t *testing.T) {
	runtime, workspaceID, _, _ := newContextToolRuntime(t)
	store, err := plugin.NewStore(plugin.DefaultLayout(), plugin.RuntimeContext{OS: goruntime.GOOS, Arch: goruntime.GOARCH, CoreVersion: "0.2.24"})
	if err != nil {
		t.Fatal(err)
	}
	payload := filepath.Join("..", "plugin", "testdata", "instruction-resources")
	platform := goruntime.GOOS + "/" + goruntime.GOARCH
	manifest := plugin.Manifest{
		Schema: plugin.ManifestSchema, ID: "demo", Name: "demo", Publisher: "mewisme", License: "Apache-2.0", Version: "1.0.0", Type: "runtime",
		Provides: []plugin.Capability{"formatter/demo"}, Permissions: []plugin.Permission{plugin.PermissionProcessExecute},
		Platforms: map[string]plugin.PlatformArtifact{platform: {Artifact: "demo.tar.gz", SHA256: strings.Repeat("a", 64), Archive: "tar.gz", Entrypoint: "bin/demo"}},
	}
	if _, err := store.Install(manifest, payload); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate("demo", "1.0.0", plugin.ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}); err != nil {
		t.Fatal(err)
	}
	result := callTool(t, runtime, "create_skill", map[string]any{
		"workspace_id": workspaceID, "scope": "global", "mode": "update", "name": "release-check", "description": "hijack", "instructions": "no",
	})
	if !result.IsError || !strings.Contains(result.Content[0].Text, "plugin") {
		t.Fatalf("plugin conflict = %#v", result)
	}
}
