package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestManagerInstallVerifyAndUninstall(t *testing.T) {
	archive := testZipBytes(t, "usr/bin/bash.exe", []byte("bash"))
	digest := sha256.Sum256(archive)
	manifest := testManifest("bash", "1.0.0", "shell/bash")
	artifact := PlatformArtifact{Artifact: "bash-1.0.0-windows-amd64.zip", Archive: "zip", Entrypoint: "usr/bin/bash.exe"}
	artifact.SHA256 = hex.EncodeToString(digest[:])
	manifest.Platforms = map[string]PlatformArtifact{"windows/amd64": artifact}
	generatedAt := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	publisher := Publisher{Name: "mewisme", Source: "https://github.com/mewisme/chatgpt-mcp", Trusted: true, Sigstore: SigstoreIdentity{Issuer: "https://token.actions.githubusercontent.com", Repository: "mewisme/chatgpt-mcp"}}
	index := RegistryIndex{Schema: RegistrySchema, GeneratedAt: generatedAt, Plugins: map[PluginID]RegistryEntry{"bash": {Publisher: "mewisme", Name: "Bash", Type: "runtime", Stable: "1.0.0", Versions: map[Version]string{"1.0.0": "bash-1.0.0.json"}}}}
	publishers := PublisherIndex{Schema: PublishersSchema, Publishers: map[string]Publisher{"mewisme": publisher}}
	indexData, _ := json.Marshal(index)
	publishersData, _ := json.Marshal(publishers)
	manifestData, _ := json.Marshal(manifest)
	assets := map[string][]byte{
		"index.json": indexData, "index.json.sigstore.json": []byte(`{"sig":1}`), "publishers.json": publishersData, "publishers.json.sigstore.json": []byte(`{"sig":1}`),
		"bash-1.0.0.json": manifestData, "bash-1.0.0.json.sigstore.json": []byte(`{"sig":1}`), artifact.Artifact: archive,
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		data, ok := assets[strings.TrimPrefix(request.URL.Path, "/")]
		if !ok {
			http.NotFound(writer, request)
			return
		}
		_, _ = writer.Write(data)
	}))
	defer server.Close()
	root := t.TempDir()
	layout := Layout{ConfigRoot: filepath.Join(root, "config"), DataRoot: filepath.Join(root, "data"), CacheRoot: filepath.Join(root, "cache")}
	store, err := NewStore(layout, RuntimeContext{OS: "windows", Arch: "amd64", CoreVersion: "0.2.24"})
	if err != nil {
		t.Fatal(err)
	}
	registry := Registry{Name: "official", URL: server.URL, UnqualifiedResolution: true}
	client := RegistryClient{HTTPClient: server.Client(), Layout: layout, Verifier: testRegistryVerifier}
	manager := Manager{Store: store, RegistryClient: client, HTTPClient: server.Client()}
	oldOfficial := OfficialRegistryBaseURL
	_ = oldOfficial
	config := NewConfig()
	trust := SigstoreIdentity{Issuer: OfficialSigstoreIssuer, Repository: OfficialSigstoreRepo}
	config.Registries["test"] = Registry{Name: "test", URL: server.URL, UnqualifiedResolution: true, Trust: &trust}
	if err := WriteConfig(layout.ConfigPath(), config); err != nil {
		t.Fatal(err)
	}
	clientSnapshot := RegistrySnapshot{Registry: registry, Index: index, Publishers: publishers}
	if err := client.writeCache(registry, indexData, []byte(`{"sig":1}`), publishersData, []byte(`{"sig":1}`), generatedAt); err != nil {
		t.Fatal(err)
	}
	_ = clientSnapshot
	result, err := installResolvedForTest(context.Background(), manager, ResolvedPlugin{Registry: Registry{Name: "test", URL: server.URL, UnqualifiedResolution: true}, PluginID: "bash", Version: "1.0.0", Entry: index.Plugins["bash"], Publisher: publisher, ManifestName: "bash-1.0.0.json"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Plugin.Manifest.ID != "bash" {
		t.Fatalf("installed = %#v", result)
	}
	if err := manager.Verify(context.Background(), "bash"); err != nil {
		t.Fatal(err)
	}
	if err := manager.Uninstall(context.Background(), "bash", false); err != nil {
		t.Fatal(err)
	}
	lock, err := LoadLock(layout.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := lock.Plugins["bash"]; ok {
		t.Fatal("plugin remained active after uninstall")
	}
}

func TestManagerUninstallRefusesActiveDependentWithoutForce(t *testing.T) {
	store := testStore(t)
	trust := ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}
	provider := testManifest("bash", "1.0.0", "shell/bash")
	if _, err := store.Install(provider, testPayload(t, "bash")); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate("bash", "1.0.0", trust); err != nil {
		t.Fatal(err)
	}
	consumer := testManifest("consumer", "1.0.0", "formatter/consumer")
	consumer.Dependencies.Capabilities = []Capability{"shell/bash"}
	if _, err := store.Install(consumer, testPayload(t, "consumer")); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate("consumer", "1.0.0", trust); err != nil {
		t.Fatal(err)
	}
	manager := Manager{Store: store}
	if err := manager.Uninstall(context.Background(), "bash", false); err == nil {
		t.Fatal("dependency provider uninstalled without force")
	}
	if err := manager.Uninstall(context.Background(), "bash", true); err != nil {
		t.Fatal(err)
	}
	lock, err := LoadLock(store.layout.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	if lock.Plugins["consumer"].Enabled {
		t.Fatal("forced uninstall left dependent plugin enabled")
	}
	config, err := LoadConfig(store.layout.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := config.Desired["bash"]; ok {
		t.Fatal("uninstalled plugin remained in desired state")
	}
	if desired := config.Desired["consumer"]; desired.Enabled {
		t.Fatalf("forced dependent desired state remained enabled: %#v", desired)
	}
}

func TestManagerPruneVersionsRetainsActiveAndNewestRollbackVersions(t *testing.T) {
	store := testStore(t)
	trust := ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}
	for _, version := range []string{"1.0.0", "1.1.0", "1.2.0", "2.0.0"} {
		if _, err := store.Install(testManifest("bash", version, "shell/bash"), testPayload(t, "bash")); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Activate("bash", "2.0.0", trust); err != nil {
		t.Fatal(err)
	}
	manager := Manager{Store: store}
	removed, err := manager.PruneVersions("bash", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != "1.0.0" {
		t.Fatalf("removed versions = %#v", removed)
	}
	versions, err := store.InstalledVersions("bash")
	if err != nil {
		t.Fatal(err)
	}
	want := []Version{"1.1.0", "1.2.0", "2.0.0"}
	if len(versions) != len(want) {
		t.Fatalf("retained versions = %#v", versions)
	}
	for index := range want {
		if versions[index] != want[index] {
			t.Fatalf("retained versions = %#v want %#v", versions, want)
		}
	}
	previous, err := manager.previousVersion("bash", "2.0.0")
	if err != nil || previous != "1.2.0" {
		t.Fatalf("previous version = %q err=%v", previous, err)
	}
}

func TestManagerPruneVersionsNeverRemovesActiveVersion(t *testing.T) {
	store := testStore(t)
	trust := ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}
	for _, version := range []string{"1.0.0", "2.0.0"} {
		if _, err := store.Install(testManifest("bash", version, "shell/bash"), testPayload(t, "bash")); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Activate("bash", "1.0.0", trust); err != nil {
		t.Fatal(err)
	}
	removed, err := (&Manager{Store: store}).PruneVersions("bash", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != "2.0.0" {
		t.Fatalf("removed versions = %#v", removed)
	}
	if _, err := store.Installed("bash", "1.0.0"); err != nil {
		t.Fatalf("active version removed: %v", err)
	}
}

func TestManagerPruneAllVersionsRemovesOrphansAndRetainsActiveRollback(t *testing.T) {
	store := testStore(t)
	trust := ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}
	for _, version := range []string{"1.0.0", "1.1.0", "2.0.0"} {
		if _, err := store.Install(testManifest("bash", version, "shell/bash"), testPayload(t, "bash")); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Activate("bash", "2.0.0", trust); err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"1.0.0", "1.1.0"} {
		if _, err := store.Install(testManifest("orphan", version, "formatter/orphan"), testPayload(t, "orphan")); err != nil {
			t.Fatal(err)
		}
	}
	removed, err := (&Manager{Store: store}).PruneAllVersions(1)
	if err != nil {
		t.Fatal(err)
	}
	if got := removed["bash"]; len(got) != 1 || got[0] != "1.0.0" {
		t.Fatalf("active plugin removed versions = %#v", got)
	}
	if got := removed["orphan"]; len(got) != 2 {
		t.Fatalf("orphan removed versions = %#v", got)
	}
	versions, err := store.InstalledVersions("bash")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 || versions[0] != "1.1.0" || versions[1] != "2.0.0" {
		t.Fatalf("retained versions = %#v", versions)
	}
	versions, err = store.InstalledVersions("orphan")
	if err != nil || len(versions) != 0 {
		t.Fatalf("orphan versions = %#v err=%v", versions, err)
	}
}

func TestManagerPruneCacheRemovesOnlyPluginCache(t *testing.T) {
	store := testStore(t)
	pluginCache := filepath.Join(store.layout.CacheRoot, "plugins", "downloads")
	staleExtract := filepath.Join(store.layout.CacheRoot, ".plugin-extract-stale")
	unrelated := filepath.Join(store.layout.CacheRoot, "keep.txt")
	for _, path := range []string{pluginCache, staleExtract} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(pluginCache, "artifact.tar.gz"), []byte("cache"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(unrelated, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := (&Manager{Store: store}).PruneCache(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store.layout.CacheRoot, "plugins")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("plugin cache still exists: %v", err)
	}
	if _, err := os.Stat(staleExtract); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale extraction cache still exists: %v", err)
	}
	if data, err := os.ReadFile(unrelated); err != nil || string(data) != "keep" {
		t.Fatalf("unrelated cache changed: %q err=%v", data, err)
	}
}

func TestManagerRollbackRefetchesRetainedVersionBeforeActivation(t *testing.T) {
	archive := testZipBytes(t, "usr/bin/bash.exe", []byte("signed-v1"))
	digest := sha256.Sum256(archive)
	v1 := testManifest("bash", "1.0.0", "shell/bash")
	v1Artifact := PlatformArtifact{Artifact: "bash-1.0.0-windows-amd64.zip", SHA256: hex.EncodeToString(digest[:]), Archive: "zip", Entrypoint: "usr/bin/bash.exe"}
	v1.Platforms = map[string]PlatformArtifact{"windows/amd64": v1Artifact}
	v2 := testManifest("bash", "2.0.0", "shell/bash")
	v2.Platforms = map[string]PlatformArtifact{"windows/amd64": {Artifact: "bash-2.0.0-windows-amd64.zip", SHA256: strings.Repeat("b", 64), Archive: "zip", Entrypoint: "usr/bin/bash.exe"}}
	publisher := Publisher{Name: "mewisme", Source: "https://github.com/mewisme/chatgpt-mcp", Trusted: true, Sigstore: SigstoreIdentity{Issuer: OfficialSigstoreIssuer, Repository: OfficialSigstoreRepo}}
	generatedAt := time.Date(2026, 9, 15, 2, 0, 0, 0, time.UTC)
	index := RegistryIndex{Schema: RegistrySchema, GeneratedAt: generatedAt, Plugins: map[PluginID]RegistryEntry{"bash": {Publisher: "mewisme", Name: "Bash", Type: "runtime", Stable: "2.0.0", Versions: map[Version]string{"1.0.0": "bash-1.0.0.json", "2.0.0": "bash-2.0.0.json"}}}}
	publishers := PublisherIndex{Schema: PublishersSchema, Publishers: map[string]Publisher{"mewisme": publisher}}
	indexData, _ := json.Marshal(index)
	publishersData, _ := json.Marshal(publishers)
	v1Data, _ := json.Marshal(v1)
	assets := map[string][]byte{
		"index.json": indexData, "index.json.sigstore.json": []byte(`{"sig":1}`), "publishers.json": publishersData, "publishers.json.sigstore.json": []byte(`{"sig":1}`),
		"bash-1.0.0.json": v1Data, "bash-1.0.0.json.sigstore.json": []byte(`{"sig":1}`), v1Artifact.Artifact: archive,
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		data, ok := assets[strings.TrimPrefix(request.URL.Path, "/")]
		if !ok {
			http.NotFound(writer, request)
			return
		}
		_, _ = writer.Write(data)
	}))
	defer server.Close()
	root := t.TempDir()
	layout := Layout{ConfigRoot: filepath.Join(root, "config"), DataRoot: filepath.Join(root, "data"), CacheRoot: filepath.Join(root, "cache")}
	store, err := NewStore(layout, RuntimeContext{OS: "windows", Arch: "amd64", CoreVersion: "0.2.24"})
	if err != nil {
		t.Fatal(err)
	}
	trust := SigstoreIdentity{Issuer: OfficialSigstoreIssuer, Repository: OfficialSigstoreRepo}
	config := NewConfig()
	if err := config.AddRegistry("test", server.URL, true, trust); err != nil {
		t.Fatal(err)
	}
	if err := WriteConfig(layout.ConfigPath(), config); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		manifest Manifest
		contents string
	}{{manifest: v1, contents: "stale-v1"}, {manifest: v2, contents: "active-v2"}} {
		payload := t.TempDir()
		entrypoint := filepath.Join(payload, "usr", "bin", "bash.exe")
		if err := os.MkdirAll(filepath.Dir(entrypoint), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(entrypoint, []byte(fixture.contents), 0700); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Install(fixture.manifest, payload); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Activate("bash", "2.0.0", ActivationTrust{Registry: "test", Publisher: "mewisme", Trusted: true}); err != nil {
		t.Fatal(err)
	}
	manager := Manager{Store: store, RegistryClient: RegistryClient{HTTPClient: server.Client(), Layout: layout, Verifier: testRegistryVerifier}, HTTPClient: server.Client()}
	result, err := manager.Rollback(context.Background(), "bash", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if result.Plugin.Manifest.Version != "1.0.0" {
		t.Fatalf("rollback result = %#v", result)
	}
	payload, err := os.ReadFile(result.Plugin.Entrypoint)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != "signed-v1" {
		t.Fatalf("rollback reused stale payload: %q", payload)
	}
	lock, err := LoadLock(layout.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	if lock.Plugins["bash"].Version != "1.0.0" || !lock.Plugins["bash"].Enabled {
		t.Fatalf("rollback lock = %#v", lock.Plugins["bash"])
	}
}

func TestManagerVerifyAndReconcileDetectPackagedPayloadTamper(t *testing.T) {
	store := testStore(t)
	manifest := testManifest("demo", "1.0.0", "formatter/demo")
	installed, err := store.Install(manifest, testPayload(t, "demo"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Activate("demo", "1.0.0", ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(installed.Entrypoint, []byte("tampered"), 0700); err != nil {
		t.Fatal(err)
	}
	manager := Manager{Store: store}
	if err := manager.Verify(context.Background(), "demo"); err == nil || !strings.Contains(err.Error(), "packaged payload integrity verification failed") {
		t.Fatalf("tampered payload verify error = %v", err)
	}
	report, err := Reconcile(store)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Disabled) != 1 || report.Disabled[0] != "demo" || !strings.Contains(report.Issues["demo"], "packaged payload integrity verification failed") {
		t.Fatalf("tampered payload reconcile report = %#v", report)
	}
}

func TestManagerRollbackUsesRetainedVerifiedStateOffline(t *testing.T) {
	store := testStore(t)
	trust := ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}
	for _, version := range []string{"1.0.0", "2.0.0"} {
		if _, err := store.Install(testManifest("bash", version, "shell/bash"), testPayload(t, "bash")); err != nil {
			t.Fatal(err)
		}
		if err := store.Activate("bash", Version(version), trust); err != nil {
			t.Fatal(err)
		}
	}
	result, err := (&Manager{Store: store}).Rollback(context.Background(), "bash", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if result.Plugin.Manifest.Version != "1.0.0" || result.Registry.Name != OfficialRegistryName || result.Publisher.Name != "mewisme" {
		t.Fatalf("offline rollback result = %#v", result)
	}
	lock, err := LoadLock(store.layout.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	if lock.Plugins["bash"].Version != "1.0.0" || !lock.Plugins["bash"].Enabled {
		t.Fatalf("offline rollback lock = %#v", lock.Plugins["bash"])
	}
}

func TestParseReference(t *testing.T) {
	registry, id, version, err := ParseReference("community/bash@1.2.3")
	if err != nil || registry != "community" || id != "bash" || version != "1.2.3" {
		t.Fatalf("reference = %q %q %q %v", registry, id, version, err)
	}
}

func TestManagerResolveRejectsUnverifiedCachedRegistryFallback(t *testing.T) {
	now := time.Date(2026, 9, 15, 1, 0, 0, 0, time.UTC)
	server, _ := testRegistryServer(t, now)
	layout := testLayout(t)
	client := RegistryClient{HTTPClient: server.Client(), Layout: layout, Now: func() time.Time { return now }, Verifier: testRegistryVerifier}
	registry := Registry{Name: "community", URL: server.URL, UnqualifiedResolution: true, Trust: &SigstoreIdentity{Issuer: OfficialSigstoreIssuer, Repository: OfficialSigstoreRepo}}
	config := NewConfig()
	config.Registries[registry.Name] = registry
	if err := WriteConfig(layout.ConfigPath(), config); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Refresh(context.Background(), registry); err != nil {
		t.Fatal(err)
	}
	server.Close()
	client.Verifier = func(context.Context, []byte, []byte, SigstoreIdentity) error {
		return errors.New("cached signature rejected")
	}
	store, err := NewStore(layout, RuntimeContext{OS: "linux", Arch: "amd64", CoreVersion: "0.2.24"})
	if err != nil {
		t.Fatal(err)
	}
	manager := Manager{Store: store, RegistryClient: client}
	if _, err := manager.Resolve(context.Background(), "community/bash"); err == nil || !strings.Contains(err.Error(), "cached signature rejected") {
		t.Fatalf("unverified cached registry accepted: %v", err)
	}
}

func TestManagerResolveQualifiedRegistryIgnoresUnrelatedOfficialFailure(t *testing.T) {
	now := time.Date(2026, 9, 15, 1, 0, 0, 0, time.UTC)
	server, _ := testRegistryServer(t, now)
	defer server.Close()
	layout := testLayout(t)
	store, err := NewStore(layout, RuntimeContext{OS: "linux", Arch: "amd64", CoreVersion: "0.2.24"})
	if err != nil {
		t.Fatal(err)
	}
	config := NewConfig()
	trust := SigstoreIdentity{Issuer: OfficialSigstoreIssuer, Repository: OfficialSigstoreRepo}
	if err := config.AddRegistry("community", server.URL, false, trust); err != nil {
		t.Fatal(err)
	}
	if err := WriteConfig(layout.ConfigPath(), config); err != nil {
		t.Fatal(err)
	}
	manager := Manager{Store: store, RegistryClient: RegistryClient{HTTPClient: server.Client(), Layout: layout, Verifier: testRegistryVerifier}}
	resolved, err := manager.Resolve(context.Background(), "community/bash")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Registry.Name != "community" || resolved.PluginID != "bash" {
		t.Fatalf("resolved = %#v", resolved)
	}
}

func installResolvedForTest(ctx context.Context, manager Manager, resolved ResolvedPlugin) (InstallResult, error) {
	manifest, _, err := manager.RegistryClient.FetchManifest(ctx, resolved)
	if err != nil {
		return InstallResult{}, err
	}
	artifact, err := manifest.Platform(manager.Store.runtime.OS, manager.Store.runtime.Arch)
	if err != nil {
		return InstallResult{}, err
	}
	archivePath, err := manager.downloadArtifact(ctx, resolved.Registry.URL, artifact)
	if err != nil {
		return InstallResult{}, err
	}
	extracted := filepath.Join(manager.Store.layout.CacheRoot, "test-extract")
	if err := ExtractArchive(archivePath, artifact.Archive, extracted); err != nil {
		return InstallResult{}, err
	}
	installed, err := manager.Store.Install(manifest, extracted)
	if err != nil {
		return InstallResult{}, err
	}
	if err := manager.Store.Activate(manifest.ID, manifest.Version, ActivationTrust{Registry: resolved.Registry.Name, Publisher: resolved.Publisher.Name, Trusted: true}); err != nil {
		return InstallResult{}, err
	}
	return InstallResult{Plugin: installed, Registry: resolved.Registry, Publisher: resolved.Publisher}, nil
}
