package page

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/install"
	managed "go.mewis.me/chatgpt-mcp/internal/service"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
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
	for _, id := range []string{"runtime", "service.user", "auth.mcp", "auth.admin", "installation", "alias", "update", "about"} {
		if !page.browser.SelectID(id) {
			t.Fatalf("row missing: %s", id)
		}
		ids[id] = true
	}
	if runtime.GOOS != "windows" && !page.browser.SelectID("service.system") {
		t.Fatal("system service row missing")
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

func TestRuntimeUpdateOperationsEmitToastWithoutInlineNotice(t *testing.T) {
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
			if page.notice != "" {
				t.Fatalf("update left inline notice=%q", page.notice)
			}
			toast, ok := updateToast(test.msg)
			if !ok || toast.Tone != component.ToneSuccess || !strings.Contains(toast.Message, test.want) {
				t.Fatalf("toast=%#v ok=%t", toast, ok)
			}
			if cmd == nil {
				t.Fatal("update completion did not schedule reload/toast")
			}
		})
	}
}

func TestRuntimeUpdateFailureEmitsDangerToastAndExternalWorkflowDoesNot(t *testing.T) {
	failure := systemOperationMsg{id: 3, command: UpdateApply, err: errors.New("update failed")}
	toast, ok := updateToast(failure)
	if !ok || toast.Tone != component.ToneDanger || toast.Message != "update failed" {
		t.Fatalf("failure toast=%#v ok=%t", toast, ok)
	}
	external := systemOperationMsg{id: 4, command: UpdateApply, external: &application.ExternalCommand{Command: "cgm update"}}
	if toast, ok := updateToast(external); ok || toast != (ToastMsg{}) {
		t.Fatalf("external update unexpectedly toasted: %#v ok=%t", toast, ok)
	}
}
