package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/plugin"
)

func TestValidatePluginRegistryChecksManifestAndArtifactConsistency(t *testing.T) {
	root, indexPath, publishersPath := registryValidationFixture(t)
	if err := validatePluginRegistry(indexPath, publishersPath, root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo-1.0.0.zip"), []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validatePluginRegistry(indexPath, publishersPath, root); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("tampered artifact accepted: %v", err)
	}
}

func TestValidatePluginRegistryAcceptsHostBackedPluginWithoutArtifact(t *testing.T) {
	root, indexPath, publishersPath := registryValidationFixture(t)
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	var index plugin.RegistryIndex
	if err := json.Unmarshal(data, &index); err != nil {
		t.Fatal(err)
	}
	index.Plugins["wrap"] = plugin.RegistryEntry{Publisher: "mewisme", Name: "Host Wrapper", Type: "command-wrapper", Stable: "1.0.0", Versions: map[plugin.Version]string{"1.0.0": "wrap-1.0.0.json"}}
	data, err = json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	manifest := plugin.Manifest{
		Schema: plugin.ManifestSchema, ID: "wrap", Name: "Host Wrapper", Publisher: "mewisme", Version: "1.0.0", Type: "command-wrapper",
		Provides: []plugin.Capability{"command-wrapper/wrap"}, Permissions: []plugin.Permission{plugin.PermissionProcessExecute},
		Platforms: map[string]plugin.PlatformArtifact{"linux/amd64": {Host: &plugin.HostExecutableSpec{Executable: "wrap", CommandWrapper: &plugin.HostCommandWrapper{Args: []string{"rewrite", "{command}"}, RewriteExitCodes: []int{0}}}}},
	}
	data, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "wrap-1.0.0.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := validatePluginRegistry(indexPath, publishersPath, root); err != nil {
		t.Fatal(err)
	}
}

func TestValidatePluginRegistryRejectsMissingChannelManifestAndOrphanManifest(t *testing.T) {
	root, indexPath, publishersPath := registryValidationFixture(t)
	manifestPath := filepath.Join(root, "demo-1.0.0.json")
	if err := os.Remove(manifestPath); err != nil {
		t.Fatal(err)
	}
	if err := validatePluginRegistry(indexPath, publishersPath, root); err == nil || !strings.Contains(err.Error(), "channel manifest") {
		t.Fatalf("missing stable manifest accepted: %v", err)
	}
	_, indexPath, publishersPath = registryValidationFixtureAt(t, root)
	if err := os.WriteFile(filepath.Join(root, "orphan-1.0.0.json"), []byte(`{"schema":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validatePluginRegistry(indexPath, publishersPath, root); err == nil || !strings.Contains(err.Error(), "not referenced") {
		t.Fatalf("orphan manifest accepted: %v", err)
	}
}

func registryValidationFixture(t *testing.T) (string, string, string) {
	t.Helper()
	return registryValidationFixtureAt(t, t.TempDir())
}

func registryValidationFixtureAt(t *testing.T, root string) (string, string, string) {
	t.Helper()
	artifactData := []byte("plugin-artifact")
	digest := sha256.Sum256(artifactData)
	manifest := plugin.Manifest{Schema: plugin.ManifestSchema, ID: "demo", Name: "Demo", Publisher: "mewisme", Version: "1.0.0", Type: "formatter", Provides: []plugin.Capability{"formatter/demo"}, Platforms: map[string]plugin.PlatformArtifact{"linux/amd64": {Artifact: "demo-1.0.0.zip", SHA256: hex.EncodeToString(digest[:]), Archive: "zip", Entrypoint: "bin/demo"}}}
	index := plugin.RegistryIndex{Schema: plugin.RegistrySchema, GeneratedAt: time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC), Plugins: map[plugin.PluginID]plugin.RegistryEntry{"demo": {Publisher: "mewisme", Name: "Demo", Type: "formatter", Stable: "1.0.0", Versions: map[plugin.Version]string{"1.0.0": "demo-1.0.0.json"}}}}
	publishers := plugin.PublisherIndex{Schema: plugin.PublishersSchema, Publishers: map[string]plugin.Publisher{"mewisme": {Name: "mewisme", Source: "https://github.com/mewisme/chatgpt-mcp", Trusted: true, Sigstore: plugin.SigstoreIdentity{Issuer: plugin.OfficialSigstoreIssuer, Repository: plugin.OfficialSigstoreRepo}}}}
	writeJSON := func(path string, value any) {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	indexPath, publishersPath := filepath.Join(root, "index.json"), filepath.Join(root, "publishers.json")
	writeJSON(indexPath, index)
	writeJSON(publishersPath, publishers)
	writeJSON(filepath.Join(root, "demo-1.0.0.json"), manifest)
	if err := os.WriteFile(filepath.Join(root, "demo-1.0.0.zip"), artifactData, 0600); err != nil {
		t.Fatal(err)
	}
	return root, indexPath, publishersPath
}
