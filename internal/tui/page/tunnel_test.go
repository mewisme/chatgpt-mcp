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
