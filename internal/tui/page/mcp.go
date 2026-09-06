package page

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"go.mewis.me/chatgpt-mcp/internal/application"
	mcpoauth "go.mewis.me/chatgpt-mcp/internal/oauth"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
)

type MCPCommand string

const (
	MCPServerAdd       MCPCommand = "mcp.server.add"
	MCPServerConfigure MCPCommand = "mcp.server.configure"
	MCPServerRemove    MCPCommand = "mcp.server.remove"
	MCPServerEnable    MCPCommand = "mcp.server.enable"
	MCPServerDisable   MCPCommand = "mcp.server.disable"
	MCPServerHealth    MCPCommand = "mcp.server.status"
	MCPServerTools     MCPCommand = "mcp.server.tools"
	MCPAuthLogin       MCPCommand = "mcp.server.auth.login"
	MCPAuthLogout      MCPCommand = "mcp.server.auth.logout"
)

type MCPCommandMsg struct {
	Command    MCPCommand
	ResourceID string
}

type mcpOverlayKind uint8

const (
	mcpOverlayNone mcpOverlayKind = iota
	mcpOverlayForm
	mcpOverlayConfirm
	mcpOverlayOperation
)

type mcpHealthMsg struct {
	id       string
	status   upstream.Status
	statuses []upstream.Status
}

type mcpToolsMsg struct {
	id    string
	tools []upstream.Tool
	err   error
}

type mcpOAuthURLMsg struct {
	id  string
	url string
}

type mcpOAuthBrowserErrorMsg struct{ err error }

type mcpOAuthDoneMsg struct {
	id         string
	credential mcpoauth.Credential
	err        error
}

type MCPPage struct {
	ctx                context.Context
	manager            *upstream.Manager
	oauthStore         *mcpoauth.Store
	resourceID         string
	browser            component.Browser
	overlay            mcpOverlayKind
	form               component.Form
	confirm            component.ConfirmButtons
	command            MCPCommand
	targetID           string
	serverForm         *mcpServerFormData
	oauthForm          *mcpOAuthFormData
	progress           *component.Progress
	operationCancel    context.CancelFunc
	operationCancelled bool
	operationURL       string
	oauthEventCh       <-chan tea.Msg
	status             map[string]upstream.Status
	tools              map[string][]upstream.Tool
	notice             string
	err                error
	width              int
	height             int
	openBrowser        func(string) error
	oauthLogin         func(context.Context, mcpoauth.LoginConfig, mcpoauth.LoginOptions) (mcpoauth.Credential, error)
}

func NewMCP(ctx context.Context, resourceID string) (*MCPPage, error) {
	manager := upstream.NewManager(upstream.NewStore(upstream.Path()))
	if err := manager.Load(); err != nil {
		return nil, err
	}
	store := mcpoauth.NewStore(mcpoauth.Path())
	return newMCPPage(ctx, resourceID, manager, store)
}

func newMCPPage(ctx context.Context, resourceID string, manager *upstream.Manager, store *mcpoauth.Store) (*MCPPage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if manager == nil {
		return nil, fmt.Errorf("MCP manager is required")
	}
	if store == nil {
		store = mcpoauth.NewStore(mcpoauth.Path())
	}
	page := &MCPPage{
		ctx: ctx, manager: manager, oauthStore: store, resourceID: strings.TrimSpace(resourceID),
		status: map[string]upstream.Status{}, tools: map[string][]upstream.Tool{}, openBrowser: application.OpenBrowser,
	}
	page.oauthLogin = store.Login
	if err := page.reload(); err != nil {
		return nil, err
	}
	if page.resourceID != "" && !page.browser.OpenDetail(page.resourceID) {
		return nil, fmt.Errorf("MCP server not found: %s", page.resourceID)
	}
	return page, nil
}

func (page *MCPPage) Init() tea.Cmd { return nil }

func (page *MCPPage) OverlayActive() bool {
	return page != nil && (page.overlay != mcpOverlayNone || page.browser.DetailOpen())
}

func (page *MCPPage) InputActive() bool {
	return page != nil && (page.overlay == mcpOverlayForm || page.browser.InputActive())
}

func (page *MCPPage) Update(message tea.Msg) (Model, tea.Cmd) {
	if page == nil {
		return page, nil
	}
	switch msg := message.(type) {
	case mcpHealthMsg:
		return page, page.finishHealth(msg)
	case mcpToolsMsg:
		return page, page.finishTools(msg)
	case mcpOAuthURLMsg:
		page.operationURL = msg.url
		return page, waitMCPEvent(page.oauthEventCh)
	case mcpOAuthBrowserErrorMsg:
		page.notice = "Could not open browser automatically: " + msg.err.Error()
		return page, waitMCPEvent(page.oauthEventCh)
	case mcpOAuthDoneMsg:
		return page, page.finishOAuth(msg)
	case tea.WindowSizeMsg:
		page.width, page.height = msg.Width, msg.Height
		updated, cmd := page.browser.Update(msg)
		page.browser = updated.(component.Browser)
		if page.overlay == mcpOverlayForm {
			form, formCmd := page.form.Update(msg)
			page.form = form
			return page, tea.Batch(cmd, formCmd)
		}
		return page, cmd
	}

	if page.overlay == mcpOverlayOperation {
		if key, ok := message.(tea.KeyPressMsg); ok && key.String() == "esc" {
			page.cancelOperation()
			return page, nil
		}
		if page.progress != nil {
			updated, cmd := page.progress.Update(message)
			page.progress = &updated
			return page, cmd
		}
		return page, nil
	}

	switch msg := message.(type) {
	case component.FormSubmittedMsg:
		return page, page.submitForm()
	case component.FormCancelledMsg:
		page.closeOverlay()
		return page, nil
	case component.FormMouseMsg:
		if page.overlay == mcpOverlayForm {
			updated, cmd := page.form.Update(msg)
			page.form = updated
			return page, cmd
		}
		return page, nil
	case component.ConfirmChoiceMsg:
		if page.overlay == mcpOverlayConfirm {
			page.confirm.Select(msg.Affirmative)
			return page, page.updateConfirm(tea.KeyPressMsg{Code: tea.KeyEnter})
		}
		return page, nil
	case MCPCommandMsg:
		cmd, err := page.openCommand(msg.Command, msg.ResourceID)
		if err != nil {
			page.err = err
		}
		return page, cmd
	case tea.KeyPressMsg:
		if page.overlay == mcpOverlayForm {
			updated, cmd := page.form.Update(msg)
			page.form = updated
			return page, cmd
		}
		if page.overlay == mcpOverlayConfirm {
			return page, page.updateConfirm(msg)
		}
		if page.browser.InputActive() {
			updated, cmd := page.browser.Update(msg)
			page.browser = updated.(component.Browser)
			return page, cmd
		}
		if cmd, handled := page.handleKey(msg); handled {
			return page, cmd
		}
	}
	if page.overlay == mcpOverlayForm {
		updated, cmd := page.form.Update(message)
		page.form = updated
		return page, cmd
	}
	updated, cmd := page.browser.Update(message)
	page.browser = updated.(component.Browser)
	return page, cmd
}

func (page *MCPPage) View(width, height int) string {
	if page == nil {
		return component.StateView(component.PageError, "MCP page unavailable", "")
	}
	browserHeight := height
	if page.err != nil || page.notice != "" {
		browserHeight = max(1, height-2)
	}
	if width > 0 && browserHeight > 0 && (page.width != width || page.height != browserHeight) {
		updated, _ := page.browser.Update(tea.WindowSizeMsg{Width: width, Height: browserHeight})
		page.browser = updated.(component.Browser)
		page.width, page.height = width, browserHeight
	}
	content := page.browser.Content()
	if page.err != nil {
		content += "\n" + component.Banner(page.err.Error(), component.ToneDanger)
	} else if page.notice != "" {
		content += "\n" + component.Banner(page.notice, component.ToneSuccess)
	}
	switch page.overlay {
	case mcpOverlayForm:
		content = component.CenterOverlay(content, component.Modal(page.form.View(), overlayWidth(width, 78)), width, height)
	case mcpOverlayConfirm:
		body := component.Title(page.confirmTitle()) + "\n\n" + component.Muted(page.confirmDescription()) + "\n\n" + page.confirm.View() + "\n" + component.Muted("Enter confirm · Esc cancel")
		content = component.CenterOverlay(content, component.Modal(body, overlayWidth(width, 68)), width, height)
	case mcpOverlayOperation:
		body := ""
		if page.progress != nil {
			body = page.progress.View()
		}
		if page.operationURL != "" {
			body += "\n\n" + component.Label("Authorization URL") + "\n" + page.operationURL
		}
		body += "\n\n" + component.Muted("Esc cancel")
		content = component.CenterOverlay(content, component.Modal(body, overlayWidth(width, 82)), width, height)
	}
	return content
}

func (page *MCPPage) MouseTargets(originX, originY, z int) []component.MouseTarget {
	if page == nil {
		return nil
	}
	switch page.overlay {
	case mcpOverlayForm:
		return formOverlayMouseTargets(page.form, overlayWidth(page.width, 78), page.width, page.height, originX, originY, z+20)
	case mcpOverlayConfirm:
		return confirmOverlayMouseTargets(page.confirm, page.confirmTitle(), page.confirmDescription(), overlayWidth(page.width, 68), page.width, page.height, originX, originY, z+20)
	case mcpOverlayOperation:
		return []component.MouseTarget{mouseBlocker(originX, originY, page.width, page.height, z+20)}
	default:
		return page.browser.MouseTargets(originX, originY, z)
	}
}

func (page *MCPPage) handleKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	selected, _ := page.browser.Selected()
	id := selected.ID
	if page.resourceID != "" {
		id = page.resourceID
	}
	switch msg.String() {
	case "enter":
		if page.browser.DetailOpen() || id == "" {
			return nil, false
		}
		return func() tea.Msg { return NavigateMsg{Path: []string{"mcp", id}} }, true
	case "a":
		cmd, err := page.openCommand(MCPServerAdd, "")
		page.err = err
		return cmd, true
	case "e":
		if id == "" {
			return nil, true
		}
		cmd, err := page.openCommand(MCPServerConfigure, id)
		page.err = err
		return cmd, true
	case "d":
		if id == "" {
			return nil, true
		}
		cmd, err := page.openCommand(MCPServerRemove, id)
		page.err = err
		return cmd, true
	case "space":
		if id == "" {
			return nil, true
		}
		server, ok := page.manager.Get(id)
		if !ok {
			page.err = fmt.Errorf("unknown upstream server: %s", id)
			return nil, true
		}
		command := MCPServerEnable
		if server.Enabled {
			command = MCPServerDisable
		}
		cmd, err := page.openCommand(command, id)
		page.err = err
		return cmd, true
	case "r":
		cmd, err := page.openCommand(MCPServerHealth, id)
		page.err = err
		return cmd, true
	case "t":
		if id == "" {
			return nil, true
		}
		cmd, err := page.openCommand(MCPServerTools, id)
		page.err = err
		return cmd, true
	case "o":
		if id == "" {
			return nil, true
		}
		cmd, err := page.openCommand(MCPAuthLogin, id)
		page.err = err
		return cmd, true
	case "l":
		if id == "" {
			return nil, true
		}
		cmd, err := page.openCommand(MCPAuthLogout, id)
		page.err = err
		return cmd, true
	}
	return nil, false
}

func (page *MCPPage) openCommand(command MCPCommand, resourceID string) (tea.Cmd, error) {
	page.err, page.notice = nil, ""
	page.command, page.targetID = command, strings.TrimSpace(resourceID)
	switch command {
	case MCPServerAdd:
		page.form, page.serverForm = newMCPServerForm(upstream.Server{}, true)
		page.overlay = mcpOverlayForm
		return page.form.Init(), nil
	case MCPServerConfigure:
		server, ok := page.manager.Get(page.targetID)
		if !ok {
			return nil, fmt.Errorf("unknown upstream server: %s", page.targetID)
		}
		page.form, page.serverForm = newMCPServerForm(server, false)
		page.overlay = mcpOverlayForm
		return page.form.Init(), nil
	case MCPServerRemove:
		if _, ok := page.manager.Get(page.targetID); !ok {
			return nil, fmt.Errorf("unknown upstream server: %s", page.targetID)
		}
		page.confirm = component.NewConfirmButtons("Remove", "Cancel", false)
		page.overlay = mcpOverlayConfirm
		return nil, nil
	case MCPServerEnable, MCPServerDisable:
		return nil, page.toggleServer(command == MCPServerEnable)
	case MCPServerHealth:
		return page.startHealth(page.targetID), nil
	case MCPServerTools:
		if _, ok := page.manager.Get(page.targetID); !ok {
			return nil, fmt.Errorf("unknown upstream server: %s", page.targetID)
		}
		return page.startTools(page.targetID), nil
	case MCPAuthLogin:
		server, ok := page.manager.Get(page.targetID)
		if !ok {
			return nil, fmt.Errorf("unknown upstream server: %s", page.targetID)
		}
		if server.Transport != "http" {
			return nil, fmt.Errorf("OAuth login requires an HTTP upstream server")
		}
		if server.Auth.Type == "none" {
			return nil, fmt.Errorf("OAuth is disabled for %s", server.ID)
		}
		page.form, page.oauthForm = newMCPOAuthForm()
		page.overlay = mcpOverlayForm
		return page.form.Init(), nil
	case MCPAuthLogout:
		if _, ok := page.manager.Get(page.targetID); !ok {
			return nil, fmt.Errorf("unknown upstream server: %s", page.targetID)
		}
		page.confirm = component.NewConfirmButtons("Logout", "Cancel", false)
		page.overlay = mcpOverlayConfirm
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported MCP action: %s", command)
	}
}

func (page *MCPPage) submitForm() tea.Cmd {
	switch page.command {
	case MCPServerAdd, MCPServerConfigure:
		return page.submitServerForm()
	case MCPAuthLogin:
		return page.startOAuthLogin()
	default:
		page.err = fmt.Errorf("unsupported MCP form action: %s", page.command)
		return nil
	}
}

func (page *MCPPage) submitServerForm() tea.Cmd {
	create := page.command == MCPServerAdd
	existing := upstream.Server{}
	if !create {
		var ok bool
		existing, ok = page.manager.Get(page.targetID)
		if !ok {
			page.err = fmt.Errorf("unknown upstream server: %s", page.targetID)
			return nil
		}
	}
	server, err := serverFromMCPForm(page.serverForm, existing, create)
	if err != nil {
		page.err = err
		return nil
	}
	if create {
		if _, exists := page.manager.Get(server.ID); exists {
			page.err = fmt.Errorf("upstream server already exists: %s", server.ID)
			return nil
		}
	}
	if err := page.manager.Add(server); err != nil {
		page.err = err
		return nil
	}
	page.notice = "MCP server updated"
	if create {
		page.notice = "MCP server added"
	}
	page.serverForm = nil
	page.closeOverlay()
	if err := page.reload(); err != nil {
		page.err = err
		return nil
	}
	if create {
		return func() tea.Msg { return NavigateMsg{Path: []string{"mcp", server.ID}} }
	}
	return nil
}

func (page *MCPPage) updateConfirm(msg tea.KeyPressMsg) tea.Cmd {
	if msg.String() == "esc" {
		page.closeOverlay()
		return nil
	}
	if msg.String() != "enter" {
		return page.confirm.Update(msg)
	}
	if !page.confirm.AffirmativeSelected() {
		page.closeOverlay()
		return nil
	}
	target := page.targetID
	switch page.command {
	case MCPServerRemove:
		if err := page.manager.Remove(target); err != nil {
			page.err = err
			return nil
		}
		delete(page.status, target)
		delete(page.tools, target)
		page.resourceID = ""
		page.notice = "MCP server removed"
		page.closeOverlay()
		if err := page.reload(); err != nil {
			page.err = err
		}
		return func() tea.Msg { return NavigateMsg{Path: []string{"mcp"}} }
	case MCPAuthLogout:
		if err := page.oauthStore.Delete(target); err != nil {
			page.err = err
			return nil
		}
		page.notice = "OAuth authorization removed"
		page.closeOverlay()
		_ = page.reload()
		return nil
	default:
		page.err = fmt.Errorf("unsupported MCP confirmation: %s", page.command)
		return nil
	}
}

func (page *MCPPage) toggleServer(enabled bool) error {
	server, ok := page.manager.Get(page.targetID)
	if !ok {
		return fmt.Errorf("unknown upstream server: %s", page.targetID)
	}
	server.Enabled = enabled
	if err := page.manager.Add(server); err != nil {
		return err
	}
	page.notice = "MCP server disabled"
	if enabled {
		page.notice = "MCP server enabled"
	}
	return page.reload()
}

func (page *MCPPage) startHealth(id string) tea.Cmd {
	ctx, cancel := context.WithTimeout(page.ctx, 15*time.Second)
	page.beginOperation(MCPServerHealth, id, "Refreshing MCP health", cancel)
	if id == "" {
		return func() tea.Msg { return mcpHealthMsg{statuses: page.manager.ListStatuses(ctx, true)} }
	}
	return func() tea.Msg { return mcpHealthMsg{id: id, status: page.manager.CheckHealth(ctx, id, true)} }
}

func (page *MCPPage) finishHealth(msg mcpHealthMsg) tea.Cmd {
	if page.operationCancelled {
		page.finishCancelledOperation()
		return nil
	}
	if msg.id != "" {
		page.status[msg.id] = msg.status
	} else {
		for _, status := range msg.statuses {
			page.status[status.ID] = status
		}
	}
	page.finishOperation("MCP health refreshed", nil)
	_ = page.reload()
	return nil
}

func (page *MCPPage) startTools(id string) tea.Cmd {
	ctx, cancel := context.WithTimeout(page.ctx, 15*time.Second)
	page.beginOperation(MCPServerTools, id, "Loading MCP tools", cancel)
	return func() tea.Msg {
		values, err := page.manager.Tools(ctx, id, true)
		return mcpToolsMsg{id: id, tools: values, err: err}
	}
}

func (page *MCPPage) finishTools(msg mcpToolsMsg) tea.Cmd {
	if page.operationCancelled {
		page.finishCancelledOperation()
		return nil
	}
	if msg.err != nil {
		page.finishOperation("", msg.err)
		return nil
	}
	page.tools[msg.id] = append([]upstream.Tool(nil), msg.tools...)
	page.finishOperation(fmt.Sprintf("Loaded %d MCP tools", len(msg.tools)), nil)
	_ = page.reload()
	return nil
}

func (page *MCPPage) startOAuthLogin() tea.Cmd {
	server, ok := page.manager.Get(page.targetID)
	if !ok {
		page.err = fmt.Errorf("unknown upstream server: %s", page.targetID)
		return nil
	}
	data := page.oauthForm
	if data == nil {
		page.err = fmt.Errorf("OAuth form is unavailable")
		return nil
	}
	ctx, cancel := context.WithTimeout(page.ctx, 5*time.Minute)
	page.beginOperation(MCPAuthLogin, server.ID, "Waiting for OAuth authorization", cancel)
	page.oauthForm = nil
	events := make(chan tea.Msg, 8)
	page.oauthEventCh = events
	login := page.oauthLogin
	openBrowser := page.openBrowser
	go func() {
		credential, err := login(ctx, mcpoauth.LoginConfig{
			ServerID: server.ID, ServerURL: server.URL, Scope: server.Auth.Scope, Issuer: data.Issuer,
			ClientID: data.ClientID, ClientSecretEnvVar: data.ClientSecretEnvVar, ClientMetadataURL: data.ClientMetadataURL,
		}, mcpoauth.LoginOptions{ExtraScope: data.ExtraScope, OnURL: func(raw string) error {
			events <- mcpOAuthURLMsg{id: server.ID, url: raw}
			if data.OpenBrowser && openBrowser != nil {
				if err := openBrowser(raw); err != nil {
					events <- mcpOAuthBrowserErrorMsg{err: err}
				}
			}
			return nil
		}})
		events <- mcpOAuthDoneMsg{id: server.ID, credential: credential, err: err}
		close(events)
	}()
	return waitMCPEvent(events)
}

func (page *MCPPage) finishOAuth(msg mcpOAuthDoneMsg) tea.Cmd {
	page.oauthEventCh = nil
	if page.operationCancelled {
		page.finishCancelledOperation()
		return nil
	}
	if msg.err != nil {
		page.finishOperation("", msg.err)
		return nil
	}
	page.finishOperation("OAuth authorization stored", nil)
	_ = page.reload()
	return page.startHealth(msg.id)
}

func (page *MCPPage) beginOperation(command MCPCommand, targetID, title string, cancel context.CancelFunc) {
	page.command, page.targetID = command, targetID
	page.operationCancel = cancel
	page.operationCancelled = false
	page.operationURL = ""
	progress := component.NewProgress(title)
	page.progress = &progress
	page.overlay = mcpOverlayOperation
	page.err = nil
}

func (page *MCPPage) cancelOperation() {
	if page.operationCancel != nil {
		page.operationCancel()
	}
	page.operationCancelled = true
	page.overlay = mcpOverlayNone
	page.progress = nil
	page.operationURL = ""
	page.notice = "Operation cancellation requested"
}

func (page *MCPPage) finishCancelledOperation() {
	page.finishOperation("Operation cancelled", nil)
	page.operationCancelled = false
}

func (page *MCPPage) finishOperation(notice string, err error) {
	if page.operationCancel != nil {
		page.operationCancel()
	}
	page.operationCancel = nil
	page.overlay = mcpOverlayNone
	page.progress = nil
	page.operationURL = ""
	page.err = err
	if err == nil && notice != "" {
		page.notice = notice
	}
}

func (page *MCPPage) closeOverlay() {
	page.overlay = mcpOverlayNone
	page.form = component.Form{}
	page.serverForm = nil
	page.oauthForm = nil
	page.confirm = component.ConfirmButtons{}
}

func (page *MCPPage) reload() error {
	rows, err := page.rows()
	if err != nil {
		return err
	}
	refresh := func(context.Context) ([]component.Row, error) { return page.rows() }
	page.browser = component.NewBrowser(page.ctx, "Upstream MCP servers", rows, refresh)
	if page.width > 0 && page.height > 0 {
		updated, _ := page.browser.Update(tea.WindowSizeMsg{Width: page.width, Height: page.height})
		page.browser = updated.(component.Browser)
	}
	if page.resourceID != "" {
		page.browser.OpenDetail(page.resourceID)
	}
	return nil
}

func (page *MCPPage) rows() ([]component.Row, error) {
	servers := page.manager.List()
	rows := make([]component.Row, 0, len(servers))
	for _, server := range servers {
		redacted := upstream.RedactServer(server)
		endpoint := redacted.URL
		if redacted.Transport == "stdio" {
			endpoint = redacted.Command
		}
		state := "disabled"
		if redacted.Enabled {
			state = "enabled"
		}
		rows = append(rows, component.Row{
			ID: server.ID, Title: redacted.Name, Description: server.ID + " · " + endpoint, Meta: redacted.Transport + " · " + state,
			Search: strings.Join([]string{server.ID, redacted.Name, endpoint, redacted.ToolPrefix, redacted.Expose}, " "), DetailTitle: "MCP server · " + server.ID,
			DetailTabs: []component.DetailTab{
				{Title: "Overview", Content: page.serverOverview(redacted)},
				{Title: "Health", Content: page.serverHealth(server)},
				{Title: "Tools", Content: page.serverTools(server)},
				{Title: "OAuth", Content: page.serverOAuth(server)},
			},
		})
	}
	return rows, nil
}

func (page *MCPPage) serverOverview(server upstream.Server) string {
	endpoint := server.URL
	if server.Transport == "stdio" {
		endpoint = server.Command
	}
	fields := [][2]string{
		{"ID", server.ID}, {"Name", server.Name}, {"Transport", server.Transport}, {"Endpoint", endpoint}, {"Enabled", fmt.Sprint(server.Enabled)},
		{"Auth", server.Auth.Type}, {"Auth scope", server.Auth.Scope}, {"Expose", server.Expose}, {"Tool prefix", server.ToolPrefix}, {"Idle timeout", fmt.Sprintf("%ds", server.IdleTimeoutSec)},
		{"Bearer env", server.BearerTokenEnvVar}, {"CWD", server.CWD}, {"Args", joinedOrNone(server.Args)}, {"Headers", assignmentText(server.Headers)}, {"Environment", assignmentText(server.Env)},
		{"Allowlisted tools", joinedOrNone(server.Tools)}, {"Disabled tools", joinedOrNone(server.DisabledTools)},
	}
	return detailFields(fields...)
}

func (page *MCPPage) serverHealth(server upstream.Server) string {
	status, ok := page.status[server.ID]
	if !ok {
		if !server.Enabled {
			return detailFields([2]string{"Health", string(upstream.HealthDisabled)}, [2]string{"Enabled", "false"})
		}
		return component.Muted("Not checked yet. Use r or Command Palette -> MCP: Refresh health.")
	}
	pid := ""
	if status.PID != nil {
		pid = fmt.Sprint(*status.PID)
	}
	return detailFields(
		[2]string{"Health", string(status.Health)}, [2]string{"Connected", fmt.Sprint(status.Connected)}, [2]string{"Tools", fmt.Sprint(status.ToolCount)},
		[2]string{"Auth", status.Auth}, [2]string{"Expose", status.Expose}, [2]string{"PID", pid}, [2]string{"Error", status.LastError},
	)
}

func (page *MCPPage) serverTools(server upstream.Server) string {
	tools, ok := page.tools[server.ID]
	if !ok {
		return component.Muted("Tools not loaded yet. Use t or Command Palette -> MCP: View tools.")
	}
	if len(tools) == 0 {
		return component.Muted("No tools exposed by the upstream server.")
	}
	proxied := map[string]bool{}
	for _, name := range page.manager.ProxiedToolNames(server, tools) {
		proxied[name] = true
	}
	lines := make([]string, 0, len(tools))
	for _, tool := range tools {
		proxy := upstream.ProxyName(server.ToolPrefix, tool.Name)
		state := "hidden"
		if proxied[proxy] {
			state = proxy
		}
		line := tool.Name + " · " + state
		if strings.TrimSpace(tool.Description) != "" {
			line += "\n  " + tool.Description
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n\n")
}

func (page *MCPPage) serverOAuth(server upstream.Server) string {
	if server.Transport != "http" {
		return component.Muted("OAuth is only available for HTTP upstream servers.")
	}
	status, err := page.oauthStore.Status(server.ID)
	if err != nil {
		return component.Banner(err.Error(), component.ToneDanger)
	}
	if !status.Configured {
		return detailFields([2]string{"Configured", "false"}, [2]string{"Auth mode", server.Auth.Type}, [2]string{"Scope", server.Auth.Scope})
	}
	expires := ""
	if status.ExpiresAt != nil {
		expires = status.ExpiresAt.Format(time.RFC3339)
	}
	return detailFields(
		[2]string{"Configured", "true"}, [2]string{"Issuer", status.Issuer}, [2]string{"Registration", status.Registration}, [2]string{"Client ID", status.ClientID},
		[2]string{"Scopes", strings.Join(status.Scopes, " ")}, [2]string{"Refresh token", fmt.Sprint(status.HasRefreshToken)}, [2]string{"Expires", expires}, [2]string{"Expired", fmt.Sprint(status.Expired)},
	)
}

func (page *MCPPage) confirmTitle() string {
	if page.command == MCPAuthLogout {
		return "Remove OAuth authorization for " + page.targetID + "?"
	}
	return "Remove MCP server " + page.targetID + "?"
}

func (page *MCPPage) confirmDescription() string {
	if page.command == MCPAuthLogout {
		return "Stored OAuth credentials will be deleted. The MCP server configuration is preserved."
	}
	return "The MCP server configuration and its managed OAuth credentials will be removed. External server data is unchanged."
}

func waitMCPEvent(events <-chan tea.Msg) tea.Cmd {
	if events == nil {
		return nil
	}
	return func() tea.Msg {
		message, ok := <-events
		if !ok {
			return nil
		}
		return message
	}
}
