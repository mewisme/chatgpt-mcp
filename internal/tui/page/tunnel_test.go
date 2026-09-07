package page

import (
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
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

func TestTunnelRuntimeFormsRedactSecretsAndBlankRuntimeKeyPreservesSecret(t *testing.T) {
	setupTunnelPageConfig(t, tunnel.Config{ID: "tunnel_demo", APIKey: "runtime-secret", AdminKey: "admin-secret", AdminWorkspaceID: "ws_admin"})
	page, err := NewTunnelDashboard(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if view := page.View(120, 32); strings.Contains(view, "runtime-secret") || strings.Contains(view, "admin-secret") {
		t.Fatalf("secret leaked in tunnel dashboard: %q", view)
	}
	if _, err := page.openCommand(TunnelConfigure, ""); err != nil {
		t.Fatal(err)
	}
	if page.runtimeForm == nil || page.runtimeForm.RuntimeAPIKey != "" {
		t.Fatalf("runtime form prefilled secret: %#v", page.runtimeForm)
	}
	if input := runtimeInputFromForm(page.runtimeForm); input.APIKey != nil {
		t.Fatalf("blank runtime key should preserve existing secret: %#v", input.APIKey)
	}
	if view := page.form.View(); strings.Contains(view, "runtime-secret") {
		t.Fatalf("runtime secret leaked in configure form: %q", view)
	}
	page.closeOverlay()
	if _, err := page.openCommand(TunnelAdminKeySet, ""); err != nil {
		t.Fatal(err)
	}
	if page.adminForm == nil || page.adminForm.AdminKey != "" || strings.Contains(page.form.View(), "admin-secret") {
		t.Fatalf("admin secret leaked in form: %#v view=%q", page.adminForm, page.form.View())
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

func TestManagedTunnelMutationNoticeRendersBesidePageTitle(t *testing.T) {
	setupTunnelPageConfig(t, tunnel.Config{})
	page, err := NewManagedTunnels(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	page.notice = "Managed tunnel created"
	line := strings.Split(ansi.Strip(page.View(100, 24)), "\n")[0]
	if !strings.Contains(line, "Managed tunnels  · Managed tunnel created") {
		t.Fatalf("managed tunnel title notice=%q", line)
	}
}

func TestManagedTunnelResourceUsesRoutedChildDetailPage(t *testing.T) {
	setupTunnelPageConfig(t, tunnel.Config{})
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
	for _, want := range []string{"Managed tunnel · tunnel_one", "primary", "s scope", "r refresh", "e update", "c configure", "d delete"} {
		if !strings.Contains(view, want) {
			t.Fatalf("managed detail missing %q: %q", want, view)
		}
	}
	if strings.Contains(view, "Overview   Scope") || strings.Contains(view, "╭") {
		t.Fatalf("managed detail retained tab/modal chrome: %q", view)
	}
	updated, cmd := page.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	page = updated.(*TunnelPage)
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
	if last != 31 || !strings.Contains(lines[last], "managed tunnels") {
		t.Fatalf("tunnel help line=%d want=31 view=%q", last, strings.Join(lines, "\n"))
	}
}

func TestTunnelConfigureSwitchAndEscapeFlow(t *testing.T) {
	setupTunnelPageConfig(t, tunnel.Config{Enabled: true, ID: "tunnel_demo", APIKey: "runtime-secret"})
	page, err := NewTunnelDashboard(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	cmd, err := page.openCommand(TunnelConfigure, "")
	if err != nil {
		t.Fatal(err)
	}
	page = runTunnelPageCmd(t, page, cmd)
	plain := ansi.Strip(page.form.View())
	if !strings.Contains(plain, "Enabled [ TRUE ]") || strings.Contains(plain, "[ FALSE ]") {
		t.Fatalf("configure switch view=%q", plain)
	}
	updated, _ := page.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	page = updated.(*TunnelPage)
	if page.runtimeForm.Enabled || !strings.Contains(ansi.Strip(page.form.View()), "Enabled [ FALSE ]") {
		t.Fatalf("space did not toggle enabled: %#v view=%q", page.runtimeForm, ansi.Strip(page.form.View()))
	}
	updated, next := page.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	before := page.runtimeForm.ID
	page = advanceTunnelFormAndType(t, updated.(*TunnelPage), next, 'x')
	if page.runtimeForm.ID == before {
		t.Fatalf("switch did not advance to tunnel id input: before=%q after=%q", before, page.runtimeForm.ID)
	}
	updated, next = page.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	page = updated.(*TunnelPage)
	if next != nil || !page.form.ConfirmingExit() || page.overlay != tunnelOverlayForm {
		t.Fatalf("dirty escape overlay=%d confirm=%t cmd=%v", page.overlay, page.form.ConfirmingExit(), next)
	}
	updated, _ = page.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	page = updated.(*TunnelPage)
	if page.form.ConfirmingExit() || page.overlay != tunnelOverlayForm {
		t.Fatalf("escape from discard confirmation overlay=%d confirm=%t", page.overlay, page.form.ConfirmingExit())
	}

	page.closeOverlay()
	cmd, err = page.openCommand(TunnelConfigure, "")
	if err != nil {
		t.Fatal(err)
	}
	page = runTunnelPageCmd(t, page, cmd)
	updated, next = page.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	page = updated.(*TunnelPage)
	page = runTunnelPageCmd(t, page, next)
	if page.overlay != tunnelOverlayNone {
		t.Fatalf("clean escape did not close configure dialog: overlay=%d", page.overlay)
	}
}

func TestTunnelRuntimeLayoutUsesHierarchyAndGroupWrapping(t *testing.T) {
	setupTunnelPageConfig(t, tunnel.Config{Enabled: true, ID: "tunnel_6a9462c95f008191a665c3330bcd8368", APIKey: "runtime-secret", AdminKey: "admin-secret", AdminOrganizationID: "org_demo"})
	page, err := NewTunnelDashboard(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	page.dashboard.Status.Metadata = &tunnel.Metadata{ID: page.dashboard.Config.ID, Name: "MCP_Tunnel_WSL", FetchedAt: time.Now()}
	wide := ansi.Strip(page.runtimeView(120))
	for _, want := range []string{"Status", "Tunnel", "Admin", "Metadata", "● ON", "Configured", "Runtime key", "MCP_Tunnel_WSL", "e configure", "space toggle", "d remove admin", "m managed tunnels"} {
		if !strings.Contains(wide, want) {
			t.Fatalf("wide tunnel layout missing %q: %q", want, wide)
		}
	}
	if strings.Contains(wide, "Enabled        true") || strings.Contains(wide, " · ") {
		t.Fatalf("wide tunnel layout retained raw boolean or dot-joined hints: %q", wide)
	}
	wideLines := strings.Split(wide, "\n")
	foundPair := false
	for _, line := range wideLines {
		if strings.Contains(line, "Status") && strings.Contains(line, "Tunnel") {
			foundPair = true
			break
		}
	}
	if !foundPair {
		t.Fatalf("wide layout did not place Status and Tunnel in two columns: %q", wideLines)
	}

	narrow := ansi.Strip(page.runtimeView(72))
	if !strings.Contains(narrow, "e configure") || !strings.Contains(narrow, "space toggle") {
		t.Fatalf("narrow default help missing core actions: %q", narrow)
	}
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

func TestManagedTunnelRefreshPersistsCacheAndUpdatePrefetchesRemoteState(t *testing.T) {
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
		case r.Method == http.MethodGet && r.URL.Path == "/v1/tunnels/tunnel_one":
			_, _ = w.Write([]byte(`{"id":"tunnel_one","name":"Remote One","description":"fresh","workspace_ids":["ws_admin"]}`))
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
	if err != nil || cmd == nil || !page.managedUpdateFetch {
		t.Fatalf("update prefetch cmd=%v err=%v fetch=%t", cmd, err, page.managedUpdateFetch)
	}
	updated, next := page.Update(cmd())
	page = updated.(*TunnelPage)
	if next == nil || page.overlay != tunnelOverlayForm || page.managedUpdateFetch || page.managedForm == nil || page.managedForm.Name != "Remote One" || page.managedForm.Description != "fresh" {
		t.Fatalf("overlay=%d fetch=%t form=%#v next=%v", page.overlay, page.managedUpdateFetch, page.managedForm, next)
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

func TestManagedTunnelDeleteSelectedRuntimeOffersClearConfigChoice(t *testing.T) {
	setupTunnelPageConfig(t, tunnel.Config{Enabled: true, ID: "tunnel_selected", APIKey: "runtime-secret", AdminKey: "admin-secret", AdminWorkspaceID: "ws_admin"})
	if _, err := config.SaveTunnelMetadata(tunnel.Metadata{ID: "tunnel_selected", Name: "Selected"}); err != nil {
		t.Fatal(err)
	}
	page, err := NewManagedTunnels(t.Context(), "tunnel_selected")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.openCommand(TunnelManagedDelete, "tunnel_selected"); err != nil {
		t.Fatal(err)
	}
	if page.overlay != tunnelOverlayForm || !page.deleteOptions || !page.deleteClear {
		t.Fatalf("delete options overlay=%d options=%t clear=%t", page.overlay, page.deleteOptions, page.deleteClear)
	}
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
	cfg.Tunnel = value
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
}

func runTunnelPageCmd(t *testing.T, page *TunnelPage, cmd tea.Cmd) *TunnelPage {
	t.Helper()
	if cmd == nil {
		return page
	}
	message := cmd()
	if batch, ok := message.(tea.BatchMsg); ok {
		for _, next := range batch {
			page = runTunnelPageCmd(t, page, next)
		}
		return page
	}
	updated, next := page.Update(message)
	value, ok := updated.(*TunnelPage)
	if !ok {
		t.Fatalf("tunnel page update returned %T", updated)
	}
	return runTunnelPageCmd(t, value, next)
}

func advanceTunnelFormAndType(t *testing.T, page *TunnelPage, cmd tea.Cmd, value rune) *TunnelPage {
	t.Helper()
	queue := []tea.Cmd{cmd}
	before := page.runtimeForm.ID
	for steps := 0; steps < 32 && len(queue) > 0; steps++ {
		next := queue[0]
		queue = queue[1:]
		if next == nil {
			continue
		}
		message := next()
		if batch, ok := message.(tea.BatchMsg); ok {
			queue = append(queue, batch...)
			continue
		}
		updated, follow := page.Update(message)
		updatedPage, ok := updated.(*TunnelPage)
		if !ok {
			t.Fatalf("tunnel page update returned %T", updated)
		}
		page = updatedPage
		typed, typedCmd := page.Update(tea.KeyPressMsg{Code: value, Text: string(value)})
		page = typed.(*TunnelPage)
		if page.runtimeForm.ID != before {
			return page
		}
		if typedCmd != nil {
			queue = append(queue, typedCmd)
		}
		if follow != nil {
			queue = append(queue, follow)
		}
	}
	return page
}
