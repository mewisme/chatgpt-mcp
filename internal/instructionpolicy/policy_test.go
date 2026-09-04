package instructionpolicy

import (
	"path/filepath"
	"testing"
)

func boolPtr(value bool) *bool { return &value }

func TestPolicyDefaultsEnabledAndSupportsProviderMasterToggle(t *testing.T) {
	value := DefaultConfig()
	if !value.Enabled("claude", ResourceContext) || !value.Enabled(".claude", ResourceRules) || !value.Enabled("claude", ResourceSkills) {
		t.Fatal("missing source policy must default to enabled")
	}
	value.Sources["claude"] = SourcePolicy{Enabled: boolPtr(true), Context: boolPtr(false), Rules: boolPtr(true), Skills: boolPtr(false)}
	if value.Enabled("claude", ResourceContext) || !value.Enabled("claude", ResourceRules) || value.Enabled("claude", ResourceSkills) {
		t.Fatalf("resource policy not applied: %#v", value.Sources["claude"])
	}
	value.Sources["claude"] = SourcePolicy{Enabled: boolPtr(false), Context: boolPtr(true), Rules: boolPtr(true), Skills: boolPtr(true)}
	if value.Enabled("claude", ResourceContext) || value.Enabled("claude", ResourceRules) || value.Enabled("claude", ResourceSkills) {
		t.Fatal("provider master toggle did not disable every resource")
	}
}

func TestStoreRoundTripUsesJSONAndPreservesColonContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "instructions", "global.json")
	store := &Store{Path: path}
	value := DefaultConfig()
	value.Context = "context: keep this exact text"
	value.Rules = []GlobalRule{{ID: "rule_test", Name: "Test", Enabled: true, Content: "rule: value"}}
	value.Sources["claude"] = SourcePolicy{Context: boolPtr(false)}
	if err := store.Save(value); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Context != value.Context || len(loaded.Rules) != 1 || loaded.Rules[0].Content != "rule: value" || loaded.Enabled("claude", ResourceContext) {
		t.Fatalf("loaded = %#v", loaded)
	}
}
