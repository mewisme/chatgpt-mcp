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
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	pluginpkg "go.mewis.me/chatgpt-mcp/internal/plugin"
	ponytailplugin "go.mewis.me/chatgpt-mcp/plugins/ponytail"
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
	if !ok || selected.ID != "demo" || !strings.Contains(selected.Meta, "enabled") || !strings.Contains(selected.Meta, "1.0.0") || !strings.Contains(selected.Meta, "global") {
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
	for _, want := range []string{"formatter/demo", "process/execute", "enabled", "official", "mewisme", "verified installed state", "github.com/mewisme/chatgpt-mcp", "mewisme/chatgpt-mcp", "Core compatibility", "Scope", "global", "space toggle", "u update", "? more"} {
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

func TestPluginPageBuiltinHidesArtifactActions(t *testing.T) {
	service := testPluginService(t)
	page, err := newPluginsRouteAction(t.Context(), "", "", "", service)
	if err != nil {
		t.Fatal(err)
	}
	page.finishLoad(page.loadCmd()().(pluginLoadMsg))
	if !page.browser.SelectID("ponytail") {
		t.Fatal("built-in plugin missing from installed list")
	}
	selected, ok := page.browser.Selected()
	if !ok || !strings.Contains(selected.Description, "Built-in") {
		t.Fatalf("built-in row = %#v ok=%t", selected, ok)
	}
	if _, err := page.openCommand(PluginUninstall, "ponytail"); err == nil || !strings.Contains(err.Error(), "built-in") {
		t.Fatalf("uninstall builtin error = %v", err)
	}

	detail, err := newPluginsRouteAction(t.Context(), "ponytail", "", "", service)
	if err != nil {
		t.Fatal(err)
	}
	detail.finishLoad(detail.loadCmd()().(pluginLoadMsg))
	view := ansi.Strip(detail.View(100, 28))
	for _, want := range []string{"Built-in", "built-in", "ponytail"} {
		if !strings.Contains(view, want) {
			t.Fatalf("builtin detail missing %q: %q", want, view)
		}
	}
	for _, key := range []rune{'u', 'b', 'p', 'd', 'D'} {
		_, cmd := detail.detail.Update(tea.KeyPressMsg{Code: key, Text: string(key)})
		if cmd != nil {
			t.Fatalf("builtin detail key %q should be hidden, got %#v", key, cmd())
		}
	}
	_, cmd := detail.detail.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	if cmd == nil {
		t.Fatal("builtin configure action missing")
	}
	navigate, ok := cmd().(NavigateMsg)
	if !ok || strings.Join(navigate.Path, "/") != "plugins/ponytail/configure" {
		t.Fatalf("builtin configure navigation=%#v", navigate)
	}
}

func TestPluginInstallChoicesSkipWorkspaceWhenNoneRegistered(t *testing.T) {
	choices := pluginInstallChoices([]pluginpkg.PluginScope{pluginpkg.ScopeGlobal, pluginpkg.ScopeWorkspace}, nil)
	if len(choices) != 1 || choices[0].label != "Global" {
		t.Fatalf("choices = %#v", choices)
	}
}

func TestPluginRoutePathIncludesWorkspace(t *testing.T) {
	if got := strings.Join(pluginRoutePath("ws_demo", "rtk", "configure"), "/"); got != "plugins/@ws_demo/rtk/configure" {
		t.Fatalf("path = %q", got)
	}
	if got := strings.Join(pluginRoutePath("", "marketplace"), "/"); got != "plugins/marketplace" {
		t.Fatalf("global path = %q", got)
	}
}

func TestPluginPageConfigureEditorSavesSettings(t *testing.T) {
	service := testPluginService(t)
	page, err := newPluginsRouteAction(t.Context(), "ponytail", "", "configure", service)
	if err != nil {
		t.Fatal(err)
	}
	if page.editor == nil || page.configForm == nil || page.configForm.ID != "ponytail" {
		t.Fatalf("configure editor page=%#v form=%#v", page.editor, page.configForm)
	}
	if page.configForm.Bools["default_active"] == nil || !*page.configForm.Bools["default_active"] {
		t.Fatalf("default_active=%v", page.configForm.Bools["default_active"])
	}
	if page.configForm.Enums["default_mode"] == nil || *page.configForm.Enums["default_mode"] != "full" {
		t.Fatalf("default_mode=%v", page.configForm.Enums["default_mode"])
	}
	*page.configForm.Bools["default_active"] = false
	*page.configForm.Enums["default_mode"] = "lite"
	cmd := page.submitEditor()
	if cmd == nil {
		t.Fatal("configure save returned no command")
	}
	schema, values, err := service.PluginSettings("ponytail")
	if err != nil || len(schema.Fields) == 0 {
		t.Fatalf("schema=%#v err=%v", schema, err)
	}
	if values["default_active"] != false || values["default_mode"] != "lite" {
		t.Fatalf("saved values=%#v", values)
	}

	detail, err := newPluginsRouteAction(t.Context(), "ponytail", "", "", service)
	if err != nil {
		t.Fatal(err)
	}
	detail.finishLoad(detail.loadCmd()().(pluginLoadMsg))
	if _, err := detail.openCommand(PluginConfigReset, "ponytail"); err != nil {
		t.Fatal(err)
	}
	if detail.overlay != pluginOverlayConfirm || detail.confirmTitle() != "Reset plugin configuration?" {
		t.Fatalf("reset confirmation overlay=%d title=%q", detail.overlay, detail.confirmTitle())
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
	t.Setenv(configformat.EnvConfigDir, filepath.Join(root, "core"))
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
	store.Builtins = pluginpkg.BuiltinRegistry{ponytailplugin.Plugin()}
	return &application.PluginService{Manager: &pluginpkg.Manager{Store: store, RegistryClient: pluginpkg.RegistryClient{Layout: layout}}, Layout: layout}
}
