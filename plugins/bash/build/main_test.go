package main

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/plugin"
)

func TestBuildIsDeterministicAndGeneratesExactManifest(t *testing.T) {
	sourceRoot := filepath.Join(t.TempDir(), "portable")
	if err := os.MkdirAll(filepath.Join(sourceRoot, "usr", "bin"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "usr", "bin", "bash.exe"), []byte("bash"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "LICENSE.txt"), []byte("license"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sourceRoot, "etc"), 0755); err != nil {
		t.Fatal(err)
	}
	machineFiles := []string{"hosts", "networks", "protocols", "services"}
	for _, name := range machineFiles {
		if err := os.WriteFile(filepath.Join(sourceRoot, "etc", name), []byte("machine-specific"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	templatePath := filepath.Join(t.TempDir(), "plugin.json")
	manifest := plugin.Manifest{
		Schema: plugin.ManifestSchema, ID: "bash", Name: "Bash Runtime", Publisher: "mewisme", License: "Apache-2.0", Version: "1.0.0", Type: "runtime",
		Provides: []plugin.Capability{"shell/bash"}, Permissions: []plugin.Permission{plugin.PermissionProcessExecute},
		Platforms: map[string]plugin.PlatformArtifact{"windows/amd64": {Artifact: "bash-1.0.0-windows-amd64.zip", SHA256: generatedDigestSentinel, Archive: "zip", Entrypoint: "usr/bin/bash.exe"}},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(templatePath, data, 0644); err != nil {
		t.Fatal(err)
	}
	firstRoot := filepath.Join(t.TempDir(), "first")
	secondRoot := filepath.Join(t.TempDir(), "second")
	firstArtifact, firstManifest, err := build(sourceRoot, templatePath, firstRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range machineFiles {
		if err := os.WriteFile(filepath.Join(sourceRoot, "etc", name), []byte("different-machine"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	secondArtifact, _, err := build(sourceRoot, templatePath, secondRoot)
	if err != nil {
		t.Fatal(err)
	}
	firstDigest, err := fileSHA256(firstArtifact)
	if err != nil {
		t.Fatal(err)
	}
	secondDigest, err := fileSHA256(secondArtifact)
	if err != nil {
		t.Fatal(err)
	}
	if firstDigest != secondDigest {
		t.Fatalf("artifact digest differs: %s != %s", firstDigest, secondDigest)
	}
	generatedData, err := os.ReadFile(firstManifest)
	if err != nil {
		t.Fatal(err)
	}
	generated, err := plugin.ParseManifest(generatedData)
	if err != nil {
		t.Fatal(err)
	}
	if generated.Platforms["windows/amd64"].SHA256 != firstDigest {
		t.Fatalf("manifest digest = %q, want %q", generated.Platforms["windows/amd64"].SHA256, firstDigest)
	}
	reader, err := zip.OpenReader(firstArtifact)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	foundEntrypoint := false
	for _, entry := range reader.File {
		if machineSpecificPortablePath(entry.Name) {
			t.Fatalf("machine-specific PortableGit file was packaged: %s", entry.Name)
		}
		if entry.Name == "usr/bin/bash.exe" {
			foundEntrypoint = true
			if !entry.Modified.Equal(zipEpoch) {
				t.Fatalf("entrypoint timestamp = %s", entry.Modified)
			}
		}
	}
	if !foundEntrypoint {
		t.Fatal("generated archive is missing Bash entrypoint")
	}
}

func TestBuildRejectsNonSentinelTemplateDigest(t *testing.T) {
	templatePath := filepath.Join(t.TempDir(), "plugin.json")
	data := `{"schema":1,"id":"bash","name":"Bash Runtime","publisher":"mewisme","version":"1.0.0","type":"runtime","provides":["shell/bash"],"permissions":["process/execute"],"platforms":{"windows/amd64":{"artifact":"bash-1.0.0-windows-amd64.zip","sha256":"` + strings.Repeat("a", 64) + `","archive":"zip","entrypoint":"usr/bin/bash.exe"}}}`
	if err := os.WriteFile(templatePath, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := build(t.TempDir(), templatePath, t.TempDir()); err == nil {
		t.Fatal("non-sentinel source manifest digest accepted")
	}
}
