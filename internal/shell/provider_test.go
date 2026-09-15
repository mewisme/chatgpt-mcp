package shell

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
)

func TestProviderResolverPriorityConfiguredPluginSystem(t *testing.T) {
	store, pluginPath := testWindowsBashProviderStore(t)
	systemPath := filepath.Join(t.TempDir(), "system", "bash.exe")
	resolver := NewProviderResolver(store)
	resolver.goos = "windows"
	resolver.lookPath = func(name string) (string, error) {
		if name != "bash" {
			t.Fatalf("lookPath(%q)", name)
		}
		return systemPath, nil
	}

	configured := filepath.Join(t.TempDir(), "bash.exe")
	if err := os.WriteFile(configured, []byte("configured"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := resolver.SetConfiguredExecutable(configured); err != nil {
		t.Fatal(err)
	}
	provider, err := resolver.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if provider.Source != "configured" || filepath.Clean(provider.Executable) != filepath.Clean(configured) || provider.Language != "bash" {
		t.Fatalf("configured provider = %#v", provider)
	}

	if err := resolver.SetConfiguredExecutable(""); err != nil {
		t.Fatal(err)
	}
	provider, err = resolver.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if provider.Source != "plugin" || provider.PluginID != "bash" || provider.Version != "1.0.0" || filepath.Clean(provider.Executable) != filepath.Clean(pluginPath) {
		t.Fatalf("plugin provider = %#v", provider)
	}

	if err := store.SetEnabled("bash", false); err != nil {
		t.Fatal(err)
	}
	provider, err = resolver.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	if provider.Source != "system" || filepath.Clean(provider.Executable) != filepath.Clean(systemPath) {
		t.Fatalf("system provider = %#v", provider)
	}
}

func TestProviderResolverWindowsNeverFallsBackToPowerShell(t *testing.T) {
	root := t.TempDir()
	layout := pluginpkg.Layout{ConfigRoot: filepath.Join(root, "config"), DataRoot: filepath.Join(root, "data"), CacheRoot: filepath.Join(root, "cache")}
	store, err := pluginpkg.NewStore(layout, pluginpkg.RuntimeContext{OS: "windows", Arch: "amd64", CoreVersion: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	resolver := NewProviderResolver(store)
	resolver.goos = "windows"
	lookups := []string{}
	resolver.lookPath = func(name string) (string, error) {
		lookups = append(lookups, name)
		return "", exec.ErrNotFound
	}
	_, err = resolver.Resolve()
	if !errors.Is(err, ErrBashUnavailable) || !strings.Contains(err.Error(), "cgm plugin install bash") {
		t.Fatalf("missing Bash error = %v", err)
	}
	if len(lookups) != 1 || lookups[0] != "bash" {
		t.Fatalf("shell fallback lookups = %#v", lookups)
	}
}

func testWindowsBashProviderStore(t *testing.T) (*pluginpkg.Store, string) {
	t.Helper()
	root := t.TempDir()
	layout := pluginpkg.Layout{ConfigRoot: filepath.Join(root, "config"), DataRoot: filepath.Join(root, "data"), CacheRoot: filepath.Join(root, "cache")}
	store, err := pluginpkg.NewStore(layout, pluginpkg.RuntimeContext{OS: "windows", Arch: "amd64", CoreVersion: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	payload := filepath.Join(root, "payload")
	entrypoint := filepath.Join(payload, "usr", "bin", "bash.exe")
	if err := os.MkdirAll(filepath.Dir(entrypoint), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entrypoint, []byte("bash"), 0755); err != nil {
		t.Fatal(err)
	}
	manifest := pluginpkg.Manifest{
		Schema: pluginpkg.ManifestSchema, ID: "bash", Name: "Bash Runtime", Publisher: "mewisme", Version: "1.0.0", Type: "runtime",
		Provides: []pluginpkg.Capability{bashCapability}, Permissions: []pluginpkg.Permission{pluginpkg.PermissionProcessExecute},
		Platforms: map[string]pluginpkg.PlatformArtifact{"windows/amd64": {Artifact: "bash.zip", SHA256: strings.Repeat("a", 64), Archive: "zip", Entrypoint: "usr/bin/bash.exe"}},
	}
	installed, err := store.Install(manifest, payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Activate("bash", "1.0.0", pluginpkg.ActivationTrust{Registry: "official", Publisher: "mewisme", Trusted: true}); err != nil {
		t.Fatal(err)
	}
	return store, installed.Entrypoint
}
