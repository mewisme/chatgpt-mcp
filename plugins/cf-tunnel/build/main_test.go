package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/licenseinventory"
	"go.mewis.me/chatgpt-mcp/internal/plugin"
	"go.mewis.me/chatgpt-mcp/internal/pluginbuild"
)

func TestPluginTemplateValidatesWithSentinelDigests(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest plugin.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if err := manifest.Validate(); err != nil {
		t.Fatal(err)
	}
	if manifest.ID != "cf-tunnel" || manifest.Type != "runtime" {
		t.Fatalf("manifest = %#v", manifest)
	}
	if len(manifest.Platforms) == 0 {
		t.Fatal("missing platforms")
	}
}

func TestBuildCurrentPlatform(t *testing.T) {
	platform := runtime.GOOS + "/" + runtime.GOARCH
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	template := filepath.Join(repoRoot, "plugins", "cf-tunnel", "plugin.json")
	data, err := os.ReadFile(template)
	if err != nil {
		t.Fatal(err)
	}
	var manifest plugin.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if _, ok := manifest.Platforms[platform]; !ok {
		t.Skip("template does not declare " + platform)
	}
	t.Setenv(pluginbuild.EnvUPX, pluginbuild.EnvUPXOff)
	t.Setenv(licenseinventory.EnvInventory, licenseinventory.EnvInventoryOff)
	output := t.TempDir()
	manifestPath, err := pluginbuild.Build(pluginbuild.Request{
		RepoRoot: repoRoot, TemplatePath: template, OutputRoot: output, OnlyPlatform: platform,
		Package: "./plugins/cf-tunnel/cmd/cf-tunnel",
	})
	if err != nil {
		t.Fatal(err)
	}
	generated, err := plugin.ParseManifest(mustRead(t, manifestPath))
	if err != nil {
		t.Fatal(err)
	}
	artifact := generated.Platforms[platform]
	if artifact.SHA256 == pluginbuild.DigestSentinel || artifact.SHA256 == "" {
		t.Fatalf("digest not generated: %#v", artifact)
	}
	if _, err := os.Stat(filepath.Join(output, artifact.Artifact)); err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
