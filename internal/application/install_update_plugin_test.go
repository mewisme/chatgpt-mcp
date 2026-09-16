package application

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/install"
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

func TestRollbackRequiredCoreFailureSkipsAlreadyInstalled(t *testing.T) {
	report := pluginpkg.CoreReconcileReport{Items: []pluginpkg.CoreReconcileItem{{ID: "admin-ui", Action: pluginpkg.CoreActionFailed, Required: true, Error: "missing artifact"}}}
	err := RollbackRequiredCoreFailure(context.Background(), install.Result{AlreadyInstalled: true}, report)
	if err == nil || !strings.Contains(err.Error(), "required core plugin admin-ui failed") || strings.Contains(err.Error(), "previous version restored") {
		t.Fatalf("already-installed required failure = %v", err)
	}
}

func TestFormatCorePluginNoticeListsActions(t *testing.T) {
	got := formatCorePluginNotice(pluginpkg.CoreReconcileReport{Items: []pluginpkg.CoreReconcileItem{
		{ID: "admin-ui", Version: "1.0.0", Action: pluginpkg.CoreActionInstalled},
	}})
	if got != "core plugins: admin-ui installed@1.0.0" {
		t.Fatalf("notice = %q", got)
	}
}
