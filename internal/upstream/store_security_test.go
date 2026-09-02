package upstream

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreKeepsSensitiveHeaderAndEnvInKeyring(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upstream.json")
	store := NewStore(path)
	server := Server{ID: "alpha", Name: "Alpha", Transport: "http", URL: "https://example.test/mcp", Headers: map[string]string{"Authorization": "Bearer header-private-value", "X-Test": "ok"}, Env: map[string]string{"API_TOKEN": "env-private-value", "MODE": "test"}}
	if err := store.Save([]Server{server}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "header-private-value") || strings.Contains(text, "env-private-value") || !strings.Contains(text, "os-keyring") || !strings.Contains(text, `"X-Test": "ok"`) || !strings.Contains(text, `"MODE": "test"`) {
		t.Fatalf("upstream file persistence = %s", data)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].Headers["Authorization"] != "Bearer header-private-value" || loaded[0].Env["API_TOKEN"] != "env-private-value" {
		t.Fatalf("loaded=%#v", loaded)
	}
}

func TestLegacyUpstreamSecretsMigrateToKeyring(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upstream.json")
	legacy := diskStore{Servers: []Server{{ID: "alpha", Name: "Alpha", Transport: "http", URL: "https://example.test/mcp", Headers: map[string]string{"Authorization": "Bearer legacy-header-value"}, Env: map[string]string{"API_TOKEN": "legacy-env-value"}}}}
	data, err := json.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	store := NewStore(path)
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded[0].Headers["Authorization"] != "Bearer legacy-header-value" || loaded[0].Env["API_TOKEN"] != "legacy-env-value" {
		t.Fatalf("loaded=%#v", loaded)
	}
	migrated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(migrated), "legacy-header-value") || strings.Contains(string(migrated), "legacy-env-value") || strings.Count(string(migrated), "os-keyring") < 2 {
		t.Fatalf("upstream file was not migrated: %s", migrated)
	}
}
