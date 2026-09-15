package page

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"go.mewis.me/chatgpt-mcp/internal/application"
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
)

func TestPluginPageInstalledListDetailAndConfirmation(t *testing.T) {
	service := testPluginService(t)
	page, err := newPluginsRouteAction(t.Context(), "", "", "", service)
	if err != nil {
		t.Fatal(err)
	}
	message, ok := page.loadCmd()().(pluginLoadMsg)
	if !ok {
		t.Fatalf("load message type = %T", page.loadCmd()())
	}
	page.finishLoad(message)
	if !page.loaded || page.err != nil || !page.browser.SelectID("demo") {
		t.Fatalf("installed page loaded=%t err=%v", page.loaded, page.err)
	}
	selected, ok := page.browser.Selected()
	if !ok || selected.ID != "demo" || !strings.Contains(selected.Meta, "enabled") || !strings.Contains(selected.Meta, "1.0.0") {
		t.Fatalf("installed row = %#v ok=%t", selected, ok)
	}
	if _, err := page.openCommand(PluginUninstall, "demo"); err != nil {
		t.Fatal(err)
	}
	if page.overlay != pluginOverlayConfirm || page.command != PluginUninstall || page.confirmTitle() != "Uninstall plugin?" {
		t.Fatalf("uninstall confirmation overlay=%d command=%q title=%q", page.overlay, page.command, page.confirmTitle())
	}
	page.closeOverlay()

	detail, err := newPluginsRouteAction(t.Context(), "demo", "", "", service)
	if err != nil {
		t.Fatal(err)
	}
	detailMessage := detail.loadCmd()().(pluginLoadMsg)
	detail.finishLoad(detailMessage)
	view := ansi.Strip(detail.View(100, 28))
	for _, want := range []string{"formatter/demo", "process/execute", "enabled", "official", "mewisme", "verified installed state", "github.com/mewisme/chatgpt-mcp", "mewisme/chatgpt-mcp", "Core compatibility", "space toggle", "u update", "? more"} {
		if !strings.Contains(view, want) {
			t.Fatalf("plugin detail missing %q: %q", want, view)
		}
	}
	for key, want := range map[rune]PluginCommand{'b': PluginRollback, 'p': PluginPrune, 'v': PluginVerify, 'd': PluginUninstall, 'D': PluginForceUninstall} {
		_, cmd := detail.detail.Update(tea.KeyPressMsg{Code: key, Text: string(key)})
		if cmd == nil {
			t.Fatalf("detail key %q returned no command", key)
		}
		message, ok := cmd().(PluginCommandMsg)
		if !ok || message.Command != want || message.TargetID != "demo" {
			t.Fatalf("detail key %q message=%#v", key, message)
		}
	}
}

func TestPluginPageRegistryEditorAndRemoveConfirmation(t *testing.T) {
	service := testPluginService(t)
	page, err := newPluginsRouteAction(t.Context(), "", "registries", "add", service)
	if err != nil {
		t.Fatal(err)
	}
	if page.editor == nil || page.registryForm == nil || page.registryForm.Issuer != pluginpkg.OfficialSigstoreIssuer || page.Dirty() {
		t.Fatalf("registry editor page=%#v form=%#v", page.editor, page.registryForm)
	}

	list, err := newPluginsRouteAction(t.Context(), "", "registries", "", service)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.AddRegistry(context.Background(), "community", "https://plugins.example.test", false, pluginpkg.SigstoreIdentity{Issuer: pluginpkg.OfficialSigstoreIssuer, Repository: "example/plugins"}); err != nil {
		t.Fatal(err)
	}
	list.finishLoad(list.loadCmd()().(pluginLoadMsg))
	if !list.browser.SelectID("community") {
		t.Fatal("custom registry row missing")
	}
	if _, err := list.openCommand(PluginRegistryRemove, "community"); err != nil {
		t.Fatal(err)
	}
	if list.overlay != pluginOverlayConfirm || list.confirmTitle() != "Remove plugin registry?" {
		t.Fatalf("registry confirmation overlay=%d title=%q", list.overlay, list.confirmTitle())
	}
}

func TestPluginPageShowsLoadingState(t *testing.T) {
	page, err := newPluginsRouteAction(t.Context(), "", "", "", testPluginService(t))
	if err != nil {
		t.Fatal(err)
	}
	page.loading = true
	view := ansi.Strip(page.View(80, 20))
	if !strings.Contains(view, "Loading plugins") {
		t.Fatalf("loading view = %q", view)
	}
}

func TestPluginPageHostInstallChooser(t *testing.T) {
	page, err := newPluginsRouteAction(t.Context(), "", "marketplace", "", testPluginService(t))
	if err != nil {
		t.Fatal(err)
	}
	prerequisite := &pluginpkg.HostPrerequisiteError{
		Executable: "rtk", Reason: "required host executable \"rtk\" is not installed or not on PATH", Portable: &pluginpkg.HostPortableInstall{},
		Install: []pluginpkg.HostInstallHint{
			{Label: "Package manager", Command: "winget install rtk-ai.rtk", Executable: "winget", Args: []string{"install", "rtk-ai.rtk"}},
			{Label: "Install script", Command: "curl https://example.test/install.sh | sh"},
		},
	}
	if !page.openHostInstall(prerequisite) {
		t.Fatal("host install chooser did not open")
	}
	if page.overlay != pluginOverlayHostInstall || page.hostIndex != 0 || len(page.hostOptions) != 2 || !page.hostOptions[0].portable || page.hostOptions[1].portable {
		t.Fatalf("host chooser state overlay=%d index=%d options=%#v", page.overlay, page.hostIndex, page.hostOptions)
	}
	body := ansi.Strip(page.hostInstallBody())
	for _, want := range []string{"Host dependency required", "Install verified portable binary locally", "Install globally: winget install rtk-ai.rtk", "Manual install commands:", "curl https://example.test/install.sh | sh"} {
		if !strings.Contains(body, want) {
			t.Fatalf("host chooser missing %q: %q", want, body)
		}
	}
	page.updateHostInstall(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if page.hostIndex != 1 {
		t.Fatalf("host chooser down index = %d", page.hostIndex)
	}
	page.updateHostInstall(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if page.hostIndex != 0 {
		t.Fatalf("host chooser wrap index = %d", page.hostIndex)
	}
	page.updateHostInstall(tea.KeyPressMsg{Code: tea.KeyEsc})
	if page.overlay != pluginOverlayNone || page.hostPrerequisite != nil || len(page.hostOptions) != 0 {
		t.Fatalf("host chooser did not close cleanly: overlay=%d prerequisite=%#v options=%#v", page.overlay, page.hostPrerequisite, page.hostOptions)
	}
}

func testPluginService(t *testing.T) *application.PluginService {
	t.Helper()
	root := t.TempDir()
	layout := pluginpkg.Layout{ConfigRoot: filepath.Join(root, "config"), DataRoot: filepath.Join(root, "data"), CacheRoot: filepath.Join(root, "cache")}
	store, err := pluginpkg.NewStore(layout, pluginpkg.RuntimeContext{OS: runtime.GOOS, Arch: runtime.GOARCH, CoreVersion: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	payload := filepath.Join(root, "payload")
	if err := os.MkdirAll(filepath.Join(payload, "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	entrypoint := filepath.Join(payload, "bin", "demo")
	if runtime.GOOS == "windows" {
		entrypoint += ".exe"
	}
	if err := os.WriteFile(entrypoint, []byte("fixture"), 0700); err != nil {
		t.Fatal(err)
	}
	entrypointRelative, err := filepath.Rel(payload, entrypoint)
	if err != nil {
		t.Fatal(err)
	}
	manifest := pluginpkg.Manifest{
		Schema: pluginpkg.ManifestSchema, ID: "demo", Name: "Demo formatter", Publisher: "mewisme", Version: "1.0.0", Type: "formatter",
		Provides: []pluginpkg.Capability{"formatter/demo"}, Permissions: []pluginpkg.Permission{pluginpkg.PermissionProcessExecute},
		Platforms: map[string]pluginpkg.PlatformArtifact{runtime.GOOS + "/" + runtime.GOARCH: {Artifact: "demo.tar.gz", SHA256: strings.Repeat("a", 64), Archive: "tar.gz", Entrypoint: filepath.ToSlash(entrypointRelative)}},
	}
	if _, err := store.Install(manifest, payload); err != nil {
		t.Fatal(err)
	}
	if err := store.Activate("demo", "1.0.0", pluginpkg.ActivationTrust{Registry: pluginpkg.OfficialRegistryName, Publisher: "mewisme", Trusted: true}); err != nil {
		t.Fatal(err)
	}
	return &application.PluginService{Manager: &pluginpkg.Manager{Store: store, RegistryClient: pluginpkg.RegistryClient{Layout: layout}}, Layout: layout}
}
