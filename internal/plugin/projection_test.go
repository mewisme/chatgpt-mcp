package plugin

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
)

func TestSyncProjectionsGlobalFixture(t *testing.T) {
	layout := testProjectionLayout(t)
	payload := filepath.Join("testdata", "instruction-resources")
	if err := SyncProjections(layout, "", payload); err != nil {
		t.Fatal(err)
	}
	rule := filepath.Join(layout.RulesRoot(), "typescript.md")
	skill := filepath.Join(layout.SkillsRoot(), "release-check", "SKILL.md")
	support := filepath.Join(layout.SkillsRoot(), "release-check", "references", "checklist.md")
	for _, path := range []string{rule, skill, support} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("projected %s: %v", path, err)
		}
	}
	if err := SyncProjections(layout, payload, payload); err != nil {
		t.Fatal(err)
	}
}

func TestSyncProjectionsFollowsIsolatedConfigDir(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv(configformat.EnvConfigDir, configDir)
	layout := DefaultLayout()
	if err := SyncProjections(layout, "", filepath.Join("testdata", "instruction-resources")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(configDir, "rules", "typescript.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(configDir, "skills", "release-check", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

func TestSyncProjectionsWorkspaceStaysUnderCgm(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv(configformat.EnvConfigDir, configDir)
	workspace := t.TempDir()
	layout, err := WorkspaceLayout(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if err := SyncProjections(layout, "", filepath.Join("testdata", "instruction-resources")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".cgm", "rules", "typescript.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".cgm", "skills", "release-check", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(configDir, "rules", "typescript.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("workspace projection leaked globally: %v", err)
	}
}

func TestSyncProjectionsRejectsUnmanagedDestination(t *testing.T) {
	layout := testProjectionLayout(t)
	dest := filepath.Join(layout.RulesRoot(), "typescript.md")
	if err := os.MkdirAll(filepath.Dir(dest), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte("user rule"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SyncProjections(layout, "", filepath.Join("testdata", "instruction-resources")); !errors.Is(err, ErrProjectionConflict) {
		t.Fatalf("unmanaged error = %v", err)
	}
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != "user rule" {
		t.Fatalf("unmanaged dest mutated: %q %v", data, err)
	}
}

func TestSyncProjectionsRejectsOtherPluginDestination(t *testing.T) {
	layout := testProjectionLayout(t)
	first := filepath.Join("testdata", "instruction-resources")
	if err := SyncProjections(layout, "", first); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	if err := os.MkdirAll(filepath.Join(other, "rules"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "rules", "typescript.md"), []byte("---\ndescription: other\n---\nno\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SyncProjections(layout, "", other); !errors.Is(err, ErrProjectionConflict) {
		t.Fatalf("other plugin error = %v", err)
	}
}

func TestSyncProjectionsUnprojectPreservesDrift(t *testing.T) {
	layout := testProjectionLayout(t)
	payload := filepath.Join("testdata", "instruction-resources")
	if err := SyncProjections(layout, "", payload); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(layout.RulesRoot(), "typescript.md")
	if err := os.WriteFile(dest, []byte("edited"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SyncProjections(layout, payload, ""); !errors.Is(err, ErrProjectionDrift) {
		t.Fatalf("drift error = %v", err)
	}
	data, err := os.ReadFile(dest)
	if err != nil || string(data) != "edited" {
		t.Fatalf("drift dest mutated: %q %v", data, err)
	}
}

func TestSyncProjectionsDisableAndUpdate(t *testing.T) {
	layout := testProjectionLayout(t)
	v1 := filepath.Join("testdata", "instruction-resources")
	if err := SyncProjections(layout, "", v1); err != nil {
		t.Fatal(err)
	}
	if err := SyncProjections(layout, v1, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(layout.RulesRoot(), "typescript.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("disable left rule")
	}
	if _, err := os.Stat(filepath.Join(layout.SkillsRoot(), "release-check")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("disable left skill")
	}
	if err := SyncProjections(layout, "", v1); err != nil {
		t.Fatal(err)
	}
	v2 := t.TempDir()
	if err := os.MkdirAll(filepath.Join(v2, "rules"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(v2, "rules", "go.md"), []byte("---\ndescription: Go\n---\nGo.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SyncProjections(layout, v1, v2); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(layout.RulesRoot(), "typescript.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("update left old rule")
	}
	if _, err := os.Stat(filepath.Join(layout.RulesRoot(), "go.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(layout.SkillsRoot(), "release-check")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("update left old skill")
	}
}

func testProjectionLayout(t *testing.T) Layout {
	t.Helper()
	root := t.TempDir()
	layout := Layout{ConfigRoot: filepath.Join(root, "config"), DataRoot: filepath.Join(root, "data"), CacheRoot: filepath.Join(root, "cache")}
	if err := layout.Validate(); err != nil {
		t.Fatal(err)
	}
	return layout
}

func TestProjectionStatusReportsHealth(t *testing.T) {
	layout := testProjectionLayout(t)
	payload := filepath.Join("testdata", "instruction-resources")
	status, err := ProjectionStatus(layout, payload, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(status) != 2 || status[0].State != ResourceMissing || status[1].State != ResourceMissing {
		t.Fatalf("missing = %#v", status)
	}
	if err := SyncProjections(layout, "", payload); err != nil {
		t.Fatal(err)
	}
	status, err = ProjectionStatus(layout, payload, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(status) != 2 || status[0].State != ResourceActive || status[1].State != ResourceActive {
		t.Fatalf("active = %#v", status)
	}
	if err := os.WriteFile(filepath.Join(layout.RulesRoot(), "typescript.md"), []byte("edited"), 0600); err != nil {
		t.Fatal(err)
	}
	status, err = ProjectionStatus(layout, payload, true)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range status {
		if item.Kind == "rule" && item.State == ResourceModified {
			found = true
		}
	}
	if !found {
		t.Fatalf("modified = %#v", status)
	}
}
