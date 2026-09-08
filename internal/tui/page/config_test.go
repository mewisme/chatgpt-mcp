package page

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
)

func TestConfigPageLoadsAndNeverRendersSecrets(t *testing.T) {
	root := prepareConfigPageRoot(t)
	page, err := NewConfig(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	cmd := page.Init()
	if cmd == nil {
		t.Fatal("config page init returned no load command")
	}
	updated, _ := page.Update(cmd())
	page = updated.(*ConfigPage)
	if !page.loaded || page.overview.Root != root || page.overview.Source.Format != configformat.JSON {
		t.Fatalf("overview=%#v loaded=%t", page.overview, page.loaded)
	}
	page.overview.Config.Auth.MCPTokenHash = "MCP_HASH_SECRET"
	page.overview.Config.Auth.AdminTokenHash = "ADMIN_HASH_SECRET"
	page.overview.Config.Tunnel.APIKey = "TUNNEL_RUNTIME_SECRET"
	page.overview.Config.Tunnel.AdminKey = "TUNNEL_ADMIN_SECRET"
	page.rebuildBrowser("")
	view := page.View(100, 32)
	rows := page.configRows()
	var all strings.Builder
	for _, row := range rows {
		all.WriteString(row.ID + " " + row.Title + " " + row.Description + " " + row.Meta + "\n")
	}
	model := all.String()
	for _, secret := range []string{"MCP_HASH_SECRET", "ADMIN_HASH_SECRET", "TUNNEL_RUNTIME_SECRET", "TUNNEL_ADMIN_SECRET"} {
		if strings.Contains(view, secret) || strings.Contains(model, secret) {
			t.Fatalf("config page leaked %s", secret)
		}
	}
	for _, want := range []string{"Runtime & Network", "Access & Security", "Shell & Execution", "Features", "Tunnel", "Storage & Maintenance", "configured"} {
		if !strings.Contains(model, want) {
			t.Fatalf("config rows missing %q: %q", want, model)
		}
	}
}

func TestConfigDashboardContainsExactlySixDomains(t *testing.T) {
	prepareConfigPageRoot(t)
	page, _ := NewConfig(t.Context())
	updated, _ := page.Update(page.Init()())
	page = updated.(*ConfigPage)
	rows := page.configRows()
	if len(rows) != 6 {
		t.Fatalf("domains=%d rows=%#v", len(rows), rows)
	}
	for _, row := range rows {
		if strings.Contains(row.ID, ".") {
			t.Fatalf("flat config field leaked into dashboard: %#v", row)
		}
	}
}

func TestConfigPageTitleStartsAtWorkspaceTitlePosition(t *testing.T) {
	prepareConfigPageRoot(t)
	page, _ := NewConfig(t.Context())
	updated, _ := page.Update(page.Init()())
	page = updated.(*ConfigPage)
	lines := strings.Split(ansi.Strip(page.View(100, 32)), "\n")
	if len(lines) < 2 || !strings.Contains(lines[0], "Configuration") || strings.TrimSpace(lines[1]) != "" {
		t.Fatalf("config title lines=%q", lines[:min(2, len(lines))])
	}
}

func TestConfigResourceUsesFullChildDetailPage(t *testing.T) {
	prepareConfigPageRoot(t)
	page, err := NewConfigRoute(t.Context(), "server.port")
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := page.Update(page.Init()())
	page = updated.(*ConfigPage)
	if page.OverlayActive() {
		t.Fatal("config detail incorrectly reports overlay active")
	}
	view := ansi.Strip(page.View(100, 28))
	for _, want := range []string{"MCP HTTP port", "server.port", "Value", "Default", "State", "default", "sets the TCP port for the MCP HTTP server", "e edit", "r refresh"} {
		if !strings.Contains(view, want) {
			t.Fatalf("config detail missing %q: %q", want, view)
		}
	}
	if strings.Contains(view, "╭") {
		t.Fatalf("config detail retained modal chrome: %q", view)
	}
	_, cmd := page.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	if cmd == nil {
		t.Fatal("config edit detail action returned no command")
	}
	message, ok := cmd().(ConfigCommandMsg)
	if !ok || message.Command != ConfigEdit || message.ResourceID != "server.port" {
		t.Fatalf("config edit action=%#v", message)
	}
}

func TestConfigReadOnlyResourceHidesEditAction(t *testing.T) {
	prepareConfigPageRoot(t)
	page, err := NewConfigRoute(t.Context(), "auth.mcp_token_hash")
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := page.Update(page.Init()())
	page = updated.(*ConfigPage)
	view := ansi.Strip(page.View(100, 28))
	if !strings.Contains(view, "MCP credential") || !strings.Contains(view, "managed") || !strings.Contains(view, "r refresh") || strings.Contains(view, "e edit") {
		t.Fatalf("read-only config detail=%q", view)
	}
}

func TestConfigFieldDetailNeverRendersSecretOrHash(t *testing.T) {
	prepareConfigPageRoot(t)
	page, _ := NewConfigRoute(t.Context(), "auth.mcp_token_hash")
	updated, _ := page.Update(page.Init()())
	page = updated.(*ConfigPage)
	page.overview.Config.Auth.MCPTokenHash = "FIELD_DETAIL_SECRET"
	page.syncDetail()
	view := ansi.Strip(page.View(100, 30))
	if strings.Contains(view, "FIELD_DETAIL_SECRET") {
		t.Fatalf("field detail leaked secret: %q", view)
	}
	for _, want := range []string{"configured", "managed", "Guidance", "auth token workflow"} {
		if !strings.Contains(view, want) {
			t.Fatalf("field detail missing %q: %q", want, view)
		}
	}
}

func TestConfigBrowserOpenNavigatesToFieldChild(t *testing.T) {
	prepareConfigPageRoot(t)
	page, _ := NewConfig(t.Context())
	updated, _ := page.Update(page.Init()())
	page = updated.(*ConfigPage)
	_, cmd := page.Update(component.BrowserOpenMsg{Row: component.Row{ID: "runtime"}})
	if cmd == nil {
		t.Fatal("config browser open returned no navigation command")
	}
	message, ok := cmd().(NavigateMsg)
	if !ok || strings.Join(message.Path, "/") != "config/runtime" {
		t.Fatalf("config navigation=%#v", message)
	}
}

func TestConfigDomainRowsAreSectionScopedAndShowState(t *testing.T) {
	prepareConfigPageRoot(t)
	page, err := NewConfigRoute(t.Context(), "shell")
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := page.Update(page.Init()())
	page = updated.(*ConfigPage)
	page.overview.Config.Shell.ApprovalPolicy = "strict"
	page.rebuildBrowser("")
	rows := page.configRows()
	if len(rows) == 0 {
		t.Fatal("shell domain has no rows")
	}
	foundApproval := false
	for _, row := range rows {
		spec, ok := config.FieldByKey(row.ID)
		if !ok || spec.Section != config.FieldSectionShell {
			t.Fatalf("non-shell field in shell domain: %#v", row)
		}
		if row.ID == "shell.approval_policy" {
			foundApproval = strings.Contains(row.Meta, "strict") && strings.Contains(row.Meta, "custom")
		}
	}
	if !foundApproval {
		t.Fatalf("approval row missing custom state: %#v", rows)
	}
	view := ansi.Strip(page.View(100, 30))
	if !strings.Contains(view, "Configuration / Shell & Execution") || !strings.Contains(view, "e edit") || !strings.Contains(view, "/ filter") {
		t.Fatalf("shell domain view=%q", view)
	}
}

func TestConfigAccessDomainNeverRendersCredentialSecrets(t *testing.T) {
	prepareConfigPageRoot(t)
	page, _ := NewConfigRoute(t.Context(), "access")
	updated, _ := page.Update(page.Init()())
	page = updated.(*ConfigPage)
	page.overview.Config.Auth.MCPTokenHash = "DOMAIN_MCP_SECRET"
	page.overview.Config.Auth.AdminTokenHash = "DOMAIN_ADMIN_SECRET"
	page.rebuildBrowser("")
	view := ansi.Strip(page.View(100, 30))
	if strings.Contains(view, "DOMAIN_MCP_SECRET") || strings.Contains(view, "DOMAIN_ADMIN_SECRET") {
		t.Fatalf("access domain leaked credential: %q", view)
	}
	if !strings.Contains(view, "configured") || !strings.Contains(view, "managed") {
		t.Fatalf("access domain missing managed credential state: %q", view)
	}
}

func TestConfigDomainOpenNavigatesToLegacyCompatibleFieldRoute(t *testing.T) {
	prepareConfigPageRoot(t)
	page, _ := NewConfigRoute(t.Context(), "runtime")
	updated, _ := page.Update(page.Init()())
	page = updated.(*ConfigPage)
	_, cmd := page.Update(component.BrowserOpenMsg{Row: component.Row{ID: "server.port"}})
	if cmd == nil {
		t.Fatal("domain field open returned no navigation")
	}
	message, ok := cmd().(NavigateMsg)
	if !ok || strings.Join(message.Path, "/") != "config/server.port" {
		t.Fatalf("field navigation=%#v", message)
	}
}

func TestConfigStoragePageCentralizesMaintenanceActions(t *testing.T) {
	prepareConfigPageRoot(t)
	page, _ := NewConfigRoute(t.Context(), "storage")
	updated, _ := page.Update(page.Init()())
	page = updated.(*ConfigPage)
	rows := page.configRows()
	if len(rows) != 6 {
		t.Fatalf("maintenance actions=%d rows=%#v", len(rows), rows)
	}
	wantIDs := []string{"verify", "reload", "migrate", "convert", "export", "import"}
	for index, want := range wantIDs {
		if rows[index].ID != want {
			t.Fatalf("maintenance row %d=%q want=%q", index, rows[index].ID, want)
		}
	}
	view := ansi.Strip(page.View(100, 34))
	for _, want := range []string{"Configuration / Storage & Maintenance", "Format", "Config", "Root", "Initialized", "Runtime sync", "Persisted", "Runtime", "enter run"} {
		if !strings.Contains(view, want) {
			t.Fatalf("storage view missing %q: %q", want, view)
		}
	}
}

func TestConfigRuntimeSyncStatusRendersPendingAndFingerprints(t *testing.T) {
	prepareConfigPageRoot(t)
	page, _ := NewConfigRoute(t.Context(), "storage")
	updated, _ := page.Update(page.Init()())
	page = updated.(*ConfigPage)
	page.overview.RuntimeRunning = true
	page.overview.RuntimeSync = application.ConfigRuntimeSync{State: application.ConfigRuntimePending, PersistedFingerprint: "1234567890abcdef", RuntimeFingerprint: "fedcba0987654321"}
	view := ansi.Strip(page.View(100, 34))
	for _, want := range []string{"changes pending", "1234567890ab", "fedcba098765"} {
		if !strings.Contains(view, want) {
			t.Fatalf("runtime sync view missing %q: %q", want, view)
		}
	}
}

func TestConfigSaveMarksRunningRuntimePendingImmediately(t *testing.T) {
	prepareConfigPageRoot(t)
	page, _ := NewConfig(t.Context())
	updated, _ := page.Update(page.Init()())
	page = updated.(*ConfigPage)
	page.overview.RuntimeRunning = true
	page.overview.RuntimeSync.State = application.ConfigRuntimeCurrent
	mutation := page.overview.Config
	mutation.Server.Port++
	page.finishOperation(configOperationMsg{command: ConfigEdit, mutation: application.ConfigMutationResult{Config: mutation}})
	if page.overview.RuntimeSync.State != application.ConfigRuntimePending {
		t.Fatalf("sync state=%q", page.overview.RuntimeSync.State)
	}
	if page.overview.RuntimeSync.PersistedFingerprint == "" {
		t.Fatal("persisted fingerprint not updated")
	}
}

func TestConfigStorageOpenRunsMaintenanceAction(t *testing.T) {
	prepareConfigPageRoot(t)
	page, _ := NewConfigRoute(t.Context(), "storage")
	updated, _ := page.Update(page.Init()())
	page = updated.(*ConfigPage)
	_, cmd := page.Update(component.BrowserOpenMsg{Row: component.Row{ID: "convert"}})
	if cmd == nil || page.overlay != configOverlayForm || page.command != ConfigConvert {
		t.Fatalf("convert cmd=%v overlay=%d command=%q", cmd != nil, page.overlay, page.command)
	}
}

func TestConfigRootDoesNotExposeMaintenanceShortcuts(t *testing.T) {
	prepareConfigPageRoot(t)
	page, _ := NewConfig(t.Context())
	updated, _ := page.Update(page.Init()())
	page = updated.(*ConfigPage)
	for _, key := range []string{"v", "m", "c", "x", "i"} {
		_, handled := page.handleKey(tea.KeyPressMsg{Code: rune(key[0]), Text: key})
		if handled {
			t.Fatalf("root still handles maintenance shortcut %q", key)
		}
	}
}

func TestConfigGlobalSearchIndexesAllFieldsWithoutSecrets(t *testing.T) {
	prepareConfigPageRoot(t)
	page, _ := NewConfig(t.Context())
	updated, _ := page.Update(page.Init()())
	page = updated.(*ConfigPage)
	page.overview.Config.Tunnel.APIKey = "SEARCH_RUNTIME_SECRET"
	page.overview.Config.Tunnel.AdminKey = "SEARCH_ADMIN_SECRET"
	rows := page.searchRows()
	if len(rows) != len(config.Fields()) {
		t.Fatalf("search rows=%d fields=%d", len(rows), len(config.Fields()))
	}
	joined := fmt.Sprintf("%#v", rows)
	if strings.Contains(joined, "SEARCH_RUNTIME_SECRET") || strings.Contains(joined, "SEARCH_ADMIN_SECRET") {
		t.Fatalf("search index leaked secret: %s", joined)
	}
	found := false
	for _, row := range rows {
		if row.ID == "shell.approval_policy" {
			found = strings.Contains(row.Search, "Approval policy") && strings.Contains(row.Search, "shell.approval_policy") && strings.Contains(row.Description, "Shell & Execution")
		}
	}
	if !found {
		t.Fatal("search index missing approval policy metadata")
	}
}

func TestConfigSearchOpensAndNavigatesToField(t *testing.T) {
	prepareConfigPageRoot(t)
	page, _ := NewConfig(t.Context())
	updated, _ := page.Update(page.Init()())
	page = updated.(*ConfigPage)
	_, handled := page.handleKey(tea.KeyPressMsg{Code: 's'})
	if !handled || !page.searching || !page.browser.InputActive() {
		t.Fatalf("search handled=%t searching=%t input=%t", handled, page.searching, page.browser.InputActive())
	}
	_, cmd := page.Update(component.BrowserOpenMsg{Row: component.Row{ID: "shell.approval_policy"}})
	if cmd == nil {
		t.Fatal("search result returned no navigation")
	}
	message, ok := cmd().(NavigateMsg)
	if !ok || strings.Join(message.Path, "/") != "config/shell.approval_policy" {
		t.Fatalf("search navigation=%#v", message)
	}
}

func TestConfigPageReadOnlyGuidanceAndStoppedReload(t *testing.T) {
	prepareConfigPageRoot(t)
	page, _ := NewConfig(t.Context())
	updated, _ := page.Update(page.Init()())
	page = updated.(*ConfigPage)
	if cmd, err := page.openCommand(ConfigEdit, "auth.mcp_token_hash"); err != nil || cmd != nil {
		t.Fatalf("read-only edit cmd=%v err=%v", cmd != nil, err)
	}
	if page.overlay != configOverlayNone || !strings.Contains(page.notice, "auth token") {
		t.Fatalf("overlay=%d notice=%q", page.overlay, page.notice)
	}
	page.overview.RuntimeRunning = false
	if _, err := page.openCommand(ConfigReload, ""); err == nil || !strings.Contains(err.Error(), "runtime is not running") {
		t.Fatalf("reload err=%v", err)
	}
}

func TestConfigPageEditUsesDomainValidationAndPersists(t *testing.T) {
	prepareConfigPageRoot(t)
	page, _ := NewConfig(t.Context())
	updated, _ := page.Update(page.Init()())
	page = updated.(*ConfigPage)
	if _, err := page.openCommand(ConfigEdit, "server.port"); err != nil {
		t.Fatal(err)
	}
	page.fieldForm.Raw = "70000"
	result := page.submitForm()
	message := result().(configOperationMsg)
	want := page.overview.Config
	wantErr := config.SetValueValidated(&want, "server.port", "70000")
	if message.err == nil || wantErr == nil || message.err.Error() != wantErr.Error() {
		t.Fatalf("page err=%v want=%v", message.err, wantErr)
	}
	page.finishOperation(message)
	if page.err == nil || page.overlay != configOverlayNone {
		t.Fatalf("err=%v overlay=%d", page.err, page.overlay)
	}
	page.err = nil
	if _, err := page.openCommand(ConfigEdit, "server.port"); err != nil {
		t.Fatal(err)
	}
	page.fieldForm.Raw = "40124"
	message = page.submitForm()().(configOperationMsg)
	load := page.finishOperation(message)
	if message.err != nil || load == nil {
		t.Fatalf("edit err=%v load=%v", message.err, load != nil)
	}
	updated, _ = page.Update(load())
	page = updated.(*ConfigPage)
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Server.Port != 40124 || !strings.Contains(page.notice, "next runtime start") {
		t.Fatalf("port=%d notice=%q", loaded.Server.Port, page.notice)
	}
}

func TestConfigPageCancellationIgnoresLateResult(t *testing.T) {
	prepareConfigPageRoot(t)
	page, _ := NewConfig(t.Context())
	updated, _ := page.Update(page.Init()())
	page = updated.(*ConfigPage)
	cmd := page.startOperation(ConfigVerify, "Verifying", func(context.Context) configOperationMsg {
		return configOperationMsg{command: ConfigVerify, verify: config.VerifyResult{Format: configformat.JSON, Files: 99}}
	})
	updated, _ = page.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	page = updated.(*ConfigPage)
	if page.overlay != configOverlayNone {
		t.Fatalf("overlay=%d", page.overlay)
	}
	updated, follow := page.Update(cmd())
	page = updated.(*ConfigPage)
	if follow != nil || !strings.Contains(page.notice, "cancellation requested") || strings.Contains(page.notice, "99") {
		t.Fatalf("follow=%v notice=%q", follow != nil, page.notice)
	}
}

func TestConfigPageOldOperationCannotOverwriteNewOperation(t *testing.T) {
	prepareConfigPageRoot(t)
	page, _ := NewConfig(t.Context())
	updated, _ := page.Update(page.Init()())
	page = updated.(*ConfigPage)
	old := page.startOperation(ConfigVerify, "Old", func(context.Context) configOperationMsg {
		return configOperationMsg{command: ConfigVerify, verify: config.VerifyResult{Format: configformat.JSON, Files: 1}}
	})
	page.cancelOperation()
	current := page.startOperation(ConfigVerify, "Current", func(context.Context) configOperationMsg {
		return configOperationMsg{command: ConfigVerify, verify: config.VerifyResult{Format: configformat.JSON, Files: 2}}
	})
	updated, staleFollow := page.Update(old())
	page = updated.(*ConfigPage)
	if staleFollow != nil || page.overlay != configOverlayOperation {
		t.Fatalf("stale follow=%v overlay=%d", staleFollow != nil, page.overlay)
	}
	updated, follow := page.Update(current())
	page = updated.(*ConfigPage)
	if follow == nil || !strings.Contains(page.notice, "2 structured files") {
		t.Fatalf("follow=%v notice=%q", follow != nil, page.notice)
	}
}

func TestConfigPageFormsExposeSafetyConfirmations(t *testing.T) {
	convert, convertData := newConfigConvertForm(configformat.JSON)
	if convert.Init() == nil || convertData.Confirm {
		t.Fatalf("convert form init=%v confirm=%t", convert.Init() != nil, convertData.Confirm)
	}
	_, importData := newConfigBundleForm(false)
	if importData.Force || importData.Confirm || importData.Path != "chatgpt-mcp-config.cgm" {
		t.Fatalf("import defaults=%#v", importData)
	}
	_, exportData := newConfigBundleForm(true)
	if exportData.Force || exportData.Path != "chatgpt-mcp-config.cgm" {
		t.Fatalf("export defaults=%#v", exportData)
	}
}

func prepareConfigPageRoot(t *testing.T) string {
	t.Helper()
	previous := configformat.RootPath()
	t.Cleanup(func() { _ = configformat.SetRootPath(previous) })
	root := filepath.Join(t.TempDir(), "config")
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Auth.MCPTokenHash = "mcp-hash"
	cfg.Auth.AdminTokenHash = "admin-hash"
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	overview, err := application.LoadConfigOverview(context.Background())
	if err != nil || overview.RuntimeRunning {
		t.Fatalf("overview=%#v err=%v", overview, err)
	}
	return root
}
