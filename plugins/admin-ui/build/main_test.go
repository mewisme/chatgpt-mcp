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
	sourceRoot := filepath.Join(t.TempDir(), "web")
	if err := os.MkdirAll(filepath.Join(sourceRoot, "assets"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "assets", "app.js"), []byte("console.log(1)"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "index.html"), []byte("<!doctype html>"), 0644); err != nil {
		t.Fatal(err)
	}
	templatePath := filepath.Join(t.TempDir(), "plugin.json")
	manifest := plugin.Manifest{
		Schema: plugin.ManifestSchema, ID: "admin-ui", Name: "Admin UI", Publisher: "mewisme", Version: "1.0.0", Type: "web-ui",
		Provides: []plugin.Capability{plugin.CapabilityWebUIAdmin}, Permissions: []plugin.Permission{},
		Platforms: map[string]plugin.PlatformArtifact{"any/any": {Artifact: "admin-ui-1.0.0.zip", SHA256: generatedDigestSentinel, Archive: "zip", Entrypoint: "index.html"}},
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
	if generated.Platforms["any/any"].SHA256 != firstDigest {
		t.Fatalf("manifest digest = %q, want %q", generated.Platforms["any/any"].SHA256, firstDigest)
	}
	reader, err := zip.OpenReader(firstArtifact)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	foundEntrypoint := false
	for _, entry := range reader.File {
		if entry.Name == "index.html" {
			foundEntrypoint = true
			if !entry.Modified.Equal(zipEpoch) {
				t.Fatalf("entrypoint timestamp = %s", entry.Modified)
			}
		}
	}
	if !foundEntrypoint {
		t.Fatal("generated archive is missing Admin UI entrypoint")
	}
}

func TestBuildRejectsNonSentinelTemplateDigest(t *testing.T) {
	templatePath := filepath.Join(t.TempDir(), "plugin.json")
	data := `{"schema":1,"id":"admin-ui","name":"Admin UI","publisher":"mewisme","version":"1.0.0","type":"web-ui","provides":["web-ui/admin"],"permissions":[],"platforms":{"any/any":{"artifact":"admin-ui-1.0.0.zip","sha256":"` + strings.Repeat("a", 64) + `","archive":"zip","entrypoint":"index.html"}}}`
	if err := os.WriteFile(templatePath, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := build(t.TempDir(), templatePath, t.TempDir()); err == nil {
		t.Fatal("non-sentinel source manifest digest accepted")
	}
}
