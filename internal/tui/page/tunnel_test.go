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

	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
	"go.mewis.me/chatgpt-mcp/internal/tui/testutil"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
	securemcptunnel "go.mewis.me/chatgpt-mcp/plugins/secure-mcp-tunnel"
)

func TestTunnelCollectionRedactsSecretsAndBlankEditPreservesRuntimeKey(t *testing.T) {
	instances := []tunnel.InstanceConfig{{Enabled: true, ID: "tunnel_demo", APIKey: "runtime-secret", AdminProfileID: "work"}}
	admins := []tunnel.AdminConfig{{ID: "work", AdminKey: "admin-secret", WorkspaceID: "ws_admin", ManageAccess: true}}
	setupTunnelPageConfig(t, tunnel.Config{Instances: &instances, Admins: &admins})
	list, err := NewTunnelInstances(t.Context(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if view := list.View(120, 32); strings.Contains(view, "runtime-secret") || strings.Contains(view, "admin-secret") {
		t.Fatalf("secret leaked in tunnel collection: %q", view)
	}
	edit, err := NewTunnelInstances(t.Context(), "tunnel_demo", "edit")
	if err != nil {
		t.Fatal(err)
	}
	_ = edit.Init()
	if edit.OverlayActive() || edit.form == nil || edit.form.RuntimeAPIKey != "" {
		t.Fatalf("runtime editor overlay=%t draft=%#v", edit.OverlayActive(), edit.form)
	}
	instance, err := localInstanceFromForm(edit.form, "tunnel_demo")
	if err != nil || instance.APIKey != "" {
		t.Fatalf("blank runtime key should preserve existing secret: %#v err=%v", instance, err)
	}
	view := ansi.Strip(edit.View(100, 28))
	if strings.Contains(view, "runtime-secret") || !strings.Contains(view, "Blank keeps the current key") || !strings.Contains(view, "enter next") {
		t.Fatalf("runtime editor view=%q", view)
	}
	adminEditor, err := NewTunnelAdmins(t.Context(), "work", "edit")
	if err != nil {
		t.Fatal(err)
	}
	_ = adminEditor.Init()
	view = ansi.Strip(adminEditor.View(100, 28))
	if adminEditor.OverlayActive() || adminEditor.form == nil || adminEditor.form.AdminKey != "" || strings.Contains(view, "admin-secret") || !strings.Contains(view, "enter next") {
		t.Fatalf("admin editor overlay=%t draft=%#v view=%q", adminEditor.OverlayActive(), adminEditor.form, view)
	}
}

func TestTunnelCollectionTitleStartsAtWorkspaceTitlePosition(t *testing.T) {
	setupTunnelPageConfig(t, tunnel.Config{})
	page, err := NewTunnelInstances(t.Context(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(ansi.Strip(page.View(100, 32)), "\n")
	if len(lines) < 1 || !strings.Contains(lines[0], "OpenAI Secure MCP Tunnels") {
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

func TestManagedAndLocalTunnelRowsPreferLabels(t *testing.T) {
	admins := []tunnel.AdminConfig{
		{ID: "work", AdminKey: "admin-work", WorkspaceID: "ws_admin", ReadAccess: true, ManageAccess: true},
		{ID: "other", AdminKey: "admin-other", OrganizationID: "org_other", ReadAccess: true, ManageAccess: true},
	}
	instances := []tunnel.InstanceConfig{{Enabled: true, ID: "tunnel_one", APIKey: "runtime-one", AdminProfileID: "work"}, {ID: "tunnel_anon", APIKey: "runtime-anon"}}
	setupTunnelPageConfig(t, tunnel.Config{Instances: &instances, Admins: &admins})
	if _, err := config.SaveTunnelMetadata(tunnel.Metadata{ID: "tunnel_one", Name: "Alpha", Description: "prod"}); err != nil {
		t.Fatal(err)
	}
	if _, err := config.SaveTunnelMetadata(tunnel.Metadata{ID: "tunnel_two", Name: "Beta", Description: "staging"}); err != nil {
		t.Fatal(err)
	}
	managed, err := NewManagedTunnels(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	view := ansi.Strip(managed.View(100, 24))
	if !strings.Contains(view, "Alpha") || !strings.Contains(view, "Beta") || !strings.Contains(view, "prod") || !strings.Contains(view, "staging") {
		t.Fatalf("managed labels=%q", view)
	}
	local, err := NewTunnelInstances(t.Context(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	view = ansi.Strip(local.View(120, 32))
	if !strings.Contains(view, "Alpha") || !strings.Contains(view, "tunnel_anon") || strings.Contains(view, "runtime-one") {
		t.Fatalf("local labels=%q", view)
	}
}

func TestTunnelCollectionRowsDisambiguateDuplicateLabels(t *testing.T) {
	dupA, dupB := "tunnel_aaaaaaaaaaaaaaaaaaaaaaaac3330bcd", "tunnel_bbbbbbbbbbbbbbbbbbbbbbb3ce094ac"
	admins := []tunnel.AdminConfig{{ID: "work", AdminKey: "admin-work", ReadAccess: true, ManageAccess: true}}
	instances := []tunnel.InstanceConfig{{Enabled: true, ID: dupA, APIKey: "runtime-a"}, {Enabled: true, ID: dupB, APIKey: "runtime-b"}}
	setupTunnelPageConfig(t, tunnel.Config{Instances: &instances, Admins: &admins})
	if _, err := config.SaveTunnelMetadata(tunnel.Metadata{ID: dupA, Name: "Production"}); err != nil {
		t.Fatal(err)
	}
	if _, err := config.SaveTunnelMetadata(tunnel.Metadata{ID: dupB, Name: "Production"}); err != nil {
		t.Fatal(err)
	}
	local, err := NewTunnelInstances(t.Context(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	view := ansi.Strip(local.View(120, 32))
	if !strings.Contains(view, "Production · c3330bcd") || !strings.Contains(view, "Production · 3ce094ac") {
		t.Fatalf("local duplicates=%q", view)
	}
	if strings.Contains(view, dupA) || strings.Contains(view, dupB) {
		t.Fatalf("local leaked full ids=%q", view)
	}
	rows := (&TunnelPage{items: []tunnel.Metadata{{ID: dupA, Name: "Production"}, {ID: dupB, Name: "Production"}}}).managedRows()
	if len(rows) != 2 || rows[0].Title != "Production · c3330bcd" || rows[1].Title != "Production · 3ce094ac" {
		t.Fatalf("managed rows=%#v", rows)
	}
}

func TestManagedEditorsStayOnAttachedAdminProfile(t *testing.T) {
	admins := []tunnel.AdminConfig{
		{ID: "work", AdminKey: "admin-work", WorkspaceID: "ws_admin", ReadAccess: true, ManageAccess: true},
		{ID: "other", AdminKey: "admin-other", OrganizationID: "org_other", ReadAccess: true, ManageAccess: true},
	}
	instances := []tunnel.InstanceConfig{{Enabled: true, ID: "tunnel_one", APIKey: "runtime-one", AdminProfileID: "work"}}
	setupTunnelPageConfig(t, tunnel.Config{Instances: &instances, Admins: &admins})
	if _, err := config.SaveTunnelMetadata(tunnel.Metadata{ID: "tunnel_one", Name: "Alpha"}); err != nil {
		t.Fatal(err)
	}
	edit, err := NewManagedTunnelsRouteAction(t.Context(), "tunnel_one", "", "edit")
	if err != nil {
		t.Fatal(err)
	}
	if edit.managedForm == nil || edit.managedForm.AdminProfileID != "work" {
		t.Fatalf("edit profile=%#v", edit.managedForm)
	}
	if view := ansi.Strip(edit.View(80, 24)); strings.Contains(view, "other") {
		t.Fatalf("edit offered unrelated profile: %q", view)
	}
	configure, err := NewManagedTunnelsRouteAction(t.Context(), "tunnel_one", "", "configure")
	if err != nil {
		t.Fatal(err)
	}
	if configure.configureForm == nil || configure.configureForm.AdminProfileID != "work" {
		t.Fatalf("configure profile=%#v", configure.configureForm)
	}
	deletePage, err := NewManagedTunnelsRouteAction(t.Context(), "tunnel_one", "", "delete")
	if err != nil {
		t.Fatal(err)
	}
	if deletePage.deleteForm == nil || deletePage.deleteForm.AdminProfileID != "work" {
		t.Fatalf("delete profile=%#v", deletePage.deleteForm)
	}
	create, err := NewManagedTunnelsRouteAction(t.Context(), "", "", "create")
	if err != nil {
		t.Fatal(err)
	}
	if create.managedForm == nil || create.managedForm.AdminProfileID != "" {
		t.Fatalf("create should not pick a profile: %#v", create.managedForm)
	}
}

func TestManagedTunnelsForAdminScopesProfileAndAutoRefreshes(t *testing.T) {
	admins := []tunnel.AdminConfig{{ID: "work", AdminKey: "admin-secret", WorkspaceID: "ws_admin", ManageAccess: true, ReadAccess: true}}
	setupTunnelPageConfig(t, tunnel.Config{Admins: &admins})
	page, err := NewManagedTunnelsForAdmin(t.Context(), "work")
	if err != nil {
		t.Fatal(err)
	}
	if page.adminProfileID != "work" || len(page.adminProfiles) != 1 || page.adminProfiles[0].ID != "work" || page.pendingInit == nil {
		t.Fatalf("profile=%q admins=%d pending=%v", page.adminProfileID, len(page.adminProfiles), page.pendingInit != nil)
	}
	if view := ansi.Strip(page.View(100, 24)); !strings.Contains(view, "r refresh") || strings.Contains(view, "r refresh all") {
		t.Fatalf("scoped refresh help=%q", view)
	}
	cmd := page.Init()
	if cmd == nil || page.pendingInit != nil {
		t.Fatalf("init cmd=%v pending=%v", cmd != nil, page.pendingInit != nil)
	}
}

func TestManagedConfigureEditorOmitsEnableToggle(t *testing.T) {
	editor, data := newManagedConfigureEditor([]application.TunnelAdminProfile{{ID: "default", ReadAccess: true}})
	if data == nil || data.RuntimeKeyMode != "auto" {
		t.Fatalf("configure draft=%#v", data)
	}
	view := ansi.Strip(editor.View())
	if strings.Contains(view, "Enable after attach") || strings.Contains(view, "ENABLED") {
		t.Fatalf("configure form still asks to enable: %q", view)
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

func TestTunnelInstancesKeyHintsStayAtBottom(t *testing.T) {
	setupTunnelPageConfig(t, tunnel.Config{})
	page, err := NewTunnelInstances(t.Context(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(ansi.Strip(page.View(120, 32)), "\n")
	last := len(lines) - 1
	for last >= 0 && strings.TrimSpace(lines[last]) == "" {
		last--
	}
	if last < 0 || !strings.Contains(strings.Join(lines[max(0, last-2):], "\n"), "m managed") {
		t.Fatalf("tunnel help line=%d view=%q", last, strings.Join(lines, "\n"))
	}
}

func TestTunnelInstancesDetailUsesIDScopedActionsAndRedactsSecrets(t *testing.T) {
	instances := []tunnel.InstanceConfig{{Enabled: true, ID: "tunnel_demo", APIKey: "runtime-secret", AdminProfileID: "work", OrganizationID: "org_demo"}}
	admins := []tunnel.AdminConfig{{ID: "work", AdminKey: "admin-secret", OrganizationID: "org_demo"}}
	setupTunnelPageConfig(t, tunnel.Config{Instances: &instances, Admins: &admins})
	page, err := NewTunnelInstances(t.Context(), "tunnel_demo", "")
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := page.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	page = updated.(*TunnelInstancesPage)
	plain := ansi.Strip(page.View(100, 28))
	for _, want := range []string{"tunnel_demo", "Runtime key", "configured", "Admin profile", "work", "Organization", "org_demo", "disable", "start", "run", "edit", "detach"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("detail missing %q: %q", want, plain)
		}
	}
	if strings.Contains(plain, "runtime-secret") || strings.Contains(plain, "admin-secret") {
		t.Fatalf("tunnel secret leaked in detail: %q", plain)
	}
}

func TestLocalTunnelEditorPasswordLabelAlignsWithOtherFields(t *testing.T) {
	setupTunnelPageConfig(t, tunnel.Config{})
	page, err := NewTunnelInstances(t.Context(), "", "create")
	if err != nil {
		t.Fatal(err)
	}
	_ = page.Init()
	plain := ansi.Strip(page.View(100, 30))
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

func TestTunnelAdminProfileEditorFailureKeepsDraft(t *testing.T) {
	setupTunnelPageConfig(t, tunnel.Config{})
	page, err := NewTunnelAdmins(t.Context(), "", "create")
	if err != nil {
		t.Fatal(err)
	}
	_ = page.Init()
	page.form.ID = "work"
	page.form.AdminKey = "secret-draft"
	page.form.ScopeKind = "workspace"
	page.form.ScopeID = "ws_admin"
	if !page.Dirty() {
		t.Fatalf("admin draft=%#v dirty=%t", page.form, page.Dirty())
	}
	page.editor.SetSubmitting(true)
	updated, cmd := page.Update(tunnelAdminResultMsg{command: TunnelAdminAdd, err: fmt.Errorf("verification failed")})
	page = updated.(*TunnelAdminsPage)
	msg, ok := cmd().(OperationMsg)
	if !ok || msg.Phase != OperationError || !strings.Contains(msg.Message, "verification failed") {
		t.Fatalf("operation error=%#v", msg)
	}
	if page.OverlayActive() || page.form == nil || page.form.AdminKey != "secret-draft" || !page.Dirty() {
		t.Fatalf("admin failure lost draft overlay=%t draft=%#v dirty=%t", page.OverlayActive(), page.form, page.Dirty())
	}
	if strings.Contains(ansi.Strip(page.View(90, 26)), "secret-draft") {
		t.Fatalf("admin failure leaked secret")
	}
}

func TestTunnelInstancesLayoutShowsCollectionSummaryAndManagedShortcut(t *testing.T) {
	instances := []tunnel.InstanceConfig{{Enabled: true, ID: "tunnel_a", APIKey: "runtime-a", AdminProfileID: "work"}, {ID: "tunnel_b", APIKey: "runtime-b"}}
	admins := []tunnel.AdminConfig{{ID: "work", AdminKey: "admin-secret"}}
	setupTunnelPageConfig(t, tunnel.Config{Instances: &instances, Admins: &admins})
	page, err := NewTunnelInstances(t.Context(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	view := ansi.Strip(page.View(120, 32))
	for _, want := range []string{"OpenAI Secure MCP Tunnels", "2 attached", "1 admin profiles", "tunnel_a", "tunnel_b", "n attach", "a admins", "m managed"} {
		if !strings.Contains(view, want) {
			t.Fatalf("collection view missing %q: %q", want, view)
		}
	}
	if strings.Contains(view, "runtime-a") || strings.Contains(view, "runtime-b") || strings.Contains(view, "admin-secret") {
		t.Fatalf("collection view leaked secrets: %q", view)
	}
	testutil.AssertLinesFit(t, page.View(72, 24), 72)
}

func TestLocalTunnelEditorsAttachAndPreserveBlankRuntimeKey(t *testing.T) {
	instances := []tunnel.InstanceConfig{
		{Enabled: true, ID: "tunnel_one", APIKey: "runtime-one", OrganizationID: "org_old"},
		{Enabled: true, ID: "tunnel_two", APIKey: "runtime-two"},
	}
	admins := []tunnel.AdminConfig{}
	setupTunnelPageConfig(t, tunnel.Config{Instances: &instances, Admins: &admins})
	create, err := NewTunnelInstances(t.Context(), "", "create")
	if err != nil {
		t.Fatal(err)
	}
	_ = create.Init()
	view := ansi.Strip(create.View(100, 28))
	if !create.InputActive() || create.form == nil || !strings.Contains(view, "Attach a tunnel using an existing runtime API key") || strings.Contains(view, "runtime-one") {
		t.Fatalf("create editor input=%t form=%#v view=%q", create.InputActive(), create.form, view)
	}
	create.form.ID = "tunnel_three"
	create.form.RuntimeAPIKey = "runtime-three"
	create.form.Enabled = true
	instance, err := localInstanceFromForm(create.form, "")
	if err != nil {
		t.Fatal(err)
	}
	item, err := application.AttachLocalTunnel(t.Context(), instance)
	if err != nil {
		t.Fatal(err)
	}
	create.editor.SetSubmitting(true)
	if !create.Dirty() || !create.Submitting() {
		t.Fatalf("precondition dirty=%t submitting=%t", create.Dirty(), create.Submitting())
	}
	cmd := create.finishCommand(localTunnelResultMsg{command: LocalTunnelAdd, id: item.ID, item: item})
	if cmd == nil || create.Dirty() || create.Submitting() {
		t.Fatalf("attach success dirty=%t submitting=%t cmd=%v", create.Dirty(), create.Submitting(), cmd != nil)
	}

	edit, err := NewTunnelInstances(t.Context(), "tunnel_one", "edit")
	if err != nil {
		t.Fatal(err)
	}
	_ = edit.Init()
	if edit.form == nil || edit.form.RuntimeAPIKey != "" || edit.form.ID != "tunnel_one" {
		t.Fatalf("edit draft=%#v", edit.form)
	}
	view = ansi.Strip(edit.View(100, 28))
	if strings.Contains(view, "runtime-one") || strings.Contains(view, "runtime-two") || !strings.Contains(view, "Blank keeps the current key") {
		t.Fatalf("edit editor view=%q", view)
	}
	edit.form.OrganizationID = "org_new"
	instance, err = localInstanceFromForm(edit.form, "tunnel_one")
	if err != nil {
		t.Fatal(err)
	}
	item, err = application.UpdateLocalTunnel(t.Context(), instance)
	if err != nil {
		t.Fatal(err)
	}
	edit.editor.SetSubmitting(true)
	cmd = edit.finishCommand(localTunnelResultMsg{command: LocalTunnelUpdate, id: item.ID, item: item})
	if cmd == nil || edit.Dirty() || edit.Submitting() {
		t.Fatalf("edit success dirty=%t submitting=%t cmd=%v", edit.Dirty(), edit.Submitting(), cmd != nil)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	got := loaded.RuntimeTunnels().Instances
	if len(got) != 3 || got[0].ID != "tunnel_one" || got[0].APIKey != "runtime-one" || got[0].OrganizationID != "org_new" || got[1].ID != "tunnel_two" || got[1].APIKey != "runtime-two" || got[2].ID != "tunnel_three" || got[2].APIKey != "runtime-three" {
		t.Fatalf("instances=%#v", got)
	}
}

func TestTunnelInstancesHelpUsesDefaultStyle(t *testing.T) {
	instances := []tunnel.InstanceConfig{{Enabled: true, ID: "tunnel_demo", APIKey: "runtime-secret"}}
	setupTunnelPageConfig(t, tunnel.Config{Instances: &instances})
	page, err := NewTunnelInstances(t.Context(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	view := ansi.Strip(page.View(120, 32))
	if !strings.Contains(view, "n attach") || !strings.Contains(view, "m managed") {
		t.Fatalf("tunnel help missing collection actions: %q", view)
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
	if err != nil || cmd == nil {
		t.Fatalf("refresh cmd=%v err=%v", cmd, err)
	}
	if op, ok := operationMsg(cmd); !ok || op.Phase != OperationPending {
		t.Fatalf("refresh pending=%#v", op)
	}
	updated, follow := page.Update(workMsg(cmd))
	page = updated.(*TunnelPage)
	if len(page.items) != 2 || page.items[0].ID != "tunnel_one" {
		t.Fatalf("items=%#v", page.items)
	}
	if op, ok := operationMsg(follow); !ok || !strings.Contains(op.Message, "2") {
		t.Fatalf("refresh op=%#v", op)
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
	if edit.editor == nil || edit.overlay != tunnelOverlayNone || edit.managedForm == nil || edit.managedForm.Name != "Cached One" || edit.managedForm.Description != "first" {
		t.Fatalf("cached editor=%v overlay=%d form=%#v", edit.editor != nil, edit.overlay, edit.managedForm)
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
	go func() { result <- workMsg(cmd) }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("managed refresh did not start")
	}
	page.cancelOperation()
	if !page.operationCancelled {
		t.Fatal("managed refresh was not cancelled")
	}
	select {
	case message := <-result:
		updated, follow := page.Update(message)
		page = updated.(*TunnelPage)
		if page.operationCancelled || page.err != nil {
			t.Fatalf("completion cancelled=%t err=%v", page.operationCancelled, page.err)
		}
		if op, ok := operationMsg(follow); !ok || op.Phase != OperationCancelled {
			t.Fatalf("cancel operation=%#v", op)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled managed refresh did not return")
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
	updated, cmd := page.Update(tunnelOperationMsg{command: TunnelManagedCreate, err: fmt.Errorf("create failed")})
	page = updated.(*TunnelPage)
	msg, ok := cmd().(OperationMsg)
	if !ok || msg.Phase != OperationError || !strings.Contains(msg.Message, "create failed") {
		t.Fatalf("create failure operation=%#v", msg)
	}
	if page.editor == nil || page.managedForm == nil || page.managedForm.Name != "draft-name" || !page.Dirty() || page.OverlayActive() {
		t.Fatalf("create failure editor=%v draft=%#v dirty=%t overlay=%t", page.editor != nil, page.managedForm, page.Dirty(), page.OverlayActive())
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
	updated, cmd := edit.Update(tunnelOperationMsg{command: TunnelManagedUpdate, targetID: "tunnel_one", err: fmt.Errorf("update failed")})
	edit = updated.(*TunnelPage)
	msg, ok := cmd().(OperationMsg)
	if !ok || msg.Phase != OperationError || edit.managedForm == nil || edit.managedForm.Name != name || !edit.Dirty() {
		t.Fatalf("update failure draft=%#v dirty=%t msg=%#v", edit.managedForm, edit.Dirty(), msg)
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
	updated, cmd = configure.Update(tunnelOperationMsg{command: TunnelManagedConfigure, targetID: "tunnel_one", err: fmt.Errorf("configure failed")})
	configure = updated.(*TunnelPage)
	msg, ok = cmd().(OperationMsg)
	view := ansi.Strip(configure.View(44, 18))
	if !ok || msg.Phase != OperationError || configure.configureForm == nil || configure.configureForm.RuntimeKeyMode != "manual" || configure.configureForm.RuntimeAPIKey != "runtime-secret-draft" || strings.Contains(view, "runtime-secret-draft") {
		t.Fatalf("configure failure draft=%#v dirty=%t view=%q msg=%#v", configure.configureForm, configure.Dirty(), view, msg)
	}
	testutil.AssertLinesFit(t, configure.View(44, 18), 44)
}

func setupTunnelPageConfig(t *testing.T, value tunnel.Config) {
	t.Helper()
	tunnel.SetAdminBackend(securemcptunnel.ControlPlane())
	t.Cleanup(func() { tunnel.SetAdminBackend(nil) })
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

func TestTunnelAdminProfileEditorRejectsSpacedID(t *testing.T) {
	editor, _ := newTunnelAdminProfileEditor(application.TunnelAdminProfile{ID: "my profile"}, true)
	_ = editor.Init()
	if err := editor.Validate(); err == nil || !strings.Contains(err.Error(), "letters, numbers") {
		t.Fatalf("err=%v", err)
	}
	editor, _ = newTunnelAdminProfileEditor(application.TunnelAdminProfile{ID: "personal"}, true)
	_ = editor.Init()
	if err := editor.Validate(); err == nil || !strings.Contains(err.Error(), "admin API key") {
		t.Fatalf("valid id still needs key, err=%v", err)
	}
}

func TestTunnelAdminsPageCreateSuccessClearsDirtyBeforeNavigate(t *testing.T) {
	setupTunnelPageConfig(t, tunnel.Config{})
	page, err := NewTunnelAdmins(t.Context(), "", "create")
	if err != nil {
		t.Fatal(err)
	}
	_ = page.Init()
	page.form.ID = "personal"
	page.form.AdminKey = "secret-draft"
	page.form.ScopeKind = "organization"
	page.form.ScopeID = "org_demo"
	page.editor.SetSubmitting(true)
	if !page.Dirty() || !page.Submitting() {
		t.Fatalf("precondition dirty=%t submitting=%t", page.Dirty(), page.Submitting())
	}
	cmd := page.finishCommand(tunnelAdminResultMsg{
		command: TunnelAdminAdd,
		item:    application.TunnelAdminProfile{ID: "personal", OrganizationID: "org_demo", ManageAccess: true, KeyConfigured: true},
		count:   2,
	})
	if cmd == nil || page.Dirty() || page.Submitting() || page.editor == nil || page.form == nil {
		t.Fatalf("success left bad state cmd=%v dirty=%t submitting=%t editor=%v form=%v", cmd != nil, page.Dirty(), page.Submitting(), page.editor != nil, page.form != nil)
	}
	view := ansi.Strip(page.View(80, 24))
	if strings.Contains(view, "panic") || view == "" {
		t.Fatalf("post-success view=%q", view)
	}
	if op, ok := operationMsg(cmd); !ok || !strings.Contains(op.Message, "personal") || !strings.Contains(op.Message, "verified") {
		t.Fatalf("operation=%#v", op)
	}
}

func TestTunnelAdminsPageCreateFailureKeepsDirtyDraft(t *testing.T) {
	setupTunnelPageConfig(t, tunnel.Config{})
	page, err := NewTunnelAdmins(t.Context(), "", "create")
	if err != nil {
		t.Fatal(err)
	}
	_ = page.Init()
	page.form.ID = "personal"
	page.form.AdminKey = "secret-draft"
	page.form.ScopeKind = "organization"
	page.form.ScopeID = "org_demo"
	page.editor.SetSubmitting(true)
	cmd := page.finishCommand(tunnelAdminResultMsg{command: TunnelAdminAdd, err: fmt.Errorf("verification failed")})
	msg, ok := cmd().(OperationMsg)
	if !ok || msg.Phase != OperationError || page.editor == nil || page.form == nil || page.form.AdminKey != "secret-draft" || !page.Dirty() || page.Submitting() {
		t.Fatalf("failure lost draft cmd=%v editor=%v draft=%#v dirty=%t submitting=%t", cmd != nil, page.editor != nil, page.form, page.Dirty(), page.Submitting())
	}
}

func TestTunnelAdminsPageDetailVerifyRemoveAndUpdateSuccess(t *testing.T) {
	admins := []tunnel.AdminConfig{{ID: "work", AdminKey: "admin-secret", WorkspaceID: "ws_admin", ManageAccess: true, ReadAccess: true}}
	setupTunnelPageConfig(t, tunnel.Config{Admins: &admins})
	page, err := NewTunnelAdmins(t.Context(), "work", "")
	if err != nil {
		t.Fatal(err)
	}
	if page.InputActive() || page.OverlayActive() || page.Notice() != "" {
		t.Fatalf("detail input=%t overlay=%t notice=%q", page.InputActive(), page.OverlayActive(), page.Notice())
	}
	view := ansi.Strip(page.View(100, 26))
	for _, want := range []string{"work", "manage", "workspace:ws_admin", "e edit", "v verify", "d remove"} {
		if !strings.Contains(view, want) {
			t.Fatalf("detail missing %q: %q", want, view)
		}
	}
	page.SetNotice("ready")
	if page.Notice() != "ready" {
		t.Fatalf("notice=%q", page.Notice())
	}
	updated, cmd := page.Update(TunnelAdminCommandMsg{Command: TunnelAdminVerify, ResourceID: "work"})
	page = updated.(*TunnelAdminsPage)
	if cmd == nil {
		t.Fatal("verify command missing")
	}
	updated, follow := page.Update(tunnelAdminResultMsg{
		command: TunnelAdminVerify,
		item:    application.TunnelAdminProfile{ID: "work", WorkspaceID: "ws_admin", ManageAccess: true, ReadAccess: true, KeyConfigured: true},
		count:   3,
	})
	page = updated.(*TunnelAdminsPage)
	if op, ok := operationMsg(follow); !ok || !strings.Contains(op.Message, "Verified work") || page.Dirty() {
		t.Fatalf("verify op=%#v dirty=%t", op, page.Dirty())
	}
	updated, cmd = page.Update(TunnelAdminCommandMsg{Command: TunnelAdminRemove, ResourceID: "work"})
	page = updated.(*TunnelAdminsPage)
	if cmd != nil || !page.OverlayActive() || page.confirmID != "work" {
		t.Fatalf("remove confirm cmd=%v overlay=%t id=%q", cmd != nil, page.OverlayActive(), page.confirmID)
	}
	updated, _ = page.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	page = updated.(*TunnelAdminsPage)
	if page.OverlayActive() || page.confirmID != "" {
		t.Fatalf("esc did not clear confirm overlay=%t id=%q", page.OverlayActive(), page.confirmID)
	}
	edit, err := NewTunnelAdmins(t.Context(), "work", "edit")
	if err != nil {
		t.Fatal(err)
	}
	_ = edit.Init()
	if !edit.InputActive() || edit.form == nil || edit.form.ScopeKind != "workspace" {
		t.Fatalf("edit input=%t form=%#v", edit.InputActive(), edit.form)
	}
	edit.form.ScopeID = "ws_updated"
	edit.editor.SetSubmitting(true)
	cmd = edit.finishCommand(tunnelAdminResultMsg{
		command: TunnelAdminUpdate,
		item:    application.TunnelAdminProfile{ID: "work", WorkspaceID: "ws_updated", ManageAccess: true, KeyConfigured: true},
		count:   1,
	})
	if cmd == nil || edit.Dirty() || edit.Submitting() || edit.editor == nil {
		t.Fatalf("update success cmd=%v dirty=%t submitting=%t editor=%v", cmd != nil, edit.Dirty(), edit.Submitting(), edit.editor != nil)
	}
	if view := ansi.Strip(edit.View(80, 24)); view == "" {
		t.Fatal("update success view empty")
	}
	_, cancel := edit.Update(component.EditorCancelMsg{})
	nav, ok := cancel().(NavigateMsg)
	if !ok || strings.Join(nav.Path, "/") != "admins/work" {
		t.Fatalf("editor cancel navigation=%#v", cancel)
	}
}

func TestTunnelAdminsPageRefreshRemoveFinishAndMouseTargets(t *testing.T) {
	admins := []tunnel.AdminConfig{
		{ID: "work", AdminKey: "admin-secret", OrganizationID: "org_demo", ManageAccess: true, ReadAccess: true},
		{ID: "read", AdminKey: "admin-secret", TenantID: "ten_demo", ReadAccess: true},
		{ID: "raw", AdminKey: "admin-secret", WorkspaceID: "ws_demo"},
	}
	setupTunnelPageConfig(t, tunnel.Config{Admins: &admins})
	page, err := NewTunnelAdmins(t.Context(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	_ = page.View(90, 24)
	targets := page.MouseTargets(0, 0, 1)
	if len(targets) == 0 {
		t.Fatal("list mouse targets empty")
	}
	updated, cmd := page.Update(tea.KeyPressMsg{Text: "r", Code: 'r'})
	page = updated.(*TunnelAdminsPage)
	if cmd == nil {
		t.Fatal("refresh command missing")
	}
	updated, follow := page.Update(tunnelAdminResultMsg{command: TunnelAdminRefresh, items: []application.TunnelAdminProfile{
		{ID: "work", OrganizationID: "org_demo", ManageAccess: true, KeyConfigured: true},
		{ID: "read", TenantID: "ten_demo", ReadAccess: true, KeyConfigured: true},
		{ID: "raw", WorkspaceID: "ws_demo", KeyConfigured: true},
	}})
	page = updated.(*TunnelAdminsPage)
	if op, ok := operationMsg(follow); !ok || !strings.Contains(op.Message, "Refreshed 3") {
		t.Fatalf("refresh op=%#v", op)
	}
	view := ansi.Strip(page.View(100, 28))
	for _, want := range []string{"work", "read", "raw", "manage", "read", "unverified", "organization:org_demo", "tenant:ten_demo", "workspace:ws_demo"} {
		if !strings.Contains(view, want) {
			t.Fatalf("refresh list missing %q: %q", want, view)
		}
	}
	page.err = fmt.Errorf("boom")
	if !strings.Contains(ansi.Strip(page.View(80, 20)), "boom") {
		t.Fatal("list feedback missing error")
	}
	updated, _ = page.Update(TunnelAdminCommandMsg{Command: TunnelAdminRemove, ResourceID: "raw"})
	page = updated.(*TunnelAdminsPage)
	if !page.OverlayActive() || len(page.MouseTargets(0, 0, 1)) == 0 {
		t.Fatalf("remove overlay=%t targets=%d", page.OverlayActive(), len(page.MouseTargets(0, 0, 1)))
	}
	page.confirm.Select(true)
	updated, cmd = page.Update(component.ConfirmChoiceMsg{Affirmative: true})
	page = updated.(*TunnelAdminsPage)
	if cmd == nil || page.OverlayActive() {
		t.Fatalf("affirm remove cmd=%v overlay=%t", cmd != nil, page.OverlayActive())
	}
	updated, follow = page.Update(tunnelAdminResultMsg{command: TunnelAdminRemove, id: "raw"})
	page = updated.(*TunnelAdminsPage)
	if follow == nil {
		t.Fatal("list remove missing operation result")
	}
	if _, ok := follow().(NavigateMsg); ok {
		t.Fatal("list remove unexpectedly navigated")
	}
	if op, ok := operationMsg(follow); !ok || op.Message != "Admin profile removed" || len(page.items) != 2 {
		t.Fatalf("remove finish op=%#v items=%d", op, len(page.items))
	}
	detail, err := NewTunnelAdmins(t.Context(), "work", "")
	if err != nil {
		t.Fatal(err)
	}
	updated, follow = detail.Update(tunnelAdminResultMsg{command: TunnelAdminRemove, id: "work"})
	detail = updated.(*TunnelAdminsPage)
	nav, ok := navigateMsg(follow)
	if !ok || strings.Join(nav.Path, "/") != "admins" || !nav.Replace {
		t.Fatalf("detail remove navigation=%#v", nav)
	}
	_ = detail
}

func TestTunnelAdminsPageSubmitValidationAndWindowResize(t *testing.T) {
	setupTunnelPageConfig(t, tunnel.Config{})
	page, err := NewTunnelAdmins(t.Context(), "", "create")
	if err != nil {
		t.Fatal(err)
	}
	_ = page.Init()
	if cmd := page.submitEditor(); cmd != nil || page.editor == nil {
		t.Fatalf("empty create submit cmd=%v", cmd != nil)
	}
	page.form.ID = "personal"
	page.form.AdminKey = ""
	page.form.ScopeKind = "organization"
	page.form.ScopeID = "org_demo"
	if cmd := page.submitEditor(); cmd != nil {
		t.Fatal("blank key create unexpectedly submitted")
	}
	page.form.AdminKey = "secret"
	page.form.ScopeKind = "nope"
	if cmd := page.submitEditor(); cmd != nil {
		t.Fatal("invalid scope create unexpectedly submitted")
	}
	page.form.ScopeKind = "organization"
	updated, _ := page.Update(tea.WindowSizeMsg{Width: 90, Height: 28})
	page = updated.(*TunnelAdminsPage)
	if page.width != 90 || page.height != 28 {
		t.Fatalf("editor resize size=%dx%d", page.width, page.height)
	}
	admins := []tunnel.AdminConfig{{ID: "work", AdminKey: "admin-secret", WorkspaceID: "ws_admin", ManageAccess: true}}
	setupTunnelPageConfig(t, tunnel.Config{Admins: &admins})
	list, err := NewTunnelAdmins(t.Context(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	updated, _ = list.Update(tea.WindowSizeMsg{Width: 88, Height: 26})
	list = updated.(*TunnelAdminsPage)
	if list.width != 88 || list.height != 26 {
		t.Fatalf("list resize size=%dx%d", list.width, list.height)
	}
	updated, cmd := list.Update(TunnelAdminCommandMsg{Command: TunnelAdminUpdate, ResourceID: "work"})
	list = updated.(*TunnelAdminsPage)
	nav, ok := cmd().(NavigateMsg)
	if !ok || strings.Join(nav.Path, "/") != "admins/work/edit" {
		t.Fatalf("update navigation=%#v", cmd)
	}
	updated, cmd = list.Update(tea.KeyPressMsg{Text: "a", Code: 'a'})
	nav, ok = cmd().(NavigateMsg)
	if !ok || strings.Join(nav.Path, "/") != "admins/create" {
		t.Fatalf("add key navigation=%#v", cmd)
	}
	_, cmd = list.Update(tea.KeyPressMsg{Text: "m", Code: 'm'})
	nav, ok = cmd().(NavigateMsg)
	if !ok || strings.Join(nav.Path, "/") != "admins/work/managed" {
		t.Fatalf("managed key navigation=%#v", cmd)
	}
	_ = updated
}
