package page

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	mcpoauth "go.mewis.me/chatgpt-mcp/internal/oauth"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
)

type mcpPageClient struct {
	mu           sync.Mutex
	tools        []upstream.Tool
	blockTools   bool
	toolsStarted chan struct{}
	closed       []string
}

func (*mcpPageClient) Connect(context.Context, upstream.Server) error { return nil }
func (client *mcpPageClient) Close(_ context.Context, id string) error {
	client.mu.Lock()
	client.closed = append(client.closed, id)
	client.mu.Unlock()
	return nil
}
func (client *mcpPageClient) Tools(ctx context.Context, _ string) ([]upstream.Tool, error) {
	if client.blockTools {
		select {
		case client.toolsStarted <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	return append([]upstream.Tool(nil), client.tools...), nil
}
func (*mcpPageClient) Call(context.Context, string, string, map[string]any) (upstream.CallResult, error) {
	return upstream.CallResult{}, nil
}
func (*mcpPageClient) PID(string) int { return 4242 }

func newMCPPageTestHarness(t *testing.T, client *mcpPageClient) (*MCPPage, *upstream.Manager, *mcpoauth.Store, *upstream.Store) {
	t.Helper()
	root := t.TempDir()
	serverStore := upstream.NewStore(filepath.Join(root, "upstream.json"))
	manager := upstream.NewManagerWithClient(serverStore, client)
	if err := manager.Load(); err != nil {
		t.Fatal(err)
	}
	oauthStore := mcpoauth.NewStore(filepath.Join(root, "oauth.json"))
	page, err := newMCPPage(t.Context(), "", manager, oauthStore)
	if err != nil {
		t.Fatal(err)
	}
	return page, manager, oauthStore, serverStore
}

func TestMCPMutationNoticeRendersBesidePageTitle(t *testing.T) {
	page, _, _, _ := newMCPPageTestHarness(t, &mcpPageClient{})
	page.notice = "MCP server added"
	line := strings.Split(ansi.Strip(page.View(100, 24)), "\n")[0]
	if !strings.Contains(line, "Upstream MCP servers  · MCP server added") {
		t.Fatalf("MCP title notice=%q", line)
	}
}

func TestMCPResourceUsesRoutedChildDetailPage(t *testing.T) {
	client := &mcpPageClient{}
	_, manager, oauthStore, _ := newMCPPageTestHarness(t, client)
	if err := manager.Add(upstream.Server{ID: "docs", Name: "Docs", Enabled: true, Transport: "http", URL: "https://example.test/mcp", Auth: upstream.AuthConfig{Type: "oauth"}, Expose: "all"}); err != nil {
		t.Fatal(err)
	}
	page, err := newMCPRoutePage(t.Context(), "docs", "", manager, oauthStore)
	if err != nil {
		t.Fatal(err)
	}
	if page.OverlayActive() {
		t.Fatalf("resource detail state overlay=%t resource=%q", page.OverlayActive(), page.resourceID)
	}
	view := ansi.Strip(page.View(110, 28))
	for _, want := range []string{"MCP server · docs", "https://example.test/mcp", "h health", "v tools", "u oauth", "? more"} {
		if !strings.Contains(view, want) {
			t.Fatalf("MCP detail missing %q: %q", want, view)
		}
	}
	updated, _ := page.Update(tea.KeyPressMsg{Code: '?', Text: "?"})
	page = updated.(*MCPPage)
	view = ansi.Strip(page.View(110, 28))
	for _, want := range []string{"configure", "toggle", "health", "tools", "login", "less"} {
		if !strings.Contains(view, want) {
			t.Fatalf("expanded MCP detail missing %q: %q", want, view)
		}
	}
	if strings.Contains(view, "Overview   Health") || strings.Contains(view, "╭") {
		t.Fatalf("MCP detail retained tab/modal chrome: %q", view)
	}
	updated, action := page.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	page = updated.(*MCPPage)
	if action == nil {
		t.Fatal("remove detail action returned no command")
	}
	remove, ok := action().(MCPCommandMsg)
	if !ok || remove.Command != MCPServerRemove || remove.ResourceID != "docs" {
		t.Fatalf("remove action=%#v", remove)
	}
	updated, cmd := page.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	page = updated.(*MCPPage)
	if cmd == nil {
		t.Fatal("health child navigation returned no command")
	}
	navigate, ok := cmd().(NavigateMsg)
	if !ok || strings.Join(navigate.Path, "/") != "mcp/docs/health" {
		t.Fatalf("health navigation=%#v", navigate)
	}
	health, err := newMCPRoutePage(t.Context(), "docs", "health", manager, oauthStore)
	if err != nil {
		t.Fatal(err)
	}
	if got := ansi.Strip(health.View(110, 28)); !strings.Contains(got, "Not checked yet") || strings.Contains(got, "u oauth") || strings.Contains(got, "v tools") {
		t.Fatalf("health child=%q", got)
	}
}

func TestMCPPageServerLifecycleAndSecretRedaction(t *testing.T) {
	page, manager, _, store := newMCPPageTestHarness(t, &mcpPageClient{})
	if _, err := page.openCommand(MCPServerAdd, ""); err != nil {
		t.Fatal(err)
	}
	page.serverForm.ID = "docs"
	page.serverForm.Name = "Docs"
	page.serverForm.Transport = "http"
	page.serverForm.URL = "https://example.test/mcp"
	page.serverForm.Headers = "X-Mode=read"
	page.serverForm.SensitiveHeaders = `{"Authorization":"Bearer top-secret"}`
	page.serverForm.AuthType = "none"
	page.serverForm.Expose = "allowlist"
	page.serverForm.Tools = "read"
	page.serverForm.IdleTimeout = "30"
	navigate := page.submitServerForm()
	if navigate == nil {
		t.Fatal("server add returned no navigation command")
	}
	server, ok := manager.Get("docs")
	if !ok || server.Headers["Authorization"] != "Bearer top-secret" || server.Expose != "allowlist" {
		t.Fatalf("server=%#v ok=%t", server, ok)
	}
	data, err := os.ReadFile(store.Path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "top-secret") {
		t.Fatalf("secret leaked to upstream store: %s", data)
	}

	page.resourceID = "docs"
	if err := page.reload(); err != nil {
		t.Fatal(err)
	}
	view := page.View(120, 32)
	if strings.Contains(view, "top-secret") {
		t.Fatalf("secret leaked to TUI: %q", view)
	}
	if !strings.Contains(view, "<redacted>") {
		t.Fatalf("redaction marker missing from TUI: %q", view)
	}

	if _, err := page.openCommand(MCPServerConfigure, "docs"); err != nil {
		t.Fatal(err)
	}
	page.serverForm.Name = "Docs Renamed"
	page.serverForm.Expose = "all"
	page.submitServerForm()
	server, _ = manager.Get("docs")
	if server.Name != "Docs Renamed" || server.Expose != "all" || server.Headers["Authorization"] != "Bearer top-secret" {
		t.Fatalf("configured server=%#v", server)
	}

	if _, err := page.openCommand(MCPServerDisable, "docs"); err != nil {
		t.Fatal(err)
	}
	server, _ = manager.Get("docs")
	if server.Enabled {
		t.Fatal("server remained enabled")
	}
	if _, err := page.openCommand(MCPServerEnable, "docs"); err != nil {
		t.Fatal(err)
	}
	server, _ = manager.Get("docs")
	if !server.Enabled {
		t.Fatal("server remained disabled")
	}

	if _, err := page.openCommand(MCPServerRemove, "docs"); err != nil {
		t.Fatal(err)
	}
	page.confirm = component.NewConfirmButtons("Remove", "Cancel", true)
	page.updateConfirm(tea.KeyPressMsg{Code: tea.KeyEnter})
	if _, ok := manager.Get("docs"); ok {
		t.Fatal("server remained after confirmed removal")
	}
}

func TestMCPPageHealthAndToolsRunAsCommands(t *testing.T) {
	client := &mcpPageClient{tools: []upstream.Tool{{Name: "read", Description: "Read docs"}}}
	page, manager, _, _ := newMCPPageTestHarness(t, client)
	if err := manager.Add(upstream.Server{ID: "docs", Name: "Docs", Enabled: true, Transport: "http", URL: "https://example.test/mcp", Expose: "all"}); err != nil {
		t.Fatal(err)
	}
	if err := page.reload(); err != nil {
		t.Fatal(err)
	}
	cmd, err := page.openCommand(MCPServerHealth, "docs")
	if err != nil || cmd == nil || page.overlay != mcpOverlayOperation {
		t.Fatalf("health cmd=%v err=%v overlay=%d", cmd, err, page.overlay)
	}
	updated, _ := page.Update(cmd())
	page = updated.(*MCPPage)
	if page.status["docs"].Health != upstream.HealthConnected || page.status["docs"].ToolCount != 1 {
		t.Fatalf("health=%#v", page.status["docs"])
	}
	cmd, err = page.openCommand(MCPServerTools, "docs")
	if err != nil || cmd == nil {
		t.Fatalf("tools cmd=%v err=%v", cmd, err)
	}
	updated, _ = page.Update(cmd())
	page = updated.(*MCPPage)
	if len(page.tools["docs"]) != 1 || page.tools["docs"][0].Name != "read" {
		t.Fatalf("tools=%#v", page.tools["docs"])
	}
}

func TestMCPPageToolRefreshIsCancellable(t *testing.T) {
	client := &mcpPageClient{blockTools: true, toolsStarted: make(chan struct{}, 1)}
	page, manager, _, _ := newMCPPageTestHarness(t, client)
	if err := manager.Add(upstream.Server{ID: "slow", Enabled: true, Transport: "http", URL: "https://example.test/mcp", Expose: "all"}); err != nil {
		t.Fatal(err)
	}
	cmd, err := page.openCommand(MCPServerTools, "slow")
	if err != nil || cmd == nil {
		t.Fatalf("cmd=%v err=%v", cmd, err)
	}
	result := make(chan tea.Msg, 1)
	go func() { result <- cmd() }()
	select {
	case <-client.toolsStarted:
	case <-time.After(time.Second):
		t.Fatal("tool refresh did not start")
	}
	updated, _ := page.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	page = updated.(*MCPPage)
	if page.overlay != mcpOverlayNone || !page.operationCancelled {
		t.Fatalf("cancel state overlay=%d cancelled=%t", page.overlay, page.operationCancelled)
	}
	select {
	case message := <-result:
		updated, _ = page.Update(message)
		page = updated.(*MCPPage)
	case <-time.After(time.Second):
		t.Fatal("cancelled tool command did not return")
	}
	if page.operationCancelled || !strings.Contains(page.notice, "cancel") {
		t.Fatalf("cancel completion notice=%q cancelled=%t", page.notice, page.operationCancelled)
	}
}

func TestMCPPageOAuthEmitsURLStoresCredentialAndLogoutPreservesServer(t *testing.T) {
	client := &mcpPageClient{tools: []upstream.Tool{{Name: "read"}}}
	page, manager, oauthStore, _ := newMCPPageTestHarness(t, client)
	if err := manager.Add(upstream.Server{ID: "secure", Name: "Secure", Enabled: true, Transport: "http", URL: "https://example.test/mcp", Auth: upstream.AuthConfig{Type: "oauth", Scope: "read"}, Expose: "all"}); err != nil {
		t.Fatal(err)
	}
	opened := make(chan string, 1)
	page.openBrowser = func(raw string) error {
		opened <- raw
		return nil
	}
	page.oauthLogin = func(ctx context.Context, config mcpoauth.LoginConfig, options mcpoauth.LoginOptions) (mcpoauth.Credential, error) {
		if err := options.OnURL("https://auth.example/authorize"); err != nil {
			return mcpoauth.Credential{}, err
		}
		credential := mcpoauth.Credential{ServerID: config.ServerID, ServerURL: config.ServerURL, Issuer: "https://auth.example", ClientID: "client", Scopes: []string{"read"}, AccessToken: "access-secret", RefreshToken: "refresh-secret"}
		if err := oauthStore.Put(credential); err != nil {
			return mcpoauth.Credential{}, err
		}
		return credential, nil
	}
	if _, err := page.openCommand(MCPAuthLogin, "secure"); err != nil {
		t.Fatal(err)
	}
	page.oauthForm.OpenBrowser = true
	cmd := page.submitForm()
	if cmd == nil || page.overlay != mcpOverlayOperation {
		t.Fatalf("oauth cmd=%v overlay=%d", cmd, page.overlay)
	}
	updated, next := page.Update(cmd())
	page = updated.(*MCPPage)
	openedURL := ""
	select {
	case openedURL = <-opened:
	case <-time.After(time.Second):
		t.Fatal("browser opener was not called")
	}
	if page.operationURL != "https://auth.example/authorize" || openedURL != "https://auth.example/authorize" || next == nil {
		t.Fatalf("url=%q opened=%q next=%v", page.operationURL, openedURL, next)
	}
	updated, health := page.Update(next())
	page = updated.(*MCPPage)
	if health == nil {
		t.Fatal("OAuth success did not trigger health refresh")
	}
	if strings.Contains(page.View(120, 32), "access-secret") || strings.Contains(page.View(120, 32), "refresh-secret") {
		t.Fatal("OAuth token leaked into TUI")
	}
	updated, _ = page.Update(health())
	page = updated.(*MCPPage)
	status, err := oauthStore.Status("secure")
	if err != nil || !status.Configured || !status.HasRefreshToken {
		t.Fatalf("oauth status=%#v err=%v", status, err)
	}

	if _, err := page.openCommand(MCPAuthLogout, "secure"); err != nil {
		t.Fatal(err)
	}
	page.confirm = component.NewConfirmButtons("Logout", "Cancel", true)
	page.updateConfirm(tea.KeyPressMsg{Code: tea.KeyEnter})
	status, err = oauthStore.Status("secure")
	if err != nil || status.Configured {
		t.Fatalf("oauth remained after logout: %#v err=%v", status, err)
	}
	if _, ok := manager.Get("secure"); !ok {
		t.Fatal("OAuth logout removed MCP server configuration")
	}
}

func TestMCPPageOAuthBrowserFailureDoesNotAbortLogin(t *testing.T) {
	client := &mcpPageClient{tools: []upstream.Tool{}}
	page, manager, _, _ := newMCPPageTestHarness(t, client)
	if err := manager.Add(upstream.Server{ID: "secure", Enabled: true, Transport: "http", URL: "https://example.test/mcp", Auth: upstream.AuthConfig{Type: "oauth"}, Expose: "all"}); err != nil {
		t.Fatal(err)
	}
	page.openBrowser = func(string) error { return errors.New("no browser") }
	page.oauthLogin = func(_ context.Context, config mcpoauth.LoginConfig, options mcpoauth.LoginOptions) (mcpoauth.Credential, error) {
		_ = options.OnURL("https://auth.example/authorize")
		return mcpoauth.Credential{ServerID: config.ServerID}, nil
	}
	if _, err := page.openCommand(MCPAuthLogin, "secure"); err != nil {
		t.Fatal(err)
	}
	cmd := page.submitForm()
	updated, next := page.Update(cmd())
	page = updated.(*MCPPage)
	if next == nil {
		t.Fatal("missing next OAuth event")
	}
	updated, next = page.Update(next())
	page = updated.(*MCPPage)
	if !strings.Contains(page.notice, "no browser") || next == nil {
		t.Fatalf("browser failure notice=%q next=%v", page.notice, next)
	}
}
