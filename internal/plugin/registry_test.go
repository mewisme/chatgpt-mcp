package plugin

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRegistryResolveStableAndVersion(t *testing.T) {
	snapshot := testRegistrySnapshot("official", true)
	resolved, err := snapshot.Resolve("bash", "")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Version != "1.0.0" || resolved.ManifestName != "bash-1.0.0.json" {
		t.Fatalf("resolved = %#v", resolved)
	}
	if _, err := snapshot.Resolve("bash", "9.9.9"); err == nil {
		t.Fatal("missing version resolved")
	}
}

func TestRegistryRejectsUnknownPublisher(t *testing.T) {
	snapshot := testRegistrySnapshot("official", true)
	entry := snapshot.Index.Plugins["bash"]
	entry.Publisher = "unknown"
	snapshot.Index.Plugins["bash"] = entry
	if err := snapshot.Validate(); err == nil {
		t.Fatal("unknown publisher accepted")
	}
}

func TestResolveAcrossRejectsAmbiguity(t *testing.T) {
	official := testRegistrySnapshot("official", true)
	thirdParty := testRegistrySnapshot("community", true)
	if _, err := ResolveAcross([]RegistrySnapshot{official, thirdParty}, "", "bash", ""); err == nil {
		t.Fatal("ambiguous unqualified plugin resolved")
	}
	resolved, err := ResolveAcross([]RegistrySnapshot{official, thirdParty}, "community", "bash", "")
	if err != nil || resolved.Registry.Name != "community" {
		t.Fatalf("qualified resolve = %#v, %v", resolved, err)
	}
}

func TestRegistryRejectsInvalidScopes(t *testing.T) {
	snapshot := testRegistrySnapshot("official", true)
	entry := snapshot.Index.Plugins["bash"]
	entry.Scopes = []PluginScope{"cluster"}
	snapshot.Index.Plugins["bash"] = entry
	if err := snapshot.Index.Validate(); err == nil {
		t.Fatal("invalid registry scopes accepted")
	}
	entry.Scopes = []PluginScope{ScopeGlobal, ScopeGlobal}
	snapshot.Index.Plugins["bash"] = entry
	if err := snapshot.Index.Validate(); err == nil {
		t.Fatal("duplicate registry scopes accepted")
	}
}

func TestRegistryEntryWithoutScopesDefaultsToGlobal(t *testing.T) {
	snapshot := testRegistrySnapshot("official", true)
	if err := snapshot.Index.Validate(); err != nil {
		t.Fatal(err)
	}
	got := snapshot.Index.Plugins["bash"].AllowedScopes()
	if len(got) != 1 || got[0] != ScopeGlobal {
		t.Fatalf("implicit scopes = %#v", got)
	}
}

func TestOfficialRegistryIndexDeclaresScopes(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "plugins", "registry", "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	index, err := ParseRegistryIndex(data)
	if err != nil {
		t.Fatal(err)
	}
	want := map[PluginID][]PluginScope{
		"admin-ui": {ScopeGlobal},
		"bash":     {ScopeGlobal, ScopeWorkspace},
		"rtk":      {ScopeGlobal, ScopeWorkspace},
	}
	for id, scopes := range want {
		got := index.Plugins[id].AllowedScopes()
		if len(got) != len(scopes) {
			t.Fatalf("%s scopes = %#v, want %#v", id, got, scopes)
		}
		for i := range scopes {
			if got[i] != scopes[i] {
				t.Fatalf("%s scopes = %#v, want %#v", id, got, scopes)
			}
		}
	}
}

func testRegistrySnapshot(name string, unqualified bool) RegistrySnapshot {
	publisher := Publisher{Name: "mewisme", Source: "https://github.com/mewisme/chatgpt-mcp", Trusted: true, Sigstore: SigstoreIdentity{Issuer: "https://token.actions.githubusercontent.com", Repository: "mewisme/chatgpt-mcp"}}
	trust := SigstoreIdentity{Issuer: OfficialSigstoreIssuer, Repository: OfficialSigstoreRepo}
	return RegistrySnapshot{
		Registry: Registry{Name: name, URL: "https://example.invalid/plugins", UnqualifiedResolution: unqualified, Trust: &trust},
		Index: RegistryIndex{Schema: RegistrySchema, GeneratedAt: time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC), Plugins: map[PluginID]RegistryEntry{
			"bash": {Publisher: "mewisme", Name: "Bash Runtime", Description: "Portable Bash", Type: "runtime", Stable: "1.0.0", Versions: map[Version]string{"1.0.0": "bash-1.0.0.json"}},
		}},
		Publishers: PublisherIndex{Schema: PublishersSchema, Publishers: map[string]Publisher{"mewisme": publisher}},
	}
}
