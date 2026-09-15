package plugin

import (
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

func testRegistrySnapshot(name string, unqualified bool) RegistrySnapshot {
	publisher := Publisher{Name: "mewisme", Source: "https://github.com/mewisme/chatgpt-mcp", Trusted: true, Sigstore: SigstoreIdentity{Issuer: "https://token.actions.githubusercontent.com", Repository: "mewisme/chatgpt-mcp"}}
	return RegistrySnapshot{
		Registry: Registry{Name: name, URL: "https://example.invalid/plugins", UnqualifiedResolution: unqualified},
		Index: RegistryIndex{Schema: RegistrySchema, GeneratedAt: time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC), Plugins: map[PluginID]RegistryEntry{
			"bash": {Publisher: "mewisme", Name: "Bash Runtime", Description: "Portable Bash", Type: "runtime", Stable: "1.0.0", Versions: map[Version]string{"1.0.0": "bash-1.0.0.json"}},
		}},
		Publishers: PublisherIndex{Schema: PublishersSchema, Publishers: map[string]Publisher{"mewisme": publisher}},
	}
}
