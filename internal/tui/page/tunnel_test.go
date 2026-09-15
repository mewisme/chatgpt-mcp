package page

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
	"go.mewis.me/chatgpt-mcp/internal/tui/testutil"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

func TestTunnelRuntimeEditorsRedactSecretsAndBlankRuntimeKeyPreservesSecret(t *testing.T) {
	setupTunnelPageConfig(t, tunnel.Config{ID: "tunnel_demo", APIKey: "runtime-secret", AdminKey: "admin-secret", AdminWorkspaceID: "ws_admin"})
	dashboard, err := NewTunnelDashboard(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if view := dashboard.View(120, 32); strings.Contains(view, "runtime-secret") || strings.Contains(view, "admin-secret") {
		t.Fatalf("secret leaked in tunnel dashboard: %q", view)
	}
	runtimeEditor, err := NewTunnelDashboardRoute(t.Context(), "", "edit")
	if err != nil {
		t.Fatal(err)
	}
	_ = runtimeEditor.Init()
	if runtimeEditor.OverlayActive() || runtimeEditor.runtimeForm == nil || runtimeEditor.runtimeForm.RuntimeAPIKey != "" {
		t.Fatalf("runtime editor overlay=%t draft=%#v", runtimeEditor.OverlayActive(), runtimeEditor.runtimeForm)
	}
	if input := runtimeInputFromForm(runtimeEditor.runtimeForm); input.APIKey != nil {
		t.Fatalf("blank runtime key should preserve existing secret: %#v", input.APIKey)
	}
	view := ansi.Strip(runtimeEditor.View(100, 28))
	if strings.Contains(view, "runtime-secret") || !strings.Contains(view, "Configure the selected runtime tunnel") || !strings.Contains(view, "enter next") || strings.Contains(view, "Configure Runtime Tunnel") {
		t.Fatalf("runtime editor view=%q", view)
	}
	adminEditor, err := NewTunnelDashboardRoute(t.Context(), "admin-key", "edit")
	if err != nil {
		t.Fatal(err)
	}
	_ = adminEditor.Init()
	view = ansi.Strip(adminEditor.View(100, 28))
	if adminEditor.OverlayActive() || adminEditor.adminForm == nil || adminEditor.adminForm.AdminKey != "" || strings.Contains(view, "admin-secret") || !strings.Contains(view, "enter next") {
		t.Fatalf("admin editor overlay=%t draft=%#v view=%q", adminEditor.OverlayActive(), adminEditor.adminForm, view)
	}
}

func TestTunnelRuntimeTitleStartsAtWorkspaceTitlePosition(t *testing.T) {
	setupTunnelPageConfig(t, tunnel.Config{})
	page, err := NewTunnelDashboard(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(ansi.Strip(page.View(100, 32)), "\n")
	if len(lines) < 2 || !strings.Contains(lines[0], "OpenAI Secure MCP Tunnel") || strings.TrimSpace(lines[1]) != "" {
		t.Fatalf("tunnel title lines=%q", lines[:min(2, len(lines))])
	}
}

func TestManagedTunnelMutationNoticeRendersWithoutDuplicateChildTitle(t *testing.T) {
	setupTunnelPageConfig(t, tunnel.Config{})
	page, err := NewManagedTunnels(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	page.notice = "Managed tunnel created"
	view := ansi.Strip(page.View(100, 24))
	if !strings.Contains(view, "Managed tunnel created") || strings.Contains(view, "Managed tunnels") {
		t.Fatalf("managed tunnel child view=%q", view)
	}
}

func TestManagedTunnelBrowserUsesAttachShortcut(t *testing.T) {
	setupTunnelPageConfig(t, tunnel.Config{AdminKey: "admin-secret", AdminWorkspaceID: "ws_admin", AdminReadAccess: true, AdminManageAccess: true})
	item := tunnel.Metadata{ID: "tunnel_one", Name: "One", Description: "primary"}
	if _, err := config.SaveTunnelMetadata(item); err != nil {
		t.Fatal(err)
	}
	page, err := NewManagedTunnels(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	view := ansi.Strip(page.View(100, 24))
	if !strings.Contains(view, "t attach") {
		t.Fatalf("managed browser missing attach shortcut: %q", view)
	}
	_, cmd := page.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	if cmd == nil {
		t.Fatal("managed browser attach shortcut returned no command")
	}
	navigate, ok := cmd().(NavigateMsg)
	if !ok || strings.Join(navigate.Path, "/") != "tunnels/tunnel_one/configure" {
		t.Fatalf("managed browser attach navigation=%#v", navigate)
	}
}

func TestManagedTunnelResourceUsesRoutedChildDetailPage(t *testing.T) {
	setupTunnelPageConfig(t, tunnel.Config{AdminKey: "admin-secret", AdminWorkspaceID: "ws_admin", AdminReadAccess: true, AdminManageAccess: true})
	item := tunnel.Metadata{ID: "tunnel_one", Name: "One", Description: "primary", OrganizationIDs: []string{"org_one"}, WorkspaceIDs: []string{"ws_one"}, TenantIDs: []string{"tenant_one"}}
	if _, err := config.SaveTunnelMetadata(item); err != nil {
		t.Fatal(err)
	}
	page, err := NewManagedTunnelsRoute(t.Context(), item.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if page.OverlayActive() {
		t.Fatal("managed tunnel detail incorrectly reports overlay active")
	}
	view := ansi.Strip(page.View(100, 26))
	for _, want := range []string{"One", "primary", "s scope", "r refresh", "t attach", "? more"} {
		if !strings.Contains(view, want) {
			t.Fatalf("managed detail missing %q: %q", want, view)
		}
	}
	updated, _ := page.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	page = updated.(*TunnelPage)
	view = ansi.Strip(page.View(100, 26))
	for _, want := range []string{"update", "delete", "less"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expanded managed detail missing %q: %q", want, view)
		}
	}
	if strings.Contains(view, "Overview   Scope") || strings.Contains(view, "╭") {
		t.Fatalf("managed detail retained tab/modal chrome: %q", view)
	}
	_, cmd := page.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	if cmd == nil {
		t.Fatal("scope child navigation returned no command")
	}
	navigate, ok := cmd().(NavigateMsg)
	if !ok || strings.Join(navigate.Path, "/") != "tunnels/tunnel_one/scope" {
		t.Fatalf("scope navigation=%#v", navigate)
	}
	scope, err := NewManagedTunnelsRoute(t.Context(), item.ID, "scope")
	if err != nil {
		t.Fatal(err)
	}
	scopeView := ansi.Strip(scope.View(100, 26))
	for _, want := range []string{"org_one", "ws_one", "tenant_one"} {
		if !strings.Contains(scopeView, want) {
			t.Fatalf("scope detail missing %q: %q", want, scopeView)
		}
	}
}

func TestTunnelRuntimeKeyHintsStayAtBottom(t *testing.T) {
	setupTunnelPageConfig(t, tunnel.Config{})
	page, err := NewTunnelDashboard(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(ansi.Strip(page.View(120, 32)), "\n")
	last := len(lines) - 1
	for last >= 0 && strings.TrimSpace(lines[last]) == "" {
		last--
	}
	if last != 31 || !strings.Contains(lines[last], "managed tunnels") || strings.Contains(lines[last], "? more") {
		t.Fatalf("tunnel help line=%d want=31 view=%q", last, strings.Join(lines, "\n"))
	}
}

func TestTunnelInstancesDetailUsesIDScopedActionsAndRedactsSecrets(t *testing.T) {
	instances := []tunnel.InstanceConfig{{Enabled: true, ID: "tunnel_demo", APIKey: "runtime-secret", AdminProfileID: "work", OrganizationID: "org_demo"}}
	admins := []tunnel.AdminConfig{{ID: "work", AdminKey: "admin-secret", OrganizationID: "org_demo"}}
	setupTunnelPageConfig(t, tunnel.Config{Instances: &instances, Admins: &admins})
	page, err := NewTunnelInstances(t.Context(), "tunnel_demo")
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := page.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	page = updated.(*TunnelInstancesPage)
	plain := ansi.Strip(page.View(100, 28))
	for _, want := range []string{"tunnel_demo", "Runtime key", "configured", "Admin profile", "work", "Organization", "org_demo", "disable", "start", "run", "detach"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("detail missing %q: %q", want, plain)
		}
	}
	if strings.Contains(plain, "runtime-secret") || strings.Contains(plain, "admin-secret") {
		t.Fatalf("tunnel secret leaked in detail: %q", plain)
	}
}

func TestTunnelRuntimeEditorPasswordLabelAlignsWithOtherFields(t *testing.T) {
	setupTunnelPageConfig(t, tunnel.Config{ID: "tunnel_demo", APIKey: "runtime-secret"})
	page, err := NewTunnelDashboardRoute(t.Context(), "", "edit")
	if err != nil {
		t.Fatal(err)
	}
	_ = page.Init()
	plain := ansi.Strip(page.View(100, 30))
	if strings.Count(plain, "Blank keeps the current key.") != 1 || !strings.Contains(plain, "> Blank keeps the current key.") {
		t.Fatalf("runtime key placeholder=%q", plain)
	}
	labelColumn := func(label string) int {
		for _, line := range strings.Split(plain, "\n") {
			if column := strings.Index(line, label); column >= 0 {
				return column
			}
		}
		return -1
	}
	idColumn, keyColumn := labelColumn("Tunnel ID"), labelColumn("Runtime API key")
	if idColumn < 0 || keyColumn < 0 || idColumn != keyColumn {
		t.Fatalf("tunnel label columns id=%d runtime-key=%d view=%q", idColumn, keyColumn, plain)
	}
}

func TestTunnelAdminEditorFailureKeepsDraft(t *testing.T) {
	setupTunnelPageConfig(t, tunnel.Config{})
	page, err := NewTunnelDashboardRoute(t.Context(), "admin-key", "edit")
	if err != nil {
		t.Fatal(err)
	}
	_ = page.Init()
	updated, _ := page.Update(tea.KeyPressMsg{Code: 's', Text: "secret-draft"})
	page = updated.(*TunnelPage)
	if page.adminForm.AdminKey != "secret-draft" || !page.Dirty() {
		t.Fatalf("admin draft=%#v dirty=%t", page.adminForm, page.Dirty())
	}
	updated, cmd := page.Update(component.EditorSubmitMsg{})
	page = updated.(*TunnelPage)
	if cmd == nil || page.overlay != tunnelOverlayOperation {
		t.Fatalf("admin submit cmd=%v overlay=%d", cmd != nil, page.overlay)
	}
	updated, _ = page.Update(tunnelOperationMsg{command: TunnelAdminKeySet, err: fmt.Errorf("verification failed")})
	page = updated.(*TunnelPage)
	if page.OverlayActive() || page.adminForm == nil || page.adminForm.AdminKey != "secret-draft" || !page.Dirty() {
		t.Fatalf("admin failure lost draft overlay=%t draft=%#v dirty=%t", page.OverlayActive(), page.adminForm, page.Dirty())
	}
	plain := ansi.Strip(page.View(90, 26))
	if !strings.Contains(plain, "verification failed") || strings.Contains(plain, "secret-draft") {
		t.Fatalf("admin failure feedback/secret view=%q", plain)
	}
}

func TestTunnelRuntimeOperationOverlayBlocksEditorMouse(t *testing.T) {
	setupTunnelPageConfig(t, tunnel.Config{Enabled: true, ID: "tunnel_demo"})
	page, err := NewTunnelDashboardRoute(t.Context(), "", "edit")
	if err != nil {
		t.Fatal(err)
	}
	page.width, page.height = 100, 28
	page.overlay = tunnelOverlayOperation
	progress := component.NewProgress("Saving tunnel configuration")
	page.progress = &progress
	targets := page.MouseTargets(0, 0, 1)
	if len(targets) != 1 || targets[0].ID != "page.overlay" {
		t.Fatalf("operation mouse targets=%#v", targets)
	}
}

func TestTunnelInstancesLayoutShowsCollectionSummaryAndManagedShortcut(t *testing.T) {
	instances := []tunnel.InstanceConfig{{Enabled: true, ID: "tunnel_a", APIKey: "runtime-a", AdminProfileID: "work"}, {ID: "tunnel_b", APIKey: "runtime-b"}}
	admins := []tunnel.AdminConfig{{ID: "work", AdminKey: "admin-secret"}}
	setupTunnelPageConfig(t, tunnel.Config{Instances: &instances, Admins: &admins})
	page, err := NewTunnelInstances(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	view := ansi.Strip(page.View(120, 32))
	for _, want := range []string{"OpenAI Secure MCP Tunnels", "2 attached", "1 admin profiles", "tunnel_a", "tunnel_b", "a admins", "m managed"} {
		if !strings.Contains(view, want) {
			t.Fatalf("collection view missing %q: %q", want, view)
		}
	}
	if strings.Contains(view, "runtime-a") || strings.Contains(view, "runtime-b") || strings.Contains(view, "admin-secret") {
		t.Fatalf("collection view leaked secrets: %q", view)
	}
	testutil.AssertLinesFit(t, page.View(72, 24), 72)
}

func TestTunnelRuntimeKeyHintsUseDefaultHelpStyle(t *testing.T) {
	setupTunnelPageConfig(t, tunnel.Config{Enabled: true, ID: "tunnel_demo", APIKey: "runtime-secret", AdminKey: "admin-secret", AdminOrganizationID: "org_demo"})
	page, err := NewTunnelDashboard(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	page.height = 32
	view := page.runtimeView(120)
	want := component.DefaultHelp(120, page.runtimeHelpBindings()...)
	if !strings.Contains(view, want) {
		t.Fatalf("tunnel help does not use default help styling\nwant: %q\nview: %q", want, view)
	}
}

func TestTunnelPageActionMouseSpaceUsesSpaceKey(t *testing.T) {
	message := pageActionKeyMsg("space")
	if message.String() != "space" {
		t.Fatalf("space action key=%q", message.String())
	}
}

func TestManagedTunnelRefreshPersistsCacheAndEditUsesCachedState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer admin-secret" {
			t.Fatalf("authorization=%q", r.Header.Get("Authorization"))
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/tunnels":
			if r.URL.Query().Get("workspace_id") != "ws_admin" {
				t.Fatalf("scope=%q", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"tunnels":[{"id":"tunnel_one","name":"Cached One","description":"first","workspace_ids":["ws_admin"]},{"id":"tunnel_two","name":"Two","description":"second","workspace_ids":["ws_admin"]}]}`))
		default:
			t.Fatalf("unexpected request=%s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
	}))
	defer server.Close()
	setupTunnelPageConfig(t, tunnel.Config{AdminKey: "admin-secret", AdminWorkspaceID: "ws_admin", ControlPlaneBaseURL: server.URL})
	if _, err := config.SaveTunnelMetadata(tunnel.Metadata{ID: "stale", Name: "Stale"}); err != nil {
		t.Fatal(err)
	}
	page, err := NewManagedTunnels(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	cmd, err := page.openCommand(TunnelManagedRefresh, "")
	if err != nil || cmd == nil || page.overlay != tunnelOverlayOperation {
		t.Fatalf("refresh cmd=%v err=%v overlay=%d", cmd, err, page.overlay)
	}
	updated, _ := page.Update(cmd())
	page = updated.(*TunnelPage)
	if len(page.items) != 2 || page.items[0].ID != "tunnel_one" || !strings.Contains(page.notice, "2") {
		t.Fatalf("items=%#v notice=%q", page.items, page.notice)
	}
	cached, err := config.LoadTunnelMetadata("tunnel_two")
	if err != nil || cached.Name != "Two" {
		t.Fatalf("cached=%#v err=%v", cached, err)
	}

	cmd, err = page.openCommand(TunnelManagedUpdate, "tunnel_one")
	if err != nil || cmd == nil {
		t.Fatalf("update route cmd=%v err=%v", cmd, err)
	}
	navigate, ok := cmd().(NavigateMsg)
	if !ok || strings.Join(navigate.Path, "/") != "tunnels/tunnel_one/edit" {
		t.Fatalf("update route=%#v", navigate)
	}
	edit, err := NewManagedTunnelsRouteAction(t.Context(), "tunnel_one", "", "edit")
	if err != nil {
		t.Fatal(err)
	}
	if edit.editor == nil || edit.managedUpdateFetch || edit.overlay != tunnelOverlayNone || edit.managedForm == nil || edit.managedForm.Name != "Cached One" || edit.managedForm.Description != "first" {
		t.Fatalf("cached editor=%v fetch=%t overlay=%d form=%#v", edit.editor != nil, edit.managedUpdateFetch, edit.overlay, edit.managedForm)
	}
	if edit.Init() == nil {
		t.Fatal("edit editor init command missing")
	}
}

func TestManagedTunnelEditCancelReturnsToDetail(t *testing.T) {
	setupTunnelPageConfig(t, tunnel.Config{AdminKey: "admin-secret", AdminWorkspaceID: "ws_admin"})
	if _, err := config.SaveTunnelMetadata(tunnel.Metadata{ID: "tunnel_one", Name: "One"}); err != nil {
		t.Fatal(err)
	}
	page, err := NewManagedTunnelsRouteAction(t.Context(), "tunnel_one", "", "edit")
	if err != nil {
		t.Fatal(err)
	}
	updated, cmd := page.Update(component.EditorCancelMsg{})
	page = updated.(*TunnelPage)
	if cmd == nil || page.overlay != tunnelOverlayNone {
		t.Fatalf("cancel cmd=%v overlay=%d", cmd != nil, page.overlay)
	}
	navigate, ok := cmd().(NavigateMsg)
	if !ok || strings.Join(navigate.Path, "/") != "tunnels/tunnel_one" {
		t.Fatalf("escape navigation=%#v", navigate)
	}
}

func TestManagedTunnelEditRequiresCachedMetadata(t *testing.T) {
	setupTunnelPageConfig(t, tunnel.Config{AdminKey: "admin-secret", AdminWorkspaceID: "ws_admin"})
	if _, err := NewManagedTunnelsRouteAction(t.Context(), "tunnel_one", "", "edit"); err == nil || !strings.Contains(err.Error(), "not found in local cache") {
		t.Fatalf("missing cached metadata err=%v", err)
	}
}

func TestManagedTunnelRefreshCancellationIgnoresLateResult(t *testing.T) {
	started := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/tunnels" {
			t.Fatalf("unexpected request=%s %s", r.Method, r.URL.Path)
		}
		select {
		case started <- struct{}{}:
		default:
		}
		<-r.Context().Done()
	}))
	defer server.Close()
	setupTunnelPageConfig(t, tunnel.Config{AdminKey: "admin-secret", AdminWorkspaceID: "ws_admin", ControlPlaneBaseURL: server.URL})
	page, err := NewManagedTunnels(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	cmd, err := page.openCommand(TunnelManagedRefresh, "")
	if err != nil || cmd == nil {
		t.Fatalf("cmd=%v err=%v", cmd, err)
	}
	result := make(chan tea.Msg, 1)
	go func() { result <- cmd() }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("managed refresh did not start")
	}
	updated, _ := page.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	page = updated.(*TunnelPage)
	if page.overlay != tunnelOverlayNone || !page.operationCancelled {
		t.Fatalf("cancel state overlay=%d cancelled=%t", page.overlay, page.operationCancelled)
	}
	select {
	case message := <-result:
		updated, _ = page.Update(message)
		page = updated.(*TunnelPage)
	case <-time.After(time.Second):
		t.Fatal("cancelled managed refresh did not return")
	}
	if page.operationCancelled || page.err != nil || !strings.Contains(page.notice, "cancel") {
		t.Fatalf("completion cancelled=%t err=%v notice=%q", page.operationCancelled, page.err, page.notice)
	}
}

func TestManagedTunnelDeleteUsesProfileScopedEditor(t *testing.T) {
	setupTunnelPageConfig(t, tunnel.Config{Enabled: true, ID: "tunnel_selected", APIKey: "runtime-secret", AdminKey: "admin-secret", AdminWorkspaceID: "ws_admin"})
	if _, err := config.SaveTunnelMetadata(tunnel.Metadata{ID: "tunnel_selected", Name: "Selected"}); err != nil {
		t.Fatal(err)
	}
	page, err := NewManagedTunnels(t.Context(), "tunnel_selected")
	if err != nil {
		t.Fatal(err)
	}
	cmd, err := page.openCommand(TunnelManagedDelete, "tunnel_selected")
	if err != nil || cmd == nil {
		t.Fatal(err)
	}
	navigate, ok := cmd().(NavigateMsg)
	if !ok || strings.Join(navigate.Path, "/") != "tunnels/tunnel_selected/delete" {
		t.Fatalf("delete navigation=%#v", navigate)
	}
	deletePage, err := NewManagedTunnelsRouteAction(t.Context(), "tunnel_selected", "", "delete")
	if err != nil || deletePage.editor == nil || deletePage.deleteForm == nil || deletePage.deleteForm.AdminProfileID != "default" {
		t.Fatalf("delete editor=%v form=%#v err=%v", deletePage != nil && deletePage.editor != nil, deletePage.deleteForm, err)
	}
}

func TestManagedTunnelCreateEditorSectionsWrapAndFailureKeepsDraft(t *testing.T) {
	setupTunnelPageConfig(t, tunnel.Config{AdminKey: "admin-secret", AdminWorkspaceID: "ws_admin", AdminReadAccess: true, AdminManageAccess: true})
	page, err := NewManagedTunnelsRouteAction(t.Context(), "", "", "create")
	if err != nil {
		t.Fatal(err)
	}
	_ = page.Init()
	plain := ansi.Strip(page.View(40, 20))
	for _, want := range []string{"Admin", "General", "Scope", "tab next"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("create editor missing %q: %q", want, plain)
		}
	}
	if strings.Contains(plain, "Create Managed Tunnel") {
		t.Fatalf("create editor retained redundant page title: %q", plain)
	}
	testutil.AssertLinesFit(t, page.View(40, 20), 40)
	updated, _ := page.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	page = updated.(*TunnelPage)
	updated, _ = page.Update(tea.KeyPressMsg{Code: 'd', Text: "draft-name"})
	page = updated.(*TunnelPage)
	if page.managedForm == nil || page.managedForm.Name != "draft-name" || !page.Dirty() {
		t.Fatalf("create draft=%#v dirty=%t", page.managedForm, page.Dirty())
	}
	updated, _ = page.Update(tunnelOperationMsg{command: TunnelManagedCreate, err: fmt.Errorf("create failed")})
	page = updated.(*TunnelPage)
	if page.editor == nil || page.managedForm == nil || page.managedForm.Name != "draft-name" || !page.Dirty() || page.OverlayActive() {
		t.Fatalf("create failure editor=%v draft=%#v dirty=%t overlay=%t", page.editor != nil, page.managedForm, page.Dirty(), page.OverlayActive())
	}
	if view := ansi.Strip(page.View(40, 20)); !strings.Contains(view, "create failed") {
		t.Fatalf("create failure feedback=%q", view)
	}
}

func TestManagedTunnelUpdateAndConfigureFailuresKeepDraft(t *testing.T) {
	setupTunnelPageConfig(t, tunnel.Config{AdminKey: "admin-secret", AdminWorkspaceID: "ws_admin"})
	if _, err := config.SaveTunnelMetadata(tunnel.Metadata{ID: "tunnel_one", Name: "Remote", Description: "fresh", WorkspaceIDs: []string{"ws_admin"}}); err != nil {
		t.Fatal(err)
	}
	edit, err := NewManagedTunnelsRouteAction(t.Context(), "tunnel_one", "", "edit")
	if err != nil {
		t.Fatal(err)
	}
	if edit.editor == nil || edit.managedForm == nil || edit.managedForm.Name != "Remote" {
		t.Fatalf("edit editor=%v draft=%#v", edit.editor != nil, edit.managedForm)
	}
	_ = edit.Init()
	updated, _ := edit.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	edit = updated.(*TunnelPage)
	updated, _ = edit.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	edit = updated.(*TunnelPage)
	name := edit.managedForm.Name
	updated, _ = edit.Update(tunnelOperationMsg{command: TunnelManagedUpdate, targetID: "tunnel_one", err: fmt.Errorf("update failed")})
	edit = updated.(*TunnelPage)
	if edit.managedForm == nil || edit.managedForm.Name != name || !edit.Dirty() || !strings.Contains(ansi.Strip(edit.View(52, 22)), "update failed") {
		t.Fatalf("update failure draft=%#v dirty=%t", edit.managedForm, edit.Dirty())
	}

	configure, err := NewManagedTunnelsRouteAction(t.Context(), "tunnel_one", "", "configure")
	if err != nil {
		t.Fatal(err)
	}
	_ = configure.Init()
	if configure.configureForm == nil || configure.configureForm.RuntimeKeyMode != "auto" || configure.configureForm.AdminProfileID != "default" {
		t.Fatalf("configure draft=%#v", configure.configureForm)
	}
	configure.configureForm.RuntimeKeyMode = "manual"
	configure.configureForm.RuntimeAPIKey = "runtime-secret-draft"
	updated, _ = configure.Update(tunnelOperationMsg{command: TunnelManagedConfigure, targetID: "tunnel_one", err: fmt.Errorf("configure failed")})
	configure = updated.(*TunnelPage)
	view := ansi.Strip(configure.View(44, 18))
	if configure.configureForm == nil || configure.configureForm.RuntimeKeyMode != "manual" || configure.configureForm.RuntimeAPIKey != "runtime-secret-draft" || !strings.Contains(view, "configure failed") || strings.Contains(view, "runtime-secret-draft") {
		t.Fatalf("configure failure draft=%#v dirty=%t view=%q", configure.configureForm, configure.Dirty(), view)
	}
	testutil.AssertLinesFit(t, configure.View(44, 18), 44)
}

func setupTunnelPageConfig(t *testing.T, value tunnel.Config) {
	t.Helper()
	previous := configformat.RootPath()
	t.Cleanup(func() { _ = configformat.SetRootPath(previous) })
	root := filepath.Join(t.TempDir(), "config")
	if err := configformat.SetRootPath(root); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	cfg.Auth.MCPEnabled = false
	cfg.Auth.AdminEnabled = false
	cfg.Server.AllowUnauthenticatedLoopback = true
	if value.AdminKey != "" && !value.AdminReadAccess && !value.AdminManageAccess {
		value.AdminReadAccess, value.AdminManageAccess = true, true
	}
	cfg.Tunnel = value
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
}

func TestManagedTunnelReadOnlyAccessHidesManagementActions(t *testing.T) {
	setupTunnelPageConfig(t, tunnel.Config{AdminKey: "admin-read", AdminWorkspaceID: "ws_admin", AdminReadAccess: true})
	item := tunnel.Metadata{ID: "tunnel_one", Name: "One", Description: "read only"}
	if _, err := config.SaveTunnelMetadata(item); err != nil {
		t.Fatal(err)
	}
	page, err := NewManagedTunnels(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	view := ansi.Strip(page.View(100, 24))
	if !strings.Contains(view, "t attach") || !strings.Contains(view, "r refresh all") || !strings.Contains(view, "p admins") || strings.Contains(view, "a add") {
		t.Fatalf("read-only browser actions=%q", view)
	}
	detail, err := NewManagedTunnelsRoute(t.Context(), item.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := detail.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	detail = updated.(*TunnelPage)
	view = ansi.Strip(detail.View(100, 26))
	if !strings.Contains(view, "r refresh") || !strings.Contains(view, "t attach") || strings.Contains(view, "update") || strings.Contains(view, "delete") {
		t.Fatalf("read-only detail actions=%q", view)
	}
}

func TestTunnelAdminsPageListsProfilesAndOpensCreateEditor(t *testing.T) {
	admins := []tunnel.AdminConfig{{ID: "work", AdminKey: "admin-secret", WorkspaceID: "ws_admin", ManageAccess: true}}
	setupTunnelPageConfig(t, tunnel.Config{Admins: &admins})
	page, err := NewTunnelAdmins(t.Context(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	view := ansi.Strip(page.View(100, 28))
	for _, want := range []string{"Tunnel Admin Profiles", "work", "manage", "a add", "m managed"} {
		if !strings.Contains(view, want) {
			t.Fatalf("admin list missing %q: %q", want, view)
		}
	}
	if strings.Contains(view, "admin-secret") {
		t.Fatalf("admin list leaked secret: %q", view)
	}
	create, err := NewTunnelAdmins(t.Context(), "", "create")
	if err != nil {
		t.Fatal(err)
	}
	_ = create.Init()
	if create.editor == nil || create.form == nil || create.Dirty() {
		t.Fatalf("create editor=%v form=%v dirty=%t", create.editor != nil, create.form != nil, create.Dirty())
	}
	create.form.ID = "personal"
	create.form.AdminKey = "secret-draft"
	create.form.ScopeKind = "organization"
	create.form.ScopeID = "org_demo"
	view = ansi.Strip(create.View(80, 24))
	if !strings.Contains(view, "Profile ID") || strings.Contains(view, "secret-draft") {
		t.Fatalf("create editor view=%q", view)
	}
	testutil.AssertLinesFit(t, page.View(72, 24), 72)
}
