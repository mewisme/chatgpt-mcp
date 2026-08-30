package cli

import (
	"path/filepath"
	"testing"
)

func testCommandArgs(t *testing.T, args ...string) []string {
	t.Helper()
	configDir := filepath.Join(t.TempDir(), "config")
	return append([]string{"--config-dir", configDir}, args...)
}
