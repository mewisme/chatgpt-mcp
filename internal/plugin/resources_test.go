package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestDiscoverPayloadResourcesFixture(t *testing.T) {
	got, err := DiscoverPayloadResources(filepath.Join("testdata", "instruction-resources"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rules) != 1 || got.Rules[0].Name != "typescript" || got.Rules[0].Path != "rules/typescript.md" {
		t.Fatalf("rules = %#v", got.Rules)
	}
	if len(got.Skills) != 1 || got.Skills[0].Name != "release-check" || got.Skills[0].Path != "skills/release-check/SKILL.md" {
		t.Fatalf("skills = %#v", got.Skills)
	}
	if got.Skills[0].Description != "Validate a release before publishing." {
		t.Fatalf("description = %q", got.Skills[0].Description)
	}
	if len(got.Skills[0].Files) != 1 || got.Skills[0].Files[0] != "references/checklist.md" {
		t.Fatalf("files = %#v", got.Skills[0].Files)
	}
}

func TestDiscoverPayloadResourcesIgnoresUnrelatedAndMissing(t *testing.T) {
	root := t.TempDir()
	got, err := DiscoverPayloadResources(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rules) != 0 || len(got.Skills) != 0 {
		t.Fatalf("empty payload = %#v", got)
	}
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("hi"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "rules", "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "rules", "nested", "hidden.md"), []byte("no"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "rules", "notes.txt"), []byte("txt"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err = DiscoverPayloadResources(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rules) != 0 || len(got.Skills) != 0 {
		t.Fatalf("unrelated payload = %#v", got)
	}
}

func TestDiscoverPayloadResourcesRejectsMalformedNames(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "rules"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "rules", "Bad Name.md"), []byte("no"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := DiscoverPayloadResources(root); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("malformed rule error = %v", err)
	}
	root = t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "skills", "Bad Skill"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skills", "Bad Skill", "SKILL.md"), []byte("---\nname: bad\ndescription: x\n---\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := DiscoverPayloadResources(root); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("malformed skill error = %v", err)
	}
}

func TestDiscoverPayloadResourcesRejectsMismatchedFrontmatter(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "skills", "ship-it"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skills", "ship-it", "SKILL.md"), []byte("---\nname: other\ndescription: x\n---\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := DiscoverPayloadResources(root); err == nil || !strings.Contains(err.Error(), "must match directory") {
		t.Fatalf("mismatched name error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "skills", "ship-it", "SKILL.md"), []byte("---\nname: ship-it\n---\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := DiscoverPayloadResources(root); err == nil || !strings.Contains(err.Error(), "missing a description") {
		t.Fatalf("missing description error = %v", err)
	}
}

func TestDiscoverPayloadResourcesRejectsSymlinks(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "rules"), 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "target.md")
	if err := os.WriteFile(target, []byte("no"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "rules", "escape.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := DiscoverPayloadResources(root); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("rule symlink error = %v", err)
	}
	root = t.TempDir()
	skillDir := filepath.Join(root, "skills", "ship-it")
	if err := os.MkdirAll(skillDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: ship-it\ndescription: x\n---\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(skillDir, "notes.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := DiscoverPayloadResources(root); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("skill symlink error = %v", err)
	}
}

func TestDiscoverPayloadResourcesRejectsMissingSkillEntrypoint(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "skills", "ship-it"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := DiscoverPayloadResources(root); err == nil || !strings.Contains(err.Error(), "missing SKILL.md") {
		t.Fatalf("missing skill error = %v", err)
	}
}

func TestDiscoverPayloadResourcesRejectsSpecialFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "rules"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(root, "rules", "pipe.md"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := DiscoverPayloadResources(root); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("special file error = %v", err)
	}
}
