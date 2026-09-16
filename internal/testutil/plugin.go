package testutil

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
)

func TunnelProviderSettingsSchema() pluginpkg.SettingsSchema {
	return pluginpkg.SettingsSchema{Fields: []pluginpkg.SettingField{
		{Key: "mcp", Kind: pluginpkg.FieldBool, Title: "Expose MCP HTTP", Default: false},
		{Key: "admin", Kind: pluginpkg.FieldBool, Title: "Expose Admin HTTP", Default: false},
	}}
}

func InstallStubTunnelPlugin(t *testing.T, enabled bool) {
	t.Helper()
	layout := pluginpkg.DefaultLayout()
	store, err := pluginpkg.NewStore(layout, pluginpkg.RuntimeContext{})
	if err != nil {
		t.Fatal(err)
	}
	payload := t.TempDir()
	if err := os.MkdirAll(filepath.Join(payload, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(payload, "bin", "cf-tunnel"), []byte("payload"), 0700); err != nil {
		t.Fatal(err)
	}
	manifest := pluginpkg.Manifest{
		Schema: pluginpkg.ManifestSchemaV2, ID: "cf-tunnel", Name: "CF Tunnel", Publisher: "mewisme", License: "Apache-2.0", Version: "1.0.0", Type: "runtime",
		Provides: []pluginpkg.Capability{"tunnel/cf"}, Permissions: []pluginpkg.Permission{pluginpkg.PermissionProcessExecute, pluginpkg.PermissionNetworkOutbound},
		Scopes: []pluginpkg.PluginScope{pluginpkg.ScopeGlobal}, Config: TunnelProviderSettingsSchema(),
		Platforms: map[string]pluginpkg.PlatformArtifact{
			runtime.GOOS + "/" + runtime.GOARCH: {Artifact: "cf-tunnel-1.0.0.zip", SHA256: strings.Repeat("a", 64), Archive: "zip", Entrypoint: "bin/cf-tunnel"},
		},
	}
	if _, err := store.Install(manifest, payload); err != nil && !errors.Is(err, pluginpkg.ErrVersionInstalled) {
		t.Fatal(err)
	}
	if err := store.ActivateWithState("cf-tunnel", "1.0.0", pluginpkg.ActivationTrust{Registry: pluginpkg.OfficialRegistryName, Publisher: "mewisme", Trusted: true}, enabled); err != nil {
		t.Fatal(err)
	}
}
