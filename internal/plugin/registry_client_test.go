package plugin

import (
	"context"
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

func TestRegistryClientRefreshCachesVerifiedMetadata(t *testing.T) {
	now := time.Date(2026, 9, 15, 1, 0, 0, 0, time.UTC)
	server, assets := testRegistryServer(t, now)
	defer server.Close()
	client := RegistryClient{HTTPClient: server.Client(), Layout: testLayout(t), Now: func() time.Time { return now }, Verifier: testRegistryVerifier}
	registry := Registry{Name: "official", URL: server.URL, UnqualifiedResolution: true}
	snapshot, err := client.Refresh(context.Background(), registry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := snapshot.Resolve("bash", ""); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"index.json", "index.json.sigstore.json", "publishers.json", "publishers.json.sigstore.json", "metadata.json"} {
		if _, err := os.Stat(filepath.Join(client.cacheDir(registry), name)); err != nil {
			t.Fatalf("cache %s: %v", name, err)
		}
	}
	if len(assets) == 0 {
		t.Fatal("registry fixture is empty")
	}
	cached, err := client.LoadCached(registry, DefaultRegistryCacheTTL)
	if err != nil || !cached.FetchedAt.Equal(now) {
		t.Fatalf("cached snapshot = %#v, %v", cached, err)
	}
}

func TestRegistryClientRejectsInvalidSignatureWithoutCaching(t *testing.T) {
	now := time.Date(2026, 9, 15, 1, 0, 0, 0, time.UTC)
	server, _ := testRegistryServer(t, now)
	defer server.Close()
	layout := testLayout(t)
	client := RegistryClient{HTTPClient: server.Client(), Layout: layout, Now: func() time.Time { return now }, Verifier: func(context.Context, []byte, []byte, SigstoreIdentity) error { return errors.New("bad signature") }}
	registry := Registry{Name: "official", URL: server.URL, UnqualifiedResolution: true}
	if _, err := client.Refresh(context.Background(), registry); err == nil {
		t.Fatal("invalid signature accepted")
	}
	if _, err := os.Stat(client.cacheDir(registry)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unverified registry was cached: %v", err)
	}
}

func TestRegistryClientRejectsStaleAndCorruptCache(t *testing.T) {
	now := time.Date(2026, 9, 15, 1, 0, 0, 0, time.UTC)
	server, _ := testRegistryServer(t, now)
	defer server.Close()
	client := RegistryClient{HTTPClient: server.Client(), Layout: testLayout(t), Now: func() time.Time { return now }, Verifier: testRegistryVerifier}
	registry := Registry{Name: "official", URL: server.URL, UnqualifiedResolution: true}
	if _, err := client.Refresh(context.Background(), registry); err != nil {
		t.Fatal(err)
	}
	client.Now = func() time.Time { return now.Add(48 * time.Hour) }
	if _, err := client.LoadCached(registry, 24*time.Hour); err == nil {
		t.Fatal("stale registry cache accepted")
	}
	client.Now = func() time.Time { return now }
	if err := os.WriteFile(filepath.Join(client.cacheDir(registry), "index.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := client.LoadCached(registry, 24*time.Hour); err == nil {
		t.Fatal("corrupt registry cache accepted")
	}
}

func TestRegistryClientFetchManifestVerifiesIdentity(t *testing.T) {
	now := time.Date(2026, 9, 15, 1, 0, 0, 0, time.UTC)
	server, _ := testRegistryServer(t, now)
	defer server.Close()
	client := RegistryClient{HTTPClient: server.Client(), Layout: testLayout(t), Verifier: testRegistryVerifier}
	registry := Registry{Name: "official", URL: server.URL, UnqualifiedResolution: true}
	snapshot, err := client.Refresh(context.Background(), registry)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := snapshot.Resolve("bash", "")
	if err != nil {
		t.Fatal(err)
	}
	manifest, signature, err := client.FetchManifest(context.Background(), resolved)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ID != "bash" || len(signature) == 0 {
		t.Fatalf("manifest = %#v, signature bytes = %d", manifest, len(signature))
	}
}

func TestRegistryClientPinsOfficialTrustIdentity(t *testing.T) {
	now := time.Date(2026, 9, 15, 1, 0, 0, 0, time.UTC)
	server, _ := testRegistryServer(t, now)
	defer server.Close()
	var identities []SigstoreIdentity
	client := RegistryClient{HTTPClient: server.Client(), Layout: testLayout(t), Verifier: func(_ context.Context, _, _ []byte, identity SigstoreIdentity) error {
		identities = append(identities, identity)
		return nil
	}}
	if _, err := client.Refresh(context.Background(), Registry{Name: OfficialRegistryName, URL: server.URL, UnqualifiedResolution: true}); err != nil {
		t.Fatal(err)
	}
	if len(identities) != 2 {
		t.Fatalf("verified identities = %#v", identities)
	}
	for _, identity := range identities {
		if identity.Issuer != OfficialSigstoreIssuer || identity.Repository != OfficialSigstoreRepo {
			t.Fatalf("official trust was not pinned: %#v", identity)
		}
	}
}

func TestSecurePluginHTTPClientRejectsCrossHostRedirect(t *testing.T) {
	client := securePluginHTTPClient(nil, time.Second, "plugins.example.test")
	request, err := http.NewRequest(http.MethodGet, "https://evil.example.test/asset", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.CheckRedirect(request, nil); err == nil {
		t.Fatal("cross-host plugin redirect accepted")
	}
}

func TestSafeRegistryAssetNameRejectsTraversalSignature(t *testing.T) {
	if safeRegistryAssetName("../index.json.sigstore.json") {
		t.Fatal("traversal signature asset accepted")
	}
}

func testRegistryServer(t *testing.T, generatedAt time.Time) (*httptest.Server, map[string][]byte) {
	t.Helper()
	snapshot := testRegistrySnapshot("official", true)
	snapshot.Index.GeneratedAt = generatedAt
	index, err := json.Marshal(snapshot.Index)
	if err != nil {
		t.Fatal(err)
	}
	publishers, err := json.Marshal(snapshot.Publishers)
	if err != nil {
		t.Fatal(err)
	}
	manifest := testManifest("bash", "1.0.0", "shell/bash")
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	assets := map[string][]byte{
		"index.json": index, "index.json.sigstore.json": []byte(`{"signature":"index"}`),
		"publishers.json": publishers, "publishers.json.sigstore.json": []byte(`{"signature":"publishers"}`),
		"bash-1.0.0.json": manifestData, "bash-1.0.0.json.sigstore.json": []byte(`{"signature":"manifest"}`),
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		name := strings.TrimPrefix(request.URL.Path, "/")
		data, ok := assets[name]
		if !ok {
			http.NotFound(writer, request)
			return
		}
		_, _ = writer.Write(data)
	}))
	return server, assets
}

func testRegistryVerifier(_ context.Context, artifact, signature []byte, identity SigstoreIdentity) error {
	if len(artifact) == 0 || len(signature) == 0 || identity.Repository != "mewisme/chatgpt-mcp" {
		return errors.New("invalid fixture signature")
	}
	return nil
}

func testLayout(t *testing.T) Layout {
	t.Helper()
	root := t.TempDir()
	return Layout{ConfigRoot: filepath.Join(root, "config"), DataRoot: filepath.Join(root, "data"), CacheRoot: filepath.Join(root, "cache")}
}
