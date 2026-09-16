package pluginbuild_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeOfficialBuildersUseSharedHelper(t *testing.T) {
	root := moduleRoot(t)
	native := []string{"cf-tunnel", "secure-mcp-tunnel", "tui", "markdown-formatter", "ponytail", "caveman"}
	for _, id := range native {
		data, err := os.ReadFile(filepath.Join(root, "plugins", id, "build", "main.go"))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		if !strings.Contains(text, "pluginbuild.Build") || !strings.Contains(text, `"./plugins/`+id+`/cmd/`) {
			t.Fatalf("%s builder does not use pluginbuild.Build", id)
		}
	}
	for _, id := range []string{"admin-ui", "bash"} {
		data, err := os.ReadFile(filepath.Join(root, "plugins", id, "build", "main.go"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "pluginbuild") {
			t.Fatalf("%s builder must not use native pluginbuild compression", id)
		}
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}
