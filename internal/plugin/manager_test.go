package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestParseReference(t *testing.T) {
	registry, id, version, err := ParseReference("community/bash@1.2.3")
	if err != nil || registry != "community" || id != "bash" || version != "1.2.3" {
		t.Fatalf("reference = %q %q %q %v", registry, id, version, err)
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
