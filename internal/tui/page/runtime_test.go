package page

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/install"
	"go.mewis.me/chatgpt-mcp/internal/runtimecontrol"
	managed "go.mewis.me/chatgpt-mcp/internal/service"
	updatepkg "go.mewis.me/chatgpt-mcp/internal/update"
)

func TestRuntimePageBuildsSystemRows(t *testing.T) {
	page, err := NewRuntime(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	page.loaded = true
	page.runtime = application.RuntimeOverview{UserService: application.ServiceOverview{Scope: managed.ScopeUser, Supported: true}}
	if runtime.GOOS != "windows" {
		page.runtime.SystemService = application.ServiceOverview{Scope: managed.ScopeSystem, Supported: true}
	}
	page.auth = application.AuthStatus{MCPConfigured: true, AdminConfigured: true}
	page.install = application.InstallationOverview{AliasAvailable: true, Alias: install.AliasStatus{State: install.AliasMissing}}
	page.about = application.AboutInfo{Version: "v1.2.3"}
	page.rebuildBrowser("")
	ids := map[string]bool{}
	for _, id := range []string{"runtime", "transport.mcp-http", "service.user", "auth.mcp", "auth.admin", "installation", "alias", "update", "about"} {
		if !page.browser.SelectID(id) {
			t.Fatalf("row missing: %s", id)
		}
		ids[id] = true
	}
	if runtime.GOOS != "windows" && !page.browser.SelectID("service.system") {
		t.Fatal("system service row missing")
	}
}

func TestRuntimeRowsUseDescriptiveTitlesAndDescriptions(t *testing.T) {
	page, _ := NewRuntime(t.Context())
	page.runtime = application.RuntimeOverview{Running: true, Status: runtimecontrol.RuntimeStatus{PID: 4242, Managed: true, ServiceScope: "user"}, UserService: application.ServiceOverview{Scope: managed.ScopeUser, Supported: true, Installed: true, Running: true, Backend: "systemd --user", PID: 4242}}
	runtimeRow := page.runtimeRow()
	if runtimeRow.Title != "MCP runtime process" || !strings.Contains(runtimeRow.Description, "running · pid 4242 · managed / user") {
		t.Fatalf("runtime row=%#v", runtimeRow)
	}
	page.runtime.Status.Starting = true
	runtimeRow = page.runtimeRow()
	if !strings.Contains(runtimeRow.Description, "starting · pid 4242 · managed / user") || !strings.Contains(runtimeRow.Detail, "starting") {
		t.Fatalf("starting runtime row=%#v", runtimeRow)
	}
	if !strings.Contains(page.statusView(80), "STARTING") {
		t.Fatalf("starting runtime summary=%q", page.statusView(80))
	}
	serviceRow := page.serviceRow(page.runtime.UserService)
	if serviceRow.Title != "User managed service" || !strings.Contains(serviceRow.Description, "systemd --user") || !strings.Contains(serviceRow.Description, "pid 4242") {
		t.Fatalf("service row=%#v", serviceRow)
	}
	mcpHTTP := page.mcpHTTPRow()
	if mcpHTTP.Title != "MCP HTTP server" || !strings.Contains(mcpHTTP.Description, "port closed") {
		t.Fatalf("MCP HTTP row=%#v", mcpHTTP)
	}
	mcpAuth, adminAuth := page.authRow("mcp"), page.authRow("admin")
	if mcpAuth.Title != "MCP HTTP authentication" || adminAuth.Title != "Admin UI authentication" || !strings.Contains(mcpAuth.Description, "auth only") || !strings.Contains(mcpAuth.Detail, "listener is controlled by MCP HTTP server") {
		t.Fatalf("authentication rows MCP=%#v admin=%#v", mcpAuth, adminAuth)
	}
}

func TestRuntimeMCPHTTPRowDistinguishesConfigFromLiveListener(t *testing.T) {
	page, _ := NewRuntime(t.Context())
	page.runtime = application.RuntimeOverview{Running: true, MCPHTTPEnabled: false, MCPHTTPPort: 37421, TunnelEnabled: true, Status: runtimecontrol.RuntimeStatus{ServerEnabled: true, ServerPort: 37421}}
	row := page.mcpHTTPRow()
	if !strings.Contains(row.Description, "disabled · listening · reload required") || !strings.Contains(row.Detail, "http://127.0.0.1:37421/mcp") || !strings.Contains(row.Detail, "Tunnel") {
		t.Fatalf("MCP HTTP mismatch row=%#v", row)
	}
	page.runtime.Status.ServerEnabled = false
	row = page.mcpHTTPRow()
	if !strings.Contains(row.Description, "disabled · port closed") || strings.Contains(row.Detail, "http://127.0.0.1:37421/mcp") {
		t.Fatalf("MCP HTTP disabled row=%#v", row)
	}
}

func TestRuntimeMCPHTTPToggleUsesTransportAction(t *testing.T) {
	page, _ := NewRuntime(t.Context())
	page.runtime = application.RuntimeOverview{MCPHTTPEnabled: true, MCPHTTPPort: 37421, TunnelEnabled: true}
	page.rebuildBrowser("transport.mcp-http")
	cmd, handled := page.handleKey(tea.KeyPressMsg{Code: tea.KeySpace})
	if !handled || cmd == nil || page.pending != MCPHTTPDisable || page.overlay != systemOverlayOperation {
		t.Fatalf("disable toggle handled=%t cmd=%v pending=%q overlay=%d", handled, cmd, page.pending, page.overlay)
	}
	page.cancelOperation()
	page.runtime.MCPHTTPEnabled = false
	page.overlay = systemOverlayNone
	cmd, handled = page.handleKey(tea.KeyPressMsg{Code: 'e', Text: "e"})
	if !handled || cmd == nil || page.pending != MCPHTTPEnable || page.overlay != systemOverlayOperation {
		t.Fatalf("enable toggle handled=%t cmd=%v pending=%q overlay=%d", handled, cmd, page.pending, page.overlay)
	}
	page.cancelOperation()
}

func TestRuntimeMCPHTTPStoppedTogglePersistsAndRespectsTransportInvariant(t *testing.T) {
	previous := configformat.RootPath()
	t.Cleanup(func() { _ = configformat.SetRootPath(previous) })
	if err := configformat.SetRootPath(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Auth.MCPEnabled, cfg.Auth.AdminEnabled = false, false
	cfg.Tunnel.Enabled, cfg.Tunnel.ID, cfg.Tunnel.APIKey = true, "tunnel_test", "tunnel-key"
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	page, _ := NewRuntime(t.Context())
	page.runtime = application.RuntimeOverview{MCPHTTPEnabled: true, MCPHTTPPort: cfg.Server.Port, TunnelEnabled: true}
	cmd, err := page.openCommand(MCPHTTPDisable)
	if err != nil || cmd == nil {
		t.Fatalf("disable command err=%v cmd=%v", err, cmd)
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok || len(batch) < 2 {
		t.Fatalf("disable command message=%T", msg)
	}
	var result systemOperationMsg
	for _, next := range batch {
		if next == nil {
			continue
		}
		if value, ok := next().(systemOperationMsg); ok {
			result = value
			break
		}
	}
	if result.err != nil || !strings.Contains(result.notice, "applies on next runtime start") {
		t.Fatalf("disable result=%#v err=%v", result, result.err)
	}
	loaded, err := config.Load()
	if err != nil || loaded.Server.Enabled {
		t.Fatalf("server enabled=%t err=%v", loaded.Server.Enabled, err)
	}
	loaded.Tunnel.Enabled = false
	loaded.Server.Enabled = true
	if err := config.Save(loaded); err != nil {
		t.Fatal(err)
	}
	page.runtime.MCPHTTPEnabled, page.runtime.TunnelEnabled = true, false
	cmd, err = page.openCommand(MCPHTTPDisable)
	if err != nil || cmd == nil {
		t.Fatalf("invariant command err=%v cmd=%v", err, cmd)
	}
	msg = cmd()
	batch, ok = msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("invariant message=%T", msg)
	}
	result = systemOperationMsg{}
	for _, next := range batch {
		if next == nil {
			continue
		}
		if value, ok := next().(systemOperationMsg); ok {
			result = value
			break
		}
	}
	if result.err == nil || !strings.Contains(result.err.Error(), "at least one MCP transport") {
		t.Fatalf("invariant result=%#v", result)
	}
}

func TestRuntimeTokenRotationRequiresConfirmAndSecretIsTransient(t *testing.T) {
	page, _ := NewRuntime(t.Context())
	cmd, err := page.openCommand(AuthMCPRotate)
	if err != nil || cmd != nil || page.overlay != systemOverlayConfirm {
		t.Fatalf("rotation did not open confirmation: overlay=%v cmd=%v err=%v", page.overlay, cmd, err)
	}
	if page.confirm.AffirmativeSelected() {
		t.Fatal("token rotation confirmation defaulted affirmative")
	}
	page.operationID = 9
	const token = "secret-token-value"
	page.finishOperation(systemOperationMsg{id: 9, command: AuthMCPRotate, token: token})
	if page.overlay != systemOverlaySecret || page.secret != token {
		t.Fatalf("secret overlay missing: overlay=%v secret=%q", page.overlay, page.secret)
	}
	if !strings.Contains(page.View(100, 30), token) {
		t.Fatal("one-time token not shown")
	}
	updated, _ := page.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	page = updated.(*RuntimePage)
	if page.secret != "" || page.secretKind != "" || page.overlay != systemOverlayNone {
		t.Fatalf("secret survived close: overlay=%v secret=%q kind=%q", page.overlay, page.secret, page.secretKind)
	}
	if strings.Contains(page.notice, token) || strings.Contains(page.View(100, 30), token) {
		t.Fatal("token leaked outside one-time overlay")
	}
}

func TestRuntimeExternalCommandUsesExplicitOverlay(t *testing.T) {
	page, _ := NewRuntime(context.Background())
	page.operationID = 4
	external := &application.ExternalCommand{Command: "cgm update", Reason: "requires elevation"}
	page.finishOperation(systemOperationMsg{id: 4, command: UpdateApply, external: external})
	if page.overlay != systemOverlayExternal || page.external != external {
		t.Fatalf("external workflow not surfaced: overlay=%v external=%#v", page.overlay, page.external)
	}
	view := page.View(100, 30)
	if !strings.Contains(view, external.Command) || !strings.Contains(view, external.Reason) {
		t.Fatalf("external workflow missing from view: %q", view)
	}
	updated, _ := page.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	page = updated.(*RuntimePage)
	if page.external != nil || page.overlay != systemOverlayNone {
		t.Fatal("external command survived overlay close")
	}
}

func TestRuntimeForegroundUsesExplicitExternalWorkflow(t *testing.T) {
	page, _ := NewRuntime(t.Context())
	cmd, err := page.openCommand(RuntimeForeground)
	if err != nil || cmd != nil {
		t.Fatalf("foreground workflow err=%v cmd=%v", err, cmd)
	}
	if page.overlay != systemOverlayExternal || page.external == nil {
		t.Fatalf("foreground workflow not external: overlay=%v external=%#v", page.overlay, page.external)
	}
	if page.external.Command != "cgm serve" || !strings.Contains(page.external.Reason, "Exit the TUI") {
		t.Fatalf("foreground workflow=%#v", page.external)
	}
}

func TestRuntimeInstallAndUpdateFormsRequireConfirmation(t *testing.T) {
	page, _ := NewRuntime(t.Context())
	if _, err := page.openCommand(InstallRun); err != nil {
		t.Fatal(err)
	}
	if page.overlay != systemOverlayForm || page.installForm == nil {
		t.Fatal("install form did not open")
	}
	page.submitForm()
	if page.err == nil || page.overlay != systemOverlayNone {
		t.Fatal("unconfirmed install was accepted")
	}
	page.err = nil
	if _, err := page.openCommand(UpdateApply); err != nil {
		t.Fatal(err)
	}
	if page.overlay != systemOverlayForm || page.updateForm == nil {
		t.Fatal("update form did not open")
	}
	page.submitForm()
	if page.err == nil || page.overlay != systemOverlayNone {
		t.Fatal("unconfirmed update was accepted")
	}
}

func TestRuntimeCloseCancelsOperation(t *testing.T) {
	page, _ := NewRuntime(t.Context())
	ctx, cancel := context.WithCancel(context.Background())
	page.operationCancel = cancel
	page.Close()
	select {
	case <-ctx.Done():
	default:
		t.Fatal("page close did not cancel operation")
	}
}

func TestRuntimeUpdateOperationsUseInlineTitleNotice(t *testing.T) {
	page, _ := NewRuntime(t.Context())
	for _, test := range []struct {
		name string
		msg  systemOperationMsg
		want string
	}{
		{name: "check", msg: systemOperationMsg{id: 1, command: UpdateCheck, update: updatepkg.CheckResult{Status: updatepkg.StatusUpToDate, Latest: "v1.2.3"}}, want: "latest v1.2.3"},
		{name: "apply", msg: systemOperationMsg{id: 2, command: UpdateApply, notice: "Updated to v1.2.3"}, want: "Updated to v1.2.3"},
	} {
		t.Run(test.name, func(t *testing.T) {
			page.operationID = test.msg.id
			cmd := page.finishOperation(test.msg)
			if !strings.Contains(page.notice, test.want) {
				t.Fatalf("update title notice=%q", page.notice)
			}
			if cmd == nil {
				t.Fatal("update completion did not schedule reload")
			}
			view := page.View(100, 30)
			if !strings.Contains(view, page.notice) {
				t.Fatalf("update notice missing beside page title: %q", view)
			}
		})
	}
}

func TestRuntimeUpdateFailureRemainsErrorFeedback(t *testing.T) {
	page, _ := NewRuntime(t.Context())
	page.operationID = 3
	page.finishOperation(systemOperationMsg{id: 3, command: UpdateApply, err: errors.New("update failed")})
	if page.err == nil || page.err.Error() != "update failed" || page.notice != "" {
		t.Fatalf("failure err=%v notice=%q", page.err, page.notice)
	}
}
