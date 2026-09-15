package application

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	"go.mewis.me/chatgpt-mcp/internal/version"
)

func TestAppendUpdateNoticePreservesCompatibilityWarning(t *testing.T) {
	got := appendUpdateNotice("Warning: plugin incompatible", "Runtime restart skipped")
	if got != "Warning: plugin incompatible; Runtime restart skipped" {
		t.Fatalf("notice = %q", got)
	}
}

func TestTargetPluginCompatibilityNoticeWarnsWithoutMutatingPluginState(t *testing.T) {
	previous := configformat.RootPath()
	t.Cleanup(func() { _ = configformat.SetRootPath(previous) })
	root := filepath.Join(t.TempDir(), "config")
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	layout := pluginpkg.DefaultLayout()
	store, err := pluginpkg.NewStore(layout, pluginpkg.RuntimeContext{OS: runtime.GOOS, Arch: runtime.GOARCH, CoreVersion: version.Version})
	if err != nil {
		t.Fatal(err)
	}
	payload := filepath.Join(t.TempDir(), "payload")
	entrypoint := filepath.Join(payload, "bin", "incompatible")
	if err := os.MkdirAll(filepath.Dir(entrypoint), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entrypoint, []byte("plugin"), 0700); err != nil {
		t.Fatal(err)
	}
	manifest := pluginpkg.Manifest{
		Schema: pluginpkg.ManifestSchema, ID: "incompatible", Name: "Incompatible", Publisher: "mewisme", Version: "1.0.0", Type: "formatter",
		Requires: pluginpkg.Requirements{ChatGPTMCP: "<=0.5.0"}, Provides: []pluginpkg.Capability{"formatter/incompatible"},
		Platforms: map[string]pluginpkg.PlatformArtifact{runtime.GOOS + "/" + runtime.GOARCH: {Artifact: "incompatible.zip", SHA256: strings.Repeat("a", 64), Archive: "zip", Entrypoint: "bin/incompatible"}},
	}
	if _, err := store.Install(manifest, payload); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate("incompatible", "1.0.0", pluginpkg.ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}); err != nil {
		t.Fatal(err)
	}
	before, err := pluginpkg.LoadLock(layout.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	notice := targetPluginCompatibilityNotice("v1.2.0")
	if !strings.Contains(notice, "incompatible") || !strings.Contains(notice, "v1.2.0") {
		t.Fatalf("compatibility notice = %q", notice)
	}
	after, err := pluginpkg.LoadLock(layout.LockPath())
	if err != nil {
		t.Fatal(err)
	}
	if before.Plugins["incompatible"] != after.Plugins["incompatible"] {
		t.Fatalf("compatibility preflight mutated plugin state: before=%#v after=%#v", before.Plugins["incompatible"], after.Plugins["incompatible"])
	}
}
