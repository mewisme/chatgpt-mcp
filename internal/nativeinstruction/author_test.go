package nativeinstruction

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/plugin"
	"go.mewis.me/chatgpt-mcp/internal/testutil"
)

func TestWriteSkillCreateUpdateAndDryRun(t *testing.T) {
	configDir := t.TempDir()
	testutil.UseConfigRoot(t, configDir)
	workspace := t.TempDir()
	created, err := WriteSkill(SkillRequest{
		Scope: ScopeWorkspace, Mode: ModeCreate, Name: "ship-it", Description: "Ship the release", Instructions: "Tag after checks.",
		WorkspaceRoot: workspace, Files: []File{{Path: "references/note.md", Data: []byte("keep")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	skillMD := filepath.Join(workspace, ".cgm", "skills", "ship-it", "SKILL.md")
	if created.Path != skillMD || created.DryRun || !strings.HasPrefix(created.Directory, workspace) {
		t.Fatalf("created = %#v", created)
	}
	body, err := os.ReadFile(skillMD)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(body), "---\nname: ship-it\ndescription: Ship the release\n---\n") {
		t.Fatalf("skill md = %q", body)
	}
	if _, err := WriteSkill(SkillRequest{Scope: ScopeWorkspace, Mode: ModeCreate, Name: "ship-it", Description: "dup", Instructions: "no", WorkspaceRoot: workspace}); err == nil {
		t.Fatal("create over existing succeeded")
	}
	dry, err := WriteSkill(SkillRequest{
		Scope: ScopeWorkspace, Mode: ModeUpdate, Name: "ship-it", Description: "Ship the release", Instructions: "changed",
		WorkspaceRoot: workspace, DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !dry.DryRun {
		t.Fatal("dry_run not set")
	}
	if current, _ := os.ReadFile(skillMD); string(current) != string(body) {
		t.Fatal("dry_run mutated skill")
	}
	if _, err := WriteSkill(SkillRequest{Scope: ScopeWorkspace, Mode: ModeUpdate, Name: "missing", Description: "x", Instructions: "y", WorkspaceRoot: workspace}); err == nil {
		t.Fatal("update missing succeeded")
	}
	binary, _ := base64.StdEncoding.DecodeString("aGVsbG8=")
	updated, err := WriteSkill(SkillRequest{
		Scope: ScopeWorkspace, Mode: ModeUpdate, Name: "ship-it", Description: "Ship the release", Instructions: "Tag after checks.",
		WorkspaceRoot: workspace,
		Files:         []File{{Path: "assets/icon.bin", Data: binary}, {Path: "scripts/run.sh", Data: []byte("#!/bin/sh\n"), Executable: true}},
		Remove:        []string{"references/note.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(created.Directory, "references", "note.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("unrequested file preserved incorrectly")
	}
	if _, err := os.Stat(filepath.Join(created.Directory, "assets", "icon.bin")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(created.Directory, "scripts", "run.sh"))
	if err != nil || info.Mode().Perm()&0111 == 0 {
		t.Fatalf("executable bit missing: %v", info)
	}
	if len(updated.FilesRemoved) != 1 {
		t.Fatalf("removed = %#v", updated.FilesRemoved)
	}
}

func TestWriteSkillRejectsTraversalAndGlobalUsesConfigRoot(t *testing.T) {
	configDir := t.TempDir()
	testutil.UseConfigRoot(t, configDir)
	if _, err := WriteSkill(SkillRequest{Scope: ScopeGlobal, Mode: ModeCreate, Name: "ok", Description: "d", Instructions: "i", Files: []File{{Path: "../escape.md", Data: []byte("x")}}}); err == nil {
		t.Fatal("path escape accepted")
	}
	if _, err := WriteSkill(SkillRequest{Scope: ScopeGlobal, Mode: ModeCreate, Name: "NotValid", Description: "d", Instructions: "i"}); err == nil {
		t.Fatal("malformed name accepted")
	}
	created, err := WriteSkill(SkillRequest{Scope: ScopeGlobal, Mode: ModeCreate, Name: "global-note", Description: "Global helper", Instructions: "Help."})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.Directory, configDir) {
		t.Fatalf("global dest = %s want under %s", created.Directory, configDir)
	}
}

func TestWriteRuleGlobsAndAlwaysApply(t *testing.T) {
	configDir := t.TempDir()
	testutil.UseConfigRoot(t, configDir)
	workspace := t.TempDir()
	if _, err := WriteRule(RuleRequest{Scope: ScopeWorkspace, Name: "ts", Description: "TS", AlwaysApply: true, Globs: []string{"**/*.ts"}, Content: "Use types.", WorkspaceRoot: workspace}); err == nil {
		t.Fatal("always_apply with globs accepted")
	}
	if _, err := WriteRule(RuleRequest{Scope: ScopeWorkspace, Name: "ts", Description: "TS", AlwaysApply: false, Content: "Use types.", WorkspaceRoot: workspace}); err == nil {
		t.Fatal("file-specific without globs accepted")
	}
	created, err := WriteRule(RuleRequest{Scope: ScopeWorkspace, Name: "ts", Description: "TS conventions", AlwaysApply: false, Globs: []string{"**/*.ts", "**/*.ts", "**/*.tsx"}, Content: "Prefer explicit types.", WorkspaceRoot: workspace})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(workspace, ".cgm", "rules", "ts.md")
	if created.Path != path {
		t.Fatalf("path = %s", created.Path)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if strings.Count(text, `"**/*.ts"`) != 1 || !strings.Contains(text, "alwaysApply: false") || strings.HasSuffix(path, ".mdc") {
		t.Fatalf("rule = %q", text)
	}
	always, err := WriteRule(RuleRequest{Scope: ScopeGlobal, Name: "house", Description: "House style", AlwaysApply: true, Content: "Be concise."})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(always.Path, configDir) {
		t.Fatalf("global rule = %s", always.Path)
	}
}

func TestWriteSkillRejectsPluginProjection(t *testing.T) {
	configDir := t.TempDir()
	testutil.UseConfigRoot(t, configDir)
	store, err := plugin.NewStore(plugin.DefaultLayout(), plugin.RuntimeContext{OS: runtime.GOOS, Arch: runtime.GOARCH, CoreVersion: "0.2.24"})
	if err != nil {
		t.Fatal(err)
	}
	payload := filepath.Join("..", "plugin", "testdata", "instruction-resources")
	manifest := plugin.Manifest{
		Schema: plugin.ManifestSchema, ID: "demo", Name: "demo", Publisher: "mewisme", License: "Apache-2.0", Version: "1.0.0", Type: "runtime",
		Provides:    []plugin.Capability{"formatter/demo"},
		Permissions: []plugin.Permission{plugin.PermissionProcessExecute},
		Platforms: map[string]plugin.PlatformArtifact{
			runtime.GOOS + "/" + runtime.GOARCH: {Artifact: "demo-1.0.0.tar.gz", SHA256: strings.Repeat("a", 64), Archive: "tar.gz", Entrypoint: "bin/demo"},
		},
	}
	if _, err := store.Install(manifest, payload); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate("demo", "1.0.0", plugin.ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}); err != nil {
		t.Fatal(err)
	}
	_, err = WriteSkill(SkillRequest{Scope: ScopeGlobal, Mode: ModeUpdate, Name: "release-check", Description: "hijack", Instructions: "no"})
	if err == nil || !errors.Is(err, plugin.ErrProjectionConflict) {
		t.Fatalf("plugin overwrite error = %v", err)
	}
	_, err = WriteRule(RuleRequest{Scope: ScopeGlobal, Mode: ModeUpdate, Name: "typescript", Description: "hijack", AlwaysApply: false, Globs: []string{"**/*.ts"}, Content: "no"})
	if err == nil || !errors.Is(err, plugin.ErrProjectionConflict) {
		t.Fatalf("plugin rule overwrite error = %v", err)
	}
}
