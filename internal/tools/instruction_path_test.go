package tools

import (
	"path/filepath"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/testutil"
)

func TestWithinDirectoryAllowsIsolatedConfigRoot(t *testing.T) {
	configDir := t.TempDir()
	testutil.UseConfigRoot(t, configDir)
	path := filepath.Join(configDir, "skills", "release-check", "SKILL.md")
	if !withinDirectory(configformat.RootPath(), path) {
		t.Fatalf("config root rejected %s", path)
	}
	if withinDirectory(configformat.RootPath(), filepath.Join(t.TempDir(), "skills", "other", "SKILL.md")) {
		t.Fatal("sibling path accepted")
	}
}
