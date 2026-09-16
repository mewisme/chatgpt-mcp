package plugin

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/secretstore"
)

func testPluginSettingsSchema() SettingsSchema {
	return SettingsSchema{Fields: []SettingField{
		{Key: "default_active", Kind: FieldBool, Default: true},
		{Key: "default_mode", Kind: FieldEnum, Enum: []string{"full", "lite"}, Default: "full"},
		{Key: "api_token", Kind: FieldString, Sensitive: true, Default: ""},
	}}
}

func TestConfigDefaultsDoNotWriteFiles(t *testing.T) {
	store := SettingsStore{Layout: testStore(t).layout}
	values, err := store.Get(testPluginSettingsSchema(), "ponytail")
	if err != nil {
		t.Fatal(err)
	}
	if values["default_active"] != true || values["default_mode"] != "full" {
		t.Fatalf("defaults = %#v", values)
	}
	if _, err := os.Stat(store.Layout.PluginConfigPath("ponytail")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("defaults wrote file: %v", err)
	}
}

func TestConfigSetGetResetAndUnknownKey(t *testing.T) {
	store := SettingsStore{Layout: testStore(t).layout}
	schema := testPluginSettingsSchema()
	if err := store.Set(schema, "ponytail", "default_mode", "lite"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(schema, "ponytail", "unknown", true); !errors.Is(err, ErrUnknownConfigKey) {
		t.Fatalf("unknown key error = %v", err)
	}
	if err := store.Set(schema, "ponytail", "default_mode", "ultra"); !errors.Is(err, ErrInvalidConfigValue) {
		t.Fatalf("invalid enum error = %v", err)
	}
	values, err := store.Get(schema, "ponytail")
	if err != nil {
		t.Fatal(err)
	}
	if values["default_mode"] != "lite" {
		t.Fatalf("values = %#v", values)
	}
	if err := store.Reset(schema, "ponytail", "default_mode"); err != nil {
		t.Fatal(err)
	}
	values, err = store.Get(schema, "ponytail")
	if err != nil {
		t.Fatal(err)
	}
	if values["default_mode"] != "full" {
		t.Fatalf("reset values = %#v", values)
	}
	if _, err := os.Stat(store.Layout.PluginConfigPath("ponytail")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("reset left empty config file")
	}
}

func TestConfigSensitiveValueStaysOutOfJSON(t *testing.T) {
	root := testStore(t)
	store := SettingsStore{Layout: root.layout, Secrets: secretstore.New(root.layout.ConfigRoot)}
	schema := testPluginSettingsSchema()
	if err := store.Set(schema, "ponytail", "api_token", "sk-secret"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(store.Layout.PluginConfigPath("ponytail"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "sk-secret") {
		t.Fatalf("plaintext secret leaked: %s", data)
	}
	public, err := store.Public(schema, "ponytail")
	if err != nil {
		t.Fatal(err)
	}
	if public["api_token"] != true || public["default_active"] != true {
		t.Fatalf("public = %#v", public)
	}
	values, err := store.Get(schema, "ponytail")
	if err != nil {
		t.Fatal(err)
	}
	if values["api_token"] != "sk-secret" {
		t.Fatalf("resolved = %#v", values)
	}
}

func TestConfigApplyFailureRollsBack(t *testing.T) {
	store := SettingsStore{Layout: testStore(t).layout, Apply: func(PluginID, map[string]any) error {
		return errors.New("reload failed")
	}}
	schema := testPluginSettingsSchema()
	if err := store.Set(schema, "ponytail", "default_mode", "lite"); err == nil {
		t.Fatal("failed apply accepted")
	}
	values, err := store.Get(schema, "ponytail")
	if err != nil {
		t.Fatal(err)
	}
	if values["default_mode"] != "full" {
		t.Fatalf("rolled back values = %#v", values)
	}
	if _, err := os.Stat(store.Layout.PluginConfigPath("ponytail")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("failed apply left config file")
	}
}

func TestWorkspacePluginConfigPath(t *testing.T) {
	path := filepath.ToSlash(WorkspacePluginConfigPath("/tmp/work", "ponytail"))
	if !strings.HasSuffix(path, "/.cgm/plugins/config/ponytail.json") {
		t.Fatalf("workspace path = %s", path)
	}
}
