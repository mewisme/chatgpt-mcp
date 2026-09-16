package plugin

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAssessCoreCompatibilityReportsEnabledIncompatiblePluginsOnly(t *testing.T) {
	store := testStore(t)
	trust := ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}
	compatible := testManifest("compatible", "1.0.0", "formatter/compatible")
	compatible.Requires.ChatGPTMCP = ">=0.2.0"
	incompatible := testManifest("incompatible", "1.0.0", "formatter/incompatible")
	incompatible.Requires.ChatGPTMCP = "<=0.5.0"
	disabled := testManifest("disabled", "1.0.0", "formatter/disabled")
	disabled.Requires.ChatGPTMCP = "<=0.5.0"
	for _, manifest := range []Manifest{compatible, incompatible, disabled} {
		if _, err := store.Install(manifest, testPayload(t, string(manifest.ID))); err != nil {
			t.Fatal(err)
		}
		if err := store.Activate(manifest.ID, manifest.Version, trust); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.SetEnabled("disabled", false); err != nil {
		t.Fatal(err)
	}
	issues, err := (&Manager{Store: store}).AssessCoreCompatibility("v1.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 1 || issues[0].ID != "incompatible" || issues[0].Requirement != "<=0.5.0" {
		t.Fatalf("compatibility issues = %#v", issues)
	}
}

func TestCapabilityConflictsReportsDeterministicProviders(t *testing.T) {
	store := testStore(t)
	trust := ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}
	for _, id := range []string{"provider-b", "provider-a"} {
		manifest := testManifest(id, "1.0.0", "formatter/shared")
		if _, err := store.Install(manifest, testPayload(t, id)); err != nil {
			t.Fatal(err)
		}
		if err := store.Activate(PluginID(id), "1.0.0", trust); err != nil {
			t.Fatal(err)
		}
	}
	conflicts, err := CapabilityConflicts(store)
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 1 || conflicts[0].Capability != "formatter/shared" || len(conflicts[0].Providers) != 2 || conflicts[0].Providers[0].PluginID != "provider-a" || conflicts[0].Providers[1].PluginID != "provider-b" {
		t.Fatalf("conflicts = %#v", conflicts)
	}
}

func TestRegistryHealthFallsBackToVerifiedCache(t *testing.T) {
	now := time.Date(2026, 9, 15, 1, 0, 0, 0, time.UTC)
	server, _ := testRegistryServer(t, now)
	layout := testLayout(t)
	client := RegistryClient{HTTPClient: server.Client(), Layout: layout, Now: func() time.Time { return now }, Verifier: testRegistryVerifier}
	config := NewConfig()
	config.Registries = map[string]Registry{"test": {Name: "test", URL: server.URL, UnqualifiedResolution: true, Trust: &SigstoreIdentity{Issuer: OfficialSigstoreIssuer, Repository: OfficialSigstoreRepo}}}
	if err := WriteConfig(layout.ConfigPath(), config); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Refresh(context.Background(), config.Registries["test"]); err != nil {
		t.Fatal(err)
	}
	server.Close()
	store, err := NewStore(layout, RuntimeContext{CoreVersion: "0.2.24"})
	if err != nil {
		t.Fatal(err)
	}
	health, err := (&Manager{Store: store, RegistryClient: client}).RegistryHealth(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var found *RegistryHealth
	for index := range health {
		if health[index].Registry.Name == "test" {
			found = &health[index]
			break
		}
	}
	if found == nil || found.Status != "verified-cache" || found.Error == "" {
		t.Fatalf("registry health = %#v", health)
	}
}

func TestLoadCachedVerifiedRejectsBadSignature(t *testing.T) {
	now := time.Date(2026, 9, 15, 1, 0, 0, 0, time.UTC)
	server, _ := testRegistryServer(t, now)
	defer server.Close()
	client := RegistryClient{HTTPClient: server.Client(), Layout: testLayout(t), Now: func() time.Time { return now }, Verifier: testRegistryVerifier}
	registry := Registry{Name: "official", URL: server.URL, UnqualifiedResolution: true}
	if _, err := client.Refresh(context.Background(), registry); err != nil {
		t.Fatal(err)
	}
	client.Verifier = func(context.Context, []byte, []byte, SigstoreIdentity) error { return errors.New("signature rejected") }
	if _, err := client.LoadCachedVerified(context.Background(), registry, DefaultRegistryCacheTTL); err == nil {
		t.Fatal("bad cached signature accepted")
	}
}
