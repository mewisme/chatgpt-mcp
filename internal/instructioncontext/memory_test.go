package instructioncontext

import (
	"os"
	"path/filepath"
	"testing"
	"time"
	"unicode/utf8"
)

func writeInstructionFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadProjectMemoryPrefersAgentsAndLoadsDistinctFallbacks(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	files := []struct{ path, content string }{
		{filepath.Join(home, ".agents", "AGENTS.md"), "user agents"},
		{filepath.Join(home, ".claude", "CLAUDE.md"), "user claude"},
		{filepath.Join(home, ".codex", "AGENTS.md"), "user agents"},
		{filepath.Join(root, "AGENTS.md"), "root agents"},
		{filepath.Join(root, ".agents", "AGENTS.md"), "project agents"},
		{filepath.Join(root, "CLAUDE.md"), "root claude"},
		{filepath.Join(root, ".claude", "CLAUDE.md"), "root agents"},
		{filepath.Join(root, ".claudes", "CLAUDE.md"), "claudes fallback"},
		{filepath.Join(root, ".cursor", "AGENTS.md"), "cursor fallback"},
		{filepath.Join(root, ".codex", "AGENTS.md"), "codex fallback"},
		{filepath.Join(root, "CLAUDE.local.md"), "local claude"},
	}
	for _, file := range files {
		writeInstructionFile(t, file.path, file.content)
	}
	loadedAt := time.Date(2026, 9, 3, 1, 2, 3, 0, time.FixedZone("test", 7*60*60))
	bundle, err := LoadProjectMemory(root, MemoryLoadOptions{HomeDir: home, WorkspaceRoots: []string{root}, Now: func() time.Time { return loadedAt }})
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		kind, source, content string
	}{
		{"user", "agents", "user agents"},
		{"user", "claude", "user claude"},
		{"project", "agents", "root agents"},
		{"project", "agents", "project agents"},
		{"project", "claude", "root claude"},
		{"project", "claudes", "claudes fallback"},
		{"project", "cursor", "cursor fallback"},
		{"project", "codex", "codex fallback"},
		{"project", "claude", "local claude"},
	}
	if len(bundle.Sections) != len(want) {
		t.Fatalf("sections = %#v", bundle.Sections)
	}
	for i, expected := range want {
		section := bundle.Sections[i]
		if string(section.Kind) != expected.kind || section.Source != expected.source || section.Content != expected.content || section.Truncated {
			t.Fatalf("section %d = %#v", i, section)
		}
	}
	if bundle.LoadedAt != loadedAt.UTC() || bundle.TotalBytes == 0 {
		t.Fatalf("bundle = %#v", bundle)
	}
}

func TestLoadProjectMemoryDeduplicatesNormalizedContent(t *testing.T) {
	root := t.TempDir()
	writeInstructionFile(t, filepath.Join(root, "AGENTS.md"), "same\r\ncontent\n")
	writeInstructionFile(t, filepath.Join(root, ".agents", "AGENTS.md"), " same\ncontent ")
	writeInstructionFile(t, filepath.Join(root, ".claude", "CLAUDE.md"), "different")
	bundle, err := LoadProjectMemory(root, MemoryLoadOptions{DisableUser: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Sections) != 2 || bundle.Sections[0].Source != "agents" || bundle.Sections[0].Content != "same\ncontent" || bundle.Sections[1].Content != "different" {
		t.Fatalf("sections = %#v", bundle.Sections)
	}
}

func TestLoadProjectMemorySkipsSymlinkAndEmptyFiles(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.md")
	writeInstructionFile(t, outside, "outside")
	if err := os.Symlink(outside, filepath.Join(root, "AGENTS.md")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	writeInstructionFile(t, filepath.Join(root, ".agents", "AGENTS.md"), " \n\t")
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
	writeInstructionFile(t, filepath.Join(root, "AGENTS.md"), content)
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
