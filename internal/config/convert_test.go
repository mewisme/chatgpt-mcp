package config

import (
	"os"
	"path/filepath"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
)

func TestConvertFormatAtConvertsStructuredTree(t *testing.T) {
	root := t.TempDir()
	write := func(relative, content string) {
		t.Helper()
		path := filepath.Join(root, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("config.json", `{"server":{"port":37421},"admin":{"enabled":true}}`)
	write("tunnel.json", `{"api_key":"secret"}`)
	write("upstream.json", `[{"id":"alpha","name":"Alpha","transport":"http","enabled":true}]`)
	write("workspaces/ws_test/shell.json", `{"workspace_id":"ws_test","cwd":"/tmp"}`)
	write("workspaces/ws_test/checkpoints/index.json", `{"version":1,"checkpoints":[]}`)
	write("workspaces/ws_test/checkpoints/data/cp_test/manifest.json", `{"version":1,"id":"cp_test"}`)
	write("workspaces/ws_test/activity.jsonl", "{}\n")

	converted, err := convertFormatAt(root, configformat.TOML)
	if err != nil {
		t.Fatal(err)
	}
	if converted != 6 {
		t.Fatalf("converted = %d, want 6", converted)
	}
	for _, relative := range []string{
		"config.toml", "tunnel.toml", "upstream.toml", "workspaces/ws_test/shell.toml",
		"workspaces/ws_test/checkpoints/index.toml", "workspaces/ws_test/checkpoints/data/cp_test/manifest.toml",
	} {
		if _, err := os.Stat(filepath.Join(root, relative)); err != nil {
			t.Fatalf("missing converted %s: %v", relative, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "config.json")); !os.IsNotExist(err) {
		t.Fatalf("old config survived: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "workspaces/ws_test/activity.jsonl")); err != nil {
		t.Fatalf("jsonl log should not be converted: %v", err)
	}
	upstreamData, err := os.ReadFile(filepath.Join(root, "upstream.toml"))
	if err != nil {
		t.Fatal(err)
	}
	var upstream struct {
		Servers []map[string]any `json:"servers"`
	}
	if err := configformat.Unmarshal(configformat.TOML, upstreamData, &upstream); err != nil {
		t.Fatal(err)
	}
	if len(upstream.Servers) != 1 || upstream.Servers[0]["id"] != "alpha" {
		t.Fatalf("upstream = %#v", upstream.Servers)
	}
}
