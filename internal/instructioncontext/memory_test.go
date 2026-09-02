package instructioncontext

import (
	"os"
	"path/filepath"
	"testing"
	"time"
	"unicode/utf8"
)

func TestLoadProjectMemoryCandidatesAndPriority(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(root, "CLAUDE.md"):            "root claude",
		filepath.Join(root, ".claude", "CLAUDE.md"): "nested claude",
		filepath.Join(root, "AGENTS.md"):            "agents",
		filepath.Join(root, "CLAUDE.local.md"):      "local claude",
		filepath.Join(home, ".codex", "CLAUDE.md"):  "codex user",
		filepath.Join(home, ".claude", "CLAUDE.md"): "claude user",
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	loadedAt := time.Date(2026, 9, 3, 1, 2, 3, 0, time.FixedZone("test", 7*60*60))
	bundle, err := LoadProjectMemory(root, MemoryLoadOptions{HomeDir: home, WorkspaceRoots: []string{root}, Now: func() time.Time { return loadedAt }})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Sections) != 5 {
		t.Fatalf("sections = %#v", bundle.Sections)
	}
	want := []struct {
		kind    SectionKind
		source  string
		content string
	}{
		{SectionUser, "codex", "codex user"},
		{SectionProject, "claude", "root claude"},
		{SectionProject, "claude", "nested claude"},
		{SectionProject, "agents", "agents"},
		{SectionProject, "claude", "local claude"},
	}
	for i, expected := range want {
		section := bundle.Sections[i]
		if section.Kind != expected.kind || section.Source != expected.source || section.Content != expected.content || section.Truncated {
			t.Fatalf("section %d = %#v", i, section)
		}
	}
	if bundle.LoadedAt != loadedAt.UTC() || bundle.TotalBytes == 0 {
		t.Fatalf("bundle = %#v", bundle)
	}
}

func TestLoadProjectMemorySkipsSymlinkAndEmptyFiles(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("outside"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "CLAUDE.md")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(" \n\t"), 0644); err != nil {
		t.Fatal(err)
	}
	bundle, err := LoadProjectMemory(root, MemoryLoadOptions{DisableUser: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Sections) != 0 || bundle.TotalBytes != 0 {
		t.Fatalf("bundle = %#v", bundle)
	}
}

func TestLoadProjectMemoryLimitsSectionByLinesAndUTF8Bytes(t *testing.T) {
	root := t.TempDir()
	content := "first\nsecond\n界界界"
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	bundle, err := LoadProjectMemory(root, MemoryLoadOptions{DisableUser: true, MaxLinesPerSection: 3, MaxBytesPerSection: 16})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Sections) != 1 {
		t.Fatalf("sections = %#v", bundle.Sections)
	}
	section := bundle.Sections[0]
	if !section.Truncated || section.OriginalBytes != len([]byte(content)) || section.LoadedBytes > 16 || !utf8.ValidString(section.Content) {
		t.Fatalf("section = %#v", section)
	}
}

func TestLoadProjectMemoryDefaultsWorkspaceRootsToRoot(t *testing.T) {
	root := t.TempDir()
	bundle, err := LoadProjectMemory(root, MemoryLoadOptions{DisableUser: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.WorkspaceRoots) != 1 || bundle.WorkspaceRoots[0] != bundle.Root {
		t.Fatalf("workspace roots = %#v root=%q", bundle.WorkspaceRoots, bundle.Root)
	}
}

func TestLoadProjectMemoryFallsBackToClaudeUserInstructions(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	path := filepath.Join(home, ".claude", "CLAUDE.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("claude user"), 0644); err != nil {
		t.Fatal(err)
	}
	bundle, err := LoadProjectMemory(root, MemoryLoadOptions{HomeDir: home})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Sections) != 1 || bundle.Sections[0].Kind != SectionUser || bundle.Sections[0].Source != "claude" || bundle.Sections[0].Content != "claude user" {
		t.Fatalf("sections = %#v", bundle.Sections)
	}
}
