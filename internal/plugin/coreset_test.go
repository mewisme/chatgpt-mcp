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

func TestCoreSetFromIndexIgnoresOptionalPlugins(t *testing.T) {
	enabled := true
	specs := CoreSetFromIndex(RegistryIndex{Plugins: map[PluginID]RegistryEntry{
		"bash":     {Stable: "1.0.1"},
		"admin-ui": {Stable: "1.0.0", Core: &CorePolicy{Enabled: &enabled}},
		"rtk":      {Stable: "1.0.0"},
	}})
	if len(specs) != 1 || specs[0].ID != "admin-ui" || specs[0].Version != "1.0.0" || specs[0].Required || !specs[0].Enabled {
		t.Fatalf("core set = %#v", specs)
	}
}

func TestOfficialRegistryIndexDeclaresAdminUICore(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "plugins", "registry", "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	index, err := ParseRegistryIndex(data)
	if err != nil {
		t.Fatal(err)
	}
	entry := index.Plugins["admin-ui"]
	if entry.Core == nil || entry.Core.Required || !entry.Core.defaultEnabled() {
		t.Fatalf("admin-ui core = %#v", entry.Core)
	}
	if got := CoreSetFromIndex(index); len(got) != 3 {
		t.Fatalf("official core set = %#v", got)
	}
	ids := map[PluginID]bool{}
	for _, spec := range CoreSetFromIndex(index) {
		ids[spec.ID] = true
	}
	if !ids["admin-ui"] || !ids["secure-mcp-tunnel"] || !ids["tui"] {
		t.Fatalf("official core set = %#v", ids)
	}
	for _, id := range []PluginID{"bash", "rtk", "cf-tunnel"} {
		if index.Plugins[id].Core != nil {
			t.Fatalf("optional plugin %s marked core: %#v", id, index.Plugins[id].Core)
		}
	}
}

func TestRegistryRejectsCoreWithoutStable(t *testing.T) {
	snapshot := testRegistrySnapshot("official", true)
	entry := snapshot.Index.Plugins["bash"]
	entry.Core = &CorePolicy{Required: true}
	entry.Stable = ""
	snapshot.Index.Plugins["bash"] = entry
	if err := snapshot.Index.Validate(); err == nil {
		t.Fatal("core plugin without stable accepted")
	}
}

func TestReconcileCoreSetInstallsCoreAndSkipsOptional(t *testing.T) {
	env := newCoreTestEnv(t, []coreTestPlugin{
		{id: "admin-ui", version: "1.0.0", core: &CorePolicy{Enabled: boolPtr(true)}},
		{id: "bash", version: "1.0.0"},
	}, "")
	report, err := env.manager.reconcileCoreSet(context.Background(), env.snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Items) != 1 || report.Items[0].ID != "admin-ui" || report.Items[0].Action != CoreActionInstalled {
		t.Fatalf("report = %#v", report)
	}
	lock, err := LoadLock(env.layout.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := lock.Plugins["bash"]; ok {
		t.Fatal("optional plugin auto-installed")
	}
	item, err := env.manager.CatalogPlugin("admin-ui")
	if err != nil {
		t.Fatal(err)
	}
	if item.Origin != OriginInstalled || !item.Lifecycle.Uninstall || !item.Lifecycle.Update || !item.Lifecycle.Rollback || !item.Lifecycle.Verify {
		t.Fatalf("catalog = %#v", item)
	}
	if err := env.manager.Verify(context.Background(), "admin-ui"); err != nil {
		t.Fatal(err)
	}
	if err := env.manager.Uninstall(context.Background(), "admin-ui", false); err != nil {
		t.Fatal(err)
	}
}

func TestReconcileCoreSetPreservesUserDisable(t *testing.T) {
	env := newCoreTestEnv(t, []coreTestPlugin{{id: "admin-ui", version: "1.0.0", core: &CorePolicy{Enabled: boolPtr(true)}}}, "")
	if _, err := env.manager.reconcileCoreSet(context.Background(), env.snapshot); err != nil {
		t.Fatal(err)
	}
	if err := env.manager.Store.SetEnabled("admin-ui", false); err != nil {
		t.Fatal(err)
	}
	report, err := env.manager.reconcileCoreSet(context.Background(), env.snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Items) != 1 || report.Items[0].Action != CoreActionRetained {
		t.Fatalf("report = %#v", report)
	}
	lock, err := LoadLock(env.layout.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	if lock.Plugins["admin-ui"].Enabled {
		t.Fatal("user disable was overwritten")
	}
}

func TestReconcileCoreSetReenablesRequiredPlugin(t *testing.T) {
	env := newCoreTestEnv(t, []coreTestPlugin{{id: "admin-ui", version: "1.0.0", core: &CorePolicy{Required: true}}}, "")
	if _, err := env.manager.reconcileCoreSet(context.Background(), env.snapshot); err != nil {
		t.Fatal(err)
	}
	if err := env.manager.Store.SetEnabled("admin-ui", false); err != nil {
		t.Fatal(err)
	}
	report, err := env.manager.reconcileCoreSet(context.Background(), env.snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Items) != 1 || report.Items[0].Action != CoreActionEnabled {
		t.Fatalf("report = %#v", report)
	}
	lock, err := LoadLock(env.layout.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	if !lock.Plugins["admin-ui"].Enabled {
		t.Fatal("required core plugin remained disabled")
	}
}

func TestReconcileCoreSetRequiredFailure(t *testing.T) {
	env := newCoreTestEnv(t, []coreTestPlugin{{id: "admin-ui", version: "1.0.0", core: &CorePolicy{Required: true}, omitArtifact: true}}, "")
	report, err := env.manager.reconcileCoreSet(context.Background(), env.snapshot)
	if err == nil || !strings.Contains(err.Error(), "required core plugin admin-ui failed") {
		t.Fatalf("required failure = %v", err)
	}
	if len(report.Items) != 1 || report.Items[0].Action != CoreActionFailed || !report.Items[0].Required {
		t.Fatalf("report = %#v", report)
	}
}

func TestReconcileCoreSetRemainsOrdinaryPlugin(t *testing.T) {
	env := newCoreTestEnv(t, []coreTestPlugin{
		{id: "admin-ui", version: "1.0.0"},
		{id: "admin-ui", version: "1.1.0", core: &CorePolicy{Enabled: boolPtr(true)}},
	}, "1.1.0")
	v1 := env.resolved("admin-ui", "1.0.0")
	if _, err := env.manager.installResolvedWithOptions(context.Background(), v1, false, true, InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	report, err := env.manager.reconcileCoreSet(context.Background(), env.snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Items) != 1 || report.Items[0].Action != CoreActionUpdated || report.Items[0].Version != "1.1.0" {
		t.Fatalf("report = %#v", report)
	}
	result, err := env.manager.Rollback(context.Background(), "admin-ui", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if result.Plugin.Manifest.Version != "1.0.0" {
		t.Fatalf("rollback = %#v", result)
	}
}

func TestReconcileCoreSetUsesSidecarBundle(t *testing.T) {
	env := newCoreTestEnv(t, []coreTestPlugin{
		{id: "admin-ui", version: "1.0.0", core: &CorePolicy{Enabled: boolPtr(true)}},
		{id: "bash", version: "1.0.0"},
	}, "")
	sidecar := t.TempDir()
	for name, data := range env.assets {
		if err := os.WriteFile(filepath.Join(sidecar, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	client := &http.Client{Transport: fatalRoundTripper{t}}
	env.manager.RegistryClient.LocalDir = sidecar
	env.manager.RegistryClient.HTTPClient = client
	env.manager.HTTPClient = client
	if err := env.manager.Store.layout.WriteConfig(NewConfig()); err != nil {
		t.Fatal(err)
	}
	report, err := env.manager.ReconcileCoreSet(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Items) != 1 || report.Items[0].Action != CoreActionInstalled {
		t.Fatalf("sidecar report = %#v", report)
	}
	lock, err := LoadLock(env.layout.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := lock.Plugins["bash"]; ok {
		t.Fatal("optional plugin auto-installed from sidecar")
	}
}

func TestRegistryClientFetchPrefersSidecar(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.json"), []byte(`{"ok":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	client := RegistryClient{LocalDir: dir, HTTPClient: &http.Client{Transport: fatalRoundTripper{t}}}
	data, err := client.fetch(context.Background(), OfficialRegistryBaseURL, "index.json", maxRegistryMetadataSize)
	if err != nil || string(data) != `{"ok":true}` {
		t.Fatalf("sidecar fetch = %q err=%v", data, err)
	}
}

type coreTestPlugin struct {
	id           string
	version      string
	core         *CorePolicy
	omitArtifact bool
}

type coreTestEnv struct {
	manager  Manager
	layout   Layout
	snapshot RegistrySnapshot
	assets   map[string][]byte
	entries  map[PluginID]RegistryEntry
}

func (env coreTestEnv) resolved(id, version string) ResolvedPlugin {
	entry := env.entries[PluginID(id)]
	return ResolvedPlugin{
		Registry: env.snapshot.Registry, PluginID: PluginID(id), Version: Version(version),
		Entry: entry, Publisher: env.snapshot.Publishers.Publishers["mewisme"], ManifestName: entry.Versions[Version(version)],
	}
}

func newCoreTestEnv(t *testing.T, plugins []coreTestPlugin, stableOverride string) coreTestEnv {
	t.Helper()
	generatedAt := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC)
	publisher := Publisher{Name: "mewisme", Source: "https://github.com/mewisme/chatgpt-mcp", Trusted: true, Sigstore: SigstoreIdentity{Issuer: OfficialSigstoreIssuer, Repository: OfficialSigstoreRepo}}
	entries := map[PluginID]RegistryEntry{}
	assets := map[string][]byte{}
	for _, plugin := range plugins {
		archive := testZipBytes(t, "bin/"+plugin.id, []byte(plugin.id+"-"+plugin.version))
		digest := sha256.Sum256(archive)
		artifactName := plugin.id + "-" + plugin.version + "-linux-amd64.zip"
		manifest := testManifest(plugin.id, plugin.version, Capability("formatter/"+plugin.id))
		manifest.Type = "formatter"
		manifest.Permissions = nil
		manifest.Platforms = map[string]PlatformArtifact{"linux/amd64": {Artifact: artifactName, SHA256: hex.EncodeToString(digest[:]), Archive: "zip", Entrypoint: "bin/" + plugin.id}}
		manifestData, err := json.Marshal(manifest)
		if err != nil {
			t.Fatal(err)
		}
		manifestName := plugin.id + "-" + plugin.version + ".json"
		entry := entries[PluginID(plugin.id)]
		if entry.Versions == nil {
			entry = RegistryEntry{Publisher: "mewisme", Name: plugin.id, Description: plugin.id, Type: "formatter", Stable: Version(plugin.version), Versions: map[Version]string{}}
		}
		entry.Versions[Version(plugin.version)] = manifestName
		if plugin.core != nil {
			entry.Core = plugin.core
			entry.Stable = Version(plugin.version)
		}
		if stableOverride != "" && plugin.version == stableOverride {
			entry.Stable = Version(stableOverride)
			entry.Core = plugin.core
		}
		entries[PluginID(plugin.id)] = entry
		assets[manifestName] = manifestData
		assets[manifestName+".sigstore.json"] = []byte(`{"sig":1}`)
		if !plugin.omitArtifact {
			assets[artifactName] = archive
		}
	}
	index := RegistryIndex{Schema: RegistrySchema, GeneratedAt: generatedAt, Plugins: entries}
	publishers := PublisherIndex{Schema: PublishersSchema, Publishers: map[string]Publisher{"mewisme": publisher}}
	indexData, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	publishersData, err := json.Marshal(publishers)
	if err != nil {
		t.Fatal(err)
	}
	assets["index.json"] = indexData
	assets["index.json.sigstore.json"] = []byte(`{"sig":1}`)
	assets["publishers.json"] = publishersData
	assets["publishers.json.sigstore.json"] = []byte(`{"sig":1}`)
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		data, ok := assets[strings.TrimPrefix(request.URL.Path, "/")]
		if !ok {
			http.NotFound(writer, request)
			return
		}
		_, _ = writer.Write(data)
	}))
	t.Cleanup(server.Close)
	layout := testLayout(t)
	store, err := NewStore(layout, RuntimeContext{OS: "linux", Arch: "amd64", CoreVersion: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	if err := layout.WriteConfig(NewConfig()); err != nil {
		t.Fatal(err)
	}
	registry := Registry{Name: OfficialRegistryName, URL: server.URL, UnqualifiedResolution: true, Trust: &SigstoreIdentity{Issuer: OfficialSigstoreIssuer, Repository: OfficialSigstoreRepo}}
	client := RegistryClient{HTTPClient: server.Client(), Layout: layout, Verifier: testRegistryVerifier}
	manager := Manager{Store: store, RegistryClient: client, HTTPClient: server.Client()}
	snapshot := RegistrySnapshot{Registry: registry, Index: index, Publishers: publishers, FetchedAt: generatedAt}
	return coreTestEnv{manager: manager, layout: layout, snapshot: snapshot, assets: assets, entries: entries}
}

func boolPtr(value bool) *bool { return &value }

type fatalRoundTripper struct{ t *testing.T }

func (rt fatalRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	rt.t.Fatalf("unexpected network request: %s", request.URL)
	return nil, errors.New("network")
}
