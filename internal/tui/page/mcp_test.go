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
	"charm.land/huh/v2"
	"github.com/charmbracelet/x/ansi"
	mcpoauth "go.mewis.me/chatgpt-mcp/internal/oauth"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
	"go.mewis.me/chatgpt-mcp/internal/tui/testutil"
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
	_, cmd := page.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
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

func TestMCPRoutedServerEditorsAndSecretRedaction(t *testing.T) {
	_, manager, oauthStore, store := newMCPPageTestHarness(t, &mcpPageClient{})
	create, err := newMCPRoutePageAction(t.Context(), "", "", "create", manager, oauthStore)
	if err != nil {
		t.Fatal(err)
	}
	_ = create.Init()
	updated, _ := create.Update(tea.WindowSizeMsg{Width: 100, Height: 28})
	create = updated.(*MCPPage)
	if create.editor == nil || create.OverlayActive() || !create.InputActive() || create.Dirty() {
		t.Fatalf("create editor=%v overlay=%t input=%t dirty=%t", create.editor != nil, create.OverlayActive(), create.InputActive(), create.Dirty())
	}
	view := ansi.Strip(create.View(100, 28))
	for _, want := range []string{"Create MCP Server", "General", "Connection", "Authentication", "Tools", "ctrl+s create"} {
		if !strings.Contains(view, want) {
			t.Fatalf("create editor missing %q: %q", want, view)
		}
	}
	testutil.AssertLinesFit(t, create.View(40, 18), 40)
	updated, _ = create.Update(tea.KeyPressMsg{Code: 'd', Text: "docs"})
	create = updated.(*MCPPage)
	updated, _ = create.Update(huh.NextField())
	create = updated.(*MCPPage)
	updated, _ = create.Update(tea.KeyPressMsg{Code: 'D', Text: "Docs"})
	create = updated.(*MCPPage)
	updated, _ = create.Update(huh.NextField())
	create = updated.(*MCPPage)
	updated, _ = create.Update(huh.NextField())
	create = updated.(*MCPPage)
	updated, _ = create.Update(huh.NextField())
	create = updated.(*MCPPage)
	updated, _ = create.Update(tea.KeyPressMsg{Code: 'h', Text: "https://example.test/mcp"})
	create = updated.(*MCPPage)
	if create.serverForm.ID != "docs" || create.serverForm.Transport != "http" || create.serverForm.URL != "https://example.test/mcp" || !create.Dirty() {
		t.Fatalf("create draft=%#v dirty=%t", create.serverForm, create.Dirty())
	}
	updated, submit := create.Update(component.EditorSubmitMsg{})
	create = updated.(*MCPPage)
	if submit == nil || create.Dirty() {
		t.Fatalf("create submit=%v dirty=%t", submit != nil, create.Dirty())
	}
	server, ok := manager.Get("docs")
	if !ok || server.Transport != "http" || server.URL != "https://example.test/mcp" || server.Name != "Docs" {
		t.Fatalf("created server=%#v ok=%t", server, ok)
	}

	server.Headers = map[string]string{"Authorization": "Bearer top-secret", "X-Mode": "read"}
	server.Expose = "allowlist"
	server.Tools = []string{"read"}
	if err := manager.Add(server); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(store.Path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "top-secret") {
		t.Fatalf("secret leaked to upstream store: %s", data)
	}
	edit, err := newMCPRoutePageAction(t.Context(), "docs", "", "edit", manager, oauthStore)
	if err != nil {
		t.Fatal(err)
	}
	_ = edit.Init()
	editView := edit.View(120, 32)
	if strings.Contains(editView, "top-secret") {
		t.Fatalf("secret leaked to editor: %q", editView)
	}
	_, save := edit.Update(component.EditorSubmitMsg{})
	if save == nil {
		t.Fatal("edit submit returned no navigation command")
	}
	server, _ = manager.Get("docs")
	if server.Headers["Authorization"] != "Bearer top-secret" || server.Expose != "allowlist" {
		t.Fatalf("edit did not preserve server state: %#v", server)
	}

	detail, err := newMCPRoutePage(t.Context(), "docs", "", manager, oauthStore)
	if err != nil {
		t.Fatal(err)
	}
	view = detail.View(120, 32)
	if strings.Contains(view, "top-secret") || !strings.Contains(view, "<redacted>") {
		t.Fatalf("detail secret redaction=%q", view)
	}
	if _, err := detail.openCommand(MCPServerDisable, "docs"); err != nil {
		t.Fatal(err)
	}
	server, _ = manager.Get("docs")
	if server.Enabled {
		t.Fatal("server remained enabled")
	}
	if _, err := detail.openCommand(MCPServerEnable, "docs"); err != nil {
		t.Fatal(err)
	}
	server, _ = manager.Get("docs")
	if !server.Enabled {
		t.Fatal("server remained disabled")
	}
	if _, err := detail.openCommand(MCPServerRemove, "docs"); err != nil {
		t.Fatal(err)
	}
	detail.confirm = component.NewConfirmButtons("Remove", "Cancel", true)
	detail.updateConfirm(tea.KeyPressMsg{Code: tea.KeyEnter})
	if _, ok := manager.Get("docs"); ok {
		t.Fatal("server remained after confirmed removal")
	}
}

func TestMCPDetailRemovalKeepsDetailUntilParentNavigation(t *testing.T) {
	client := &mcpPageClient{}
	_, manager, oauthStore, _ := newMCPPageTestHarness(t, client)
	if err := manager.Add(upstream.Server{ID: "docs", Name: "Docs", Enabled: true, Transport: "http", URL: "https://example.test/mcp", Expose: "all"}); err != nil {
		t.Fatal(err)
	}
	page, err := newMCPRoutePage(t.Context(), "docs", "", manager, oauthStore)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.openCommand(MCPServerRemove, "docs"); err != nil {
		t.Fatal(err)
	}
	page.confirm = component.NewConfirmButtons("Remove", "Cancel", true)
	cmd := page.updateConfirm(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || page.resourceID != "docs" {
		t.Fatalf("navigation=%v resource=%q", cmd != nil, page.resourceID)
	}
	if got := ansi.Strip(page.View(100, 24)); !strings.Contains(got, "MCP server · docs") {
		t.Fatalf("intermediate MCP detail render=%q", got)
	}
	message, ok := cmd().(NavigateMsg)
	if !ok || strings.Join(message.Path, "/") != "mcp" || !message.Replace {
		t.Fatalf("navigation=%#v", message)
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

func TestMCPCreateEditorValidationFailureKeepsDraft(t *testing.T) {
	_, manager, oauthStore, _ := newMCPPageTestHarness(t, &mcpPageClient{})
	page, err := newMCPRoutePageAction(t.Context(), "", "", "create", manager, oauthStore)
	if err != nil {
		t.Fatal(err)
	}
	_ = page.Init()
	updated, _ := page.Update(tea.KeyPressMsg{Code: 'd', Text: "docs"})
	page = updated.(*MCPPage)
	for range 3 {
		updated, _ = page.Update(huh.NextField())
		page = updated.(*MCPPage)
	}
	updated, _ = page.Update(huh.NextField())
	page = updated.(*MCPPage)
	updated, _ = page.Update(tea.KeyPressMsg{Code: 'h', Text: "https://example.test/mcp"})
	page = updated.(*MCPPage)
	updated, _ = page.Update(huh.NextField())
	page = updated.(*MCPPage)
	updated, _ = page.Update(huh.NextField())
	page = updated.(*MCPPage)
	updated, _ = page.Update(tea.KeyPressMsg{Code: '{', Text: "{"})
	page = updated.(*MCPPage)
	updated, cmd := page.Update(component.EditorSubmitMsg{})
	page = updated.(*MCPPage)
	if cmd != nil || page.editor == nil || !page.Dirty() || page.serverForm.ID != "docs" || page.serverForm.SensitiveHeaders != "{" {
		t.Fatalf("validation failure cmd=%v editor=%v dirty=%t draft=%#v", cmd != nil, page.editor != nil, page.Dirty(), page.serverForm)
	}
	plain := ansi.Strip(page.View(80, 24))
	if !strings.Contains(plain, "decode sensitive header JSON") {
		t.Fatalf("validation feedback missing: %q", plain)
	}
	if _, ok := manager.Get("docs"); ok {
		t.Fatal("invalid draft created a server")
	}
}

func TestMCPCreateEditorExistingIDFailureKeepsDraft(t *testing.T) {
	_, manager, oauthStore, _ := newMCPPageTestHarness(t, &mcpPageClient{})
	if err := manager.Add(upstream.Server{ID: "docs", Name: "Existing", Transport: "http", URL: "https://old.example/mcp", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	page, err := newMCPRoutePageAction(t.Context(), "", "", "create", manager, oauthStore)
	if err != nil {
		t.Fatal(err)
	}
	_ = page.Init()
	updated, _ := page.Update(tea.KeyPressMsg{Code: 'd', Text: "docs"})
	page = updated.(*MCPPage)
	for range 4 {
		updated, _ = page.Update(huh.NextField())
		page = updated.(*MCPPage)
	}
	updated, _ = page.Update(tea.KeyPressMsg{Code: 'h', Text: "https://new.example/mcp"})
	page = updated.(*MCPPage)
	updated, cmd := page.Update(component.EditorSubmitMsg{})
	page = updated.(*MCPPage)
	if cmd != nil || page.editor == nil || !page.Dirty() || page.serverForm.ID != "docs" || page.serverForm.URL != "https://new.example/mcp" {
		t.Fatalf("existing-ID failure cmd=%v editor=%v dirty=%t draft=%#v", cmd != nil, page.editor != nil, page.Dirty(), page.serverForm)
	}
	if plain := ansi.Strip(page.View(80, 24)); !strings.Contains(plain, "upstream server already exists: docs") {
		t.Fatalf("existing-ID feedback missing: %q", plain)
	}
	stored, _ := manager.Get("docs")
	if stored.URL != "https://old.example/mcp" || stored.Name != "Existing" {
		t.Fatalf("existing server mutated: %#v", stored)
	}
}

func TestMCPServerEditorCancelReturnsToParent(t *testing.T) {
	_, manager, oauthStore, _ := newMCPPageTestHarness(t, &mcpPageClient{})
	create, err := newMCPRoutePageAction(t.Context(), "", "", "create", manager, oauthStore)
	if err != nil {
		t.Fatal(err)
	}
	_, cmd := create.Update(component.EditorCancelMsg{})
	if cmd == nil {
		t.Fatal("create cancel returned no navigation")
	}
	message, ok := cmd().(NavigateMsg)
	if !ok || strings.Join(message.Path, "/") != "mcp" {
		t.Fatalf("create cancel=%#v", message)
	}
	if err := manager.Add(upstream.Server{ID: "docs", Transport: "http", URL: "https://example.test/mcp", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	edit, err := newMCPRoutePageAction(t.Context(), "docs", "", "edit", manager, oauthStore)
	if err != nil {
		t.Fatal(err)
	}
	_, cmd = edit.Update(component.EditorCancelMsg{})
	if cmd == nil {
		t.Fatal("edit cancel returned no navigation")
	}
	message, ok = cmd().(NavigateMsg)
	if !ok || strings.Join(message.Path, "/") != "mcp/docs" {
		t.Fatalf("edit cancel=%#v", message)
	}
}
