package hooks

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverTrustedPonytailHooks(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	pluginRoot := filepath.Join(home, "plugins", "cache", "ponytail", "ponytail", "1.0.0")
	hooksDir := filepath.Join(pluginRoot, "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"echo start","timeout":2}]}],"UserPromptSubmit":[{"hooks":[{"type":"command","command":"echo prompt"}]}]}}`
	if err := os.WriteFile(filepath.Join(hooksDir, "codex-hooks.json"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	activationID := "ponytail@ponytail:hooks/codex-hooks.json:session_start:0:0"
	config := `[plugins."ponytail@ponytail"]
enabled = true

[hooks.state."` + activationID + `"]
`
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0644); err != nil {
		t.Fatal(err)
	}
	values, err := Discover()
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 {
		t.Fatalf("hooks = %#v", values)
	}
	if !values[0].Trusted && !values[1].Trusted {
		t.Fatalf("trusted activation missing: %#v", values)
	}
}

func TestHookContext(t *testing.T) {
	value := hookContext(`{"hookSpecificOutput":{"additionalContext":"hello"}}`)
	if value != "hello" {
		t.Fatalf("value = %q", value)
	}
}
