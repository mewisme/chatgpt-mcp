//go:build !windows

package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStableBinaryPathPreservesLauncherSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "versions", "v1", "chatgpt-mcp")
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("binary"), 0700); err != nil {
		t.Fatal(err)
	}
	launcher := filepath.Join(dir, "bin", "cmcp")
	if err := os.MkdirAll(filepath.Dir(launcher), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, launcher); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(launcher))
	resolved, err := StableBinaryPath("cmcp")
	if err != nil {
		t.Fatal(err)
	}
	if resolved != launcher {
		t.Fatalf("stable binary path = %q, want launcher %q", resolved, launcher)
	}
}