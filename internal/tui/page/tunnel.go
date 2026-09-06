package page

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

const tunnelOperationTimeout = 30 * time.Second

type TunnelCommand string

const (
	TunnelConfigure        TunnelCommand = "tunnel.configure"
	TunnelEnable           TunnelCommand = "tunnel.enable"
	TunnelDisable          TunnelCommand = "tunnel.disable"
	TunnelSync             TunnelCommand = "tunnel.sync"
	TunnelAdminKeySet      TunnelCommand = "tunnel.admin.key.set"
	TunnelAdminKeyVerify   TunnelCommand = "tunnel.admin.key.verify"
	TunnelAdminKeyRemove   TunnelCommand = "tunnel.admin.key.remove"
	TunnelManagedRefresh   TunnelCommand = "tunnel.managed.refresh"
	TunnelManagedCreate    TunnelCommand = "tunnel.managed.create"
	TunnelManagedUpdate    TunnelCommand = "tunnel.managed.update"
	TunnelManagedDelete    TunnelCommand = "tunnel.managed.delete"
	TunnelManagedConfigure TunnelCommand = "tunnel.managed.configure"
)

type TunnelCommandMsg struct {
	Command    TunnelCommand
	ResourceID string
}

type tunnelPageKind uint8

const (
	tunnelPageRuntime tunnelPageKind = iota
	tunnelPageManaged
)

type tunnelOverlayKind uint8

const (
	tunnelOverlayNone tunnelOverlayKind = iota
	tunnelOverlayForm
	tunnelOverlayConfirm
	tunnelOverlayOperation
)

type tunnelOperationMsg struct {
	command   TunnelCommand
	targetID  string
	dashboard application.TunnelDashboard
	metadata  tunnel.Metadata
	items     []tunnel.Metadata
	result    application.ManagedTunnelResult
	count     int
	scope     tunnel.AdminScope
	err       error
}

type TunnelPage struct {
	ctx                context.Context
	kind               tunnelPageKind
	resourceID         string
	dashboard          application.TunnelDashboard
	adminStatus        application.TunnelAdminStatus
	items              []tunnel.Metadata
	browser            component.Browser
	overlay            tunnelOverlayKind
	form               component.Form
	confirm            component.ConfirmButtons
	progress           *component.Progress
	command            TunnelCommand
	targetID           string
	operationCancel    context.CancelFunc
	operationCancelled bool
	runtimeForm        *tunnelRuntimeFormData
	adminForm          *tunnelAdminFormData
	managedForm        *managedTunnelFormData
	managedUpdateFetch bool
	configureForm      *managedConfigureFormData
	deleteClear        bool
	deleteOptions      bool
	notice             string
	err                error
	width              int
	height             int
}

func NewTunnelDashboard(ctx context.Context) (*TunnelPage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	dashboard, err := application.TunnelStatus()
	if err != nil {
		return nil, err
	}
	adminStatus, err := application.TunnelAdminKeyStatus()
	if err != nil {
		return nil, err
	}
	return &TunnelPage{ctx: ctx, kind: tunnelPageRuntime, dashboard: dashboard, adminStatus: adminStatus}, nil
}

func NewManagedTunnels(ctx context.Context, resourceID string) (*TunnelPage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	items, err := config.ListTunnelMetadata()
	if err != nil {
		return nil, err
	}
	page := &TunnelPage{ctx: ctx, kind: tunnelPageManaged, resourceID: strings.TrimSpace(resourceID), items: items}
	if err := page.reloadManagedBrowser(); err != nil {
		return nil, err
	}
	if page.resourceID != "" && !page.browser.OpenDetail(page.resourceID) {
		return nil, fmt.Errorf("managed tunnel not found in local cache: %s", page.resourceID)
	}
	return page, nil
}

func (page *TunnelPage) Init() tea.Cmd { return nil }

func (page *TunnelPage) OverlayActive() bool {
	return page != nil && (page.overlay != tunnelOverlayNone || page.kind == tunnelPageManaged && page.browser.DetailOpen())
}

func (page *TunnelPage) Update(message tea.Msg) (Model, tea.Cmd) {
	if page == nil {
		return page, nil
	}
	if msg, ok := message.(tunnelOperationMsg); ok {
		return page, page.finishOperation(msg)
	}
	if page.overlay == tunnelOverlayOperation {
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
	case tea.WindowSizeMsg:
		page.width, page.height = msg.Width, msg.Height
		if page.kind == tunnelPageManaged {
			updated, cmd := page.browser.Update(msg)
			page.browser = updated.(component.Browser)
			return page, cmd
		}
		return page, nil
	case component.FormSubmittedMsg:
		return page, page.submitForm()
	case component.FormCancelledMsg:
		page.closeOverlay()
		return page, nil
	case component.FormMouseMsg:
		if page.overlay == tunnelOverlayForm {
			updated, cmd := page.form.Update(msg)
			page.form = updated
			return page, cmd
		}
		return page, nil
	case component.ConfirmChoiceMsg:
		if page.overlay == tunnelOverlayConfirm {
			page.confirm.Select(msg.Affirmative)
			return page, page.updateConfirm(tea.KeyPressMsg{Code: tea.KeyEnter})
		}
		return page, nil
	case TunnelCommandMsg:
		cmd, err := page.openCommand(msg.Command, msg.ResourceID)
		if err != nil {
			page.err = err
		}
		return page, cmd
	case tea.KeyPressMsg:
		if page.overlay == tunnelOverlayForm {
			updated, cmd := page.form.Update(msg)
			page.form = updated
			return page, cmd
		}
		if page.overlay == tunnelOverlayConfirm {
			return page, page.updateConfirm(msg)
		}
		if cmd, handled := page.handleKey(msg); handled {
			return page, cmd
		}
	}
	if page.kind == tunnelPageManaged {
		updated, cmd := page.browser.Update(message)
		page.browser = updated.(component.Browser)
		return page, cmd
	}
	return page, nil
}

func (page *TunnelPage) View(width, height int) string {
	if page == nil {
		return component.StateView(component.PageError, "Tunnel page unavailable", "")
	}
	page.width, page.height = width, height
	content := page.runtimeView(width)
	if page.kind == tunnelPageManaged {
		browserHeight := height
		if page.err != nil || page.notice != "" {
			browserHeight = max(10, height-2)
		}
		updated, _ := page.browser.Update(tea.WindowSizeMsg{Width: width, Height: browserHeight})
		page.browser = updated.(component.Browser)
		content = page.browser.Content()
	}
	if page.err != nil {
		content += "\n" + component.Banner(page.err.Error(), component.ToneDanger)
	} else if page.notice != "" {
		content += "\n" + component.Banner(page.notice, component.ToneSuccess)
	}
	switch page.overlay {
	case tunnelOverlayForm:
		content = component.CenterOverlay(content, component.Modal(page.form.View(), min(80, max(48, width-8))), width, height)
	case tunnelOverlayConfirm:
		body := component.Title(page.confirmTitle()) + "\n\n" + component.Muted(page.confirmDescription()) + "\n\n" + page.confirm.View() + "\n" + component.Muted("Enter confirm · Esc cancel")
		content = component.CenterOverlay(content, component.Modal(body, min(72, max(44, width-8))), width, height)
	case tunnelOverlayOperation:
		body := ""
		if page.progress != nil {
			body = page.progress.View()
		}
		body += "\n\n" + component.Muted("Esc cancel")
		content = component.CenterOverlay(content, component.Modal(body, min(72, max(44, width-8))), width, height)
	}
	return content
}

func (page *TunnelPage) MouseTargets(originX, originY, z int) []component.MouseTarget {
	if page == nil {
		return nil
	}
	switch page.overlay {
	case tunnelOverlayForm:
		return formOverlayMouseTargets(page.form, min(80, max(48, page.width-8)), page.width, page.height, originX, originY, z+20)
	case tunnelOverlayConfirm:
		return confirmOverlayMouseTargets(page.confirm, page.confirmTitle(), page.confirmDescription(), min(72, max(44, page.width-8)), page.width, page.height, originX, originY, z+20)
	case tunnelOverlayOperation:
		return []component.MouseTarget{mouseBlocker(originX, originY, page.width, page.height, z+20)}
	}
	if page.kind == tunnelPageManaged {
		return page.browser.MouseTargets(originX, originY, z)
	}
	view := page.runtimeView(page.width)
	return keyHintMouseTargets(view, map[string]string{
		"Configure": "e", "Enable/Disable": " ", "Sync": "s", "Admin key": "a", "Verify": "v", "Remove admin": "d", "Managed tunnels": "m",
	}, originX, originY, z)
}

func (page *TunnelPage) handleKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	if page.kind == tunnelPageRuntime {
		switch msg.String() {
		case "e":
			cmd, err := page.openCommand(TunnelConfigure, "")
			page.err = err
			return cmd, true
		case "space":
			command := TunnelEnable
			if page.dashboard.Config.Enabled {
				command = TunnelDisable
			}
			cmd, err := page.openCommand(command, "")
			page.err = err
			return cmd, true
		case "s":
			cmd, err := page.openCommand(TunnelSync, "")
			page.err = err
			return cmd, true
		case "a":
			cmd, err := page.openCommand(TunnelAdminKeySet, "")
			page.err = err
			return cmd, true
		case "v":
			cmd, err := page.openCommand(TunnelAdminKeyVerify, "")
			page.err = err
			return cmd, true
		case "d":
			cmd, err := page.openCommand(TunnelAdminKeyRemove, "")
			page.err = err
			return cmd, true
		case "m":
			return func() tea.Msg { return NavigateMsg{Path: []string{"tunnels"}} }, true
		}
		return nil, false
	}

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
		return func() tea.Msg { return NavigateMsg{Path: []string{"tunnels", id}} }, true
	case "r":
		refreshID := ""
		if page.browser.DetailOpen() {
			refreshID = id
		}
		cmd, err := page.openCommand(TunnelManagedRefresh, refreshID)
		page.err = err
		return cmd, true
	case "a":
		cmd, err := page.openCommand(TunnelManagedCreate, "")
		page.err = err
		return cmd, true
	case "e":
		if id == "" {
			return nil, true
		}
		cmd, err := page.openCommand(TunnelManagedUpdate, id)
		page.err = err
		return cmd, true
	case "c":
		if id == "" {
			return nil, true
		}
		cmd, err := page.openCommand(TunnelManagedConfigure, id)
		page.err = err
		return cmd, true
	case "d":
		if id == "" {
			return nil, true
		}
		cmd, err := page.openCommand(TunnelManagedDelete, id)
		page.err = err
		return cmd, true
	}
	return nil, false
}

func (page *TunnelPage) openCommand(command TunnelCommand, resourceID string) (tea.Cmd, error) {
	page.err, page.notice = nil, ""
	page.command, page.targetID = command, strings.TrimSpace(resourceID)
	switch command {
	case TunnelConfigure:
		page.form, page.runtimeForm = newTunnelRuntimeForm(page.dashboard)
		page.overlay = tunnelOverlayForm
		return page.form.Init(), nil
	case TunnelEnable, TunnelDisable:
		enabled := command == TunnelEnable
		return page.startOperation(command, "", "Updating tunnel state", func(context.Context) tunnelOperationMsg {
			dashboard, err := application.SetTunnelEnabled(enabled)
			return tunnelOperationMsg{command: command, dashboard: dashboard, err: err}
		}), nil
	case TunnelSync:
		return page.startOperation(command, "", "Syncing tunnel metadata", func(ctx context.Context) tunnelOperationMsg {
			metadata, _, err := application.SyncConfiguredTunnel(ctx)
			return tunnelOperationMsg{command: command, metadata: metadata, err: err}
		}), nil
	case TunnelAdminKeySet:
		status, err := application.TunnelAdminKeyStatus()
		if err != nil {
			return nil, err
		}
		page.form, page.adminForm = newTunnelAdminForm(status)
		page.overlay = tunnelOverlayForm
		return page.form.Init(), nil
	case TunnelAdminKeyVerify:
		return page.startOperation(command, "", "Verifying tunnel admin key", func(ctx context.Context) tunnelOperationMsg {
			count, scope, err := application.VerifyTunnelAdminKey(ctx)
			return tunnelOperationMsg{command: command, count: count, scope: scope, err: err}
		}), nil
	case TunnelAdminKeyRemove:
		if !page.adminStatus.Configured {
			return nil, fmt.Errorf("tunnel admin key is not configured")
		}
		page.confirm = component.NewConfirmButtons("Remove", "Cancel", false)
		page.overlay = tunnelOverlayConfirm
		return nil, nil
	case TunnelManagedRefresh:
		if page.targetID == "" {
			return page.startOperation(command, "", "Refreshing managed tunnels", func(ctx context.Context) tunnelOperationMsg {
				items, err := application.RefreshManagedTunnels(ctx)
				return tunnelOperationMsg{command: command, items: items, err: err}
			}), nil
		}
		return page.startOperation(command, page.targetID, "Refreshing managed tunnel", func(ctx context.Context) tunnelOperationMsg {
			result, err := application.GetManagedTunnel(ctx, page.targetID, application.ManagedTunnelOptions{})
			return tunnelOperationMsg{command: command, targetID: page.targetID, result: result, err: err}
		}), nil
	case TunnelManagedCreate:
		page.form, page.managedForm = newManagedTunnelForm(tunnel.Metadata{}, true)
		page.overlay = tunnelOverlayForm
		return page.form.Init(), nil
	case TunnelManagedUpdate:
		if page.targetID == "" {
			return nil, fmt.Errorf("managed tunnel id is required")
		}
		page.managedUpdateFetch = true
		return page.startOperation(command, page.targetID, "Loading managed tunnel", func(ctx context.Context) tunnelOperationMsg {
			result, err := application.GetManagedTunnel(ctx, page.targetID, application.ManagedTunnelOptions{})
			return tunnelOperationMsg{command: command, targetID: page.targetID, result: result, err: err}
		}), nil
	case TunnelManagedConfigure:
		if page.targetID == "" {
			return nil, fmt.Errorf("managed tunnel id is required")
		}
		page.form, page.configureForm = newManagedConfigureForm()
		page.overlay = tunnelOverlayForm
		return page.form.Init(), nil
	case TunnelManagedDelete:
		if page.targetID == "" {
			return nil, fmt.Errorf("managed tunnel id is required")
		}
		page.deleteClear = page.dashboard.Config.ID == page.targetID
		if dashboard, err := application.TunnelStatus(); err == nil {
			page.dashboard = dashboard
			page.deleteClear = dashboard.Config.ID == page.targetID
		}
		if page.deleteClear {
			page.deleteOptions = true
			page.form = component.NewForm(component.Group(component.Confirm("Also clear the selected runtime tunnel configuration", &page.deleteClear)))
			page.overlay = tunnelOverlayForm
			return page.form.Init(), nil
		}
		page.confirm = component.NewConfirmButtons("Delete", "Cancel", false)
		page.overlay = tunnelOverlayConfirm
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported tunnel action: %s", command)
	}
}

func (page *TunnelPage) submitForm() tea.Cmd {
	switch page.command {
	case TunnelConfigure:
		input := runtimeInputFromForm(page.runtimeForm)
		page.runtimeForm = nil
		return page.startOperation(page.command, "", "Saving tunnel configuration", func(ctx context.Context) tunnelOperationMsg {
			dashboard, err := application.ConfigureTunnelRuntime(ctx, input)
			return tunnelOperationMsg{command: TunnelConfigure, dashboard: dashboard, err: err}
		})
	case TunnelAdminKeySet:
		input := adminInputFromForm(page.adminForm)
		page.adminForm = nil
		return page.startOperation(page.command, "", "Verifying tunnel admin key", func(ctx context.Context) tunnelOperationMsg {
			count, scope, err := application.SetTunnelAdminKey(ctx, input)
			return tunnelOperationMsg{command: TunnelAdminKeySet, count: count, scope: scope, err: err}
		})
	case TunnelManagedCreate:
		request, options := managedCreateInput(page.managedForm)
		page.managedForm = nil
		return page.startOperation(page.command, "", "Creating managed tunnel", func(ctx context.Context) tunnelOperationMsg {
			result, err := application.CreateManagedTunnel(ctx, request, options)
			return tunnelOperationMsg{command: TunnelManagedCreate, result: result, err: err}
		})
	case TunnelManagedUpdate:
		request, options := managedUpdateInput(page.managedForm)
		page.managedForm = nil
		id := page.targetID
		return page.startOperation(page.command, id, "Updating managed tunnel", func(ctx context.Context) tunnelOperationMsg {
			result, err := application.UpdateManagedTunnel(ctx, id, request, options)
			return tunnelOperationMsg{command: TunnelManagedUpdate, targetID: id, result: result, err: err}
		})
	case TunnelManagedConfigure:
		data, id := page.configureForm, page.targetID
		page.configureForm = nil
		return page.startOperation(page.command, id, "Configuring managed tunnel", func(ctx context.Context) tunnelOperationMsg {
			result, err := application.GetManagedTunnel(ctx, id, application.ManagedTunnelOptions{Configure: true, RuntimeAPIKey: data.RuntimeAPIKey, Enable: data.Enable})
			return tunnelOperationMsg{command: TunnelManagedConfigure, targetID: id, result: result, err: err}
		})
	case TunnelManagedDelete:
		if page.deleteOptions {
			page.deleteOptions = false
			page.confirm = component.NewConfirmButtons("Delete", "Cancel", false)
			page.overlay = tunnelOverlayConfirm
		}
		return nil
	default:
		page.err = fmt.Errorf("unsupported tunnel form action: %s", page.command)
		return nil
	}
}

func (page *TunnelPage) updateConfirm(msg tea.KeyPressMsg) tea.Cmd {
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
	switch page.command {
	case TunnelAdminKeyRemove:
		if err := application.RemoveTunnelAdminKey(); err != nil {
			page.err = err
			return nil
		}
		page.notice = "Tunnel admin key removed"
		page.closeOverlay()
		page.reloadDashboard()
		return nil
	case TunnelManagedDelete:
		id, clearConfig := page.targetID, page.deleteClear
		return page.startOperation(page.command, id, "Deleting managed tunnel", func(ctx context.Context) tunnelOperationMsg {
			result, err := application.DeleteManagedTunnel(ctx, id, clearConfig)
			return tunnelOperationMsg{command: TunnelManagedDelete, targetID: id, result: result, err: err}
		})
	default:
		page.err = fmt.Errorf("unsupported tunnel confirmation: %s", page.command)
		return nil
	}
}

func (page *TunnelPage) startOperation(command TunnelCommand, targetID, title string, run func(context.Context) tunnelOperationMsg) tea.Cmd {
	ctx, cancel := context.WithTimeout(page.ctx, tunnelOperationTimeout)
	page.command, page.targetID = command, targetID
	page.operationCancel = cancel
	page.operationCancelled = false
	progress := component.NewProgress(title)
	page.progress = &progress
	page.overlay = tunnelOverlayOperation
	page.err = nil
	return func() tea.Msg { return run(ctx) }
}

func (page *TunnelPage) finishOperation(msg tunnelOperationMsg) tea.Cmd {
	if page.operationCancel != nil {
		page.operationCancel()
	}
	page.operationCancel = nil
	if page.operationCancelled {
		page.operationCancelled = false
		page.overlay = tunnelOverlayNone
		page.progress = nil
		page.notice = "Operation cancelled"
		return nil
	}
	page.overlay = tunnelOverlayNone
	page.progress = nil
	if msg.err != nil {
		page.err = msg.err
		return nil
	}
	switch msg.command {
	case TunnelConfigure, TunnelEnable, TunnelDisable:
		page.dashboard = msg.dashboard
		page.reloadDashboard()
		page.notice = "Tunnel configuration updated"
	case TunnelSync:
		page.reloadDashboard()
		page.notice = "Tunnel metadata synced"
	case TunnelAdminKeySet, TunnelAdminKeyVerify:
		page.reloadDashboard()
		page.notice = fmt.Sprintf("Admin key verified for %d tunnel(s) · %s", msg.count, tunnelScopeLabel(msg.scope))
	case TunnelManagedRefresh:
		if msg.targetID == "" {
			page.items = append([]tunnel.Metadata(nil), msg.items...)
			page.notice = fmt.Sprintf("Refreshed %d managed tunnel(s)", len(page.items))
		} else {
			page.upsertMetadata(msg.result.Metadata)
			page.notice = "Managed tunnel refreshed"
		}
		_ = page.reloadManagedBrowser()
	case TunnelManagedUpdate:
		if page.managedUpdateFetch {
			page.managedUpdateFetch = false
			page.form, page.managedForm = newManagedTunnelForm(msg.result.Metadata, false)
			page.overlay = tunnelOverlayForm
			return page.form.Init()
		}
		page.upsertMetadata(msg.result.Metadata)
		_ = page.reloadManagedBrowser()
		page.notice = "Managed tunnel updated"
	case TunnelManagedCreate:
		page.upsertMetadata(msg.result.Metadata)
		_ = page.reloadManagedBrowser()
		page.notice = "Managed tunnel created"
		return func() tea.Msg { return NavigateMsg{Path: []string{"tunnels", msg.result.Metadata.ID}} }
	case TunnelManagedConfigure:
		page.upsertMetadata(msg.result.Metadata)
		_ = page.reloadManagedBrowser()
		page.notice = "Managed tunnel configured for runtime"
	case TunnelManagedDelete:
		page.removeMetadata(msg.targetID)
		page.resourceID = ""
		_ = page.reloadManagedBrowser()
		page.notice = "Managed tunnel deleted"
		return func() tea.Msg { return NavigateMsg{Path: []string{"tunnels"}} }
	}
	return nil
}

func (page *TunnelPage) cancelOperation() {
	if page.operationCancel != nil {
		page.operationCancel()
	}
	page.operationCancelled = true
	page.managedUpdateFetch = false
	page.overlay = tunnelOverlayNone
	page.progress = nil
	page.notice = "Operation cancellation requested"
}

func (page *TunnelPage) closeOverlay() {
	page.overlay = tunnelOverlayNone
	page.form = component.Form{}
	page.confirm = component.ConfirmButtons{}
	page.runtimeForm, page.adminForm, page.managedForm, page.configureForm = nil, nil, nil, nil
	page.managedUpdateFetch = false
	page.deleteOptions = false
}

func (page *TunnelPage) reloadDashboard() {
	if dashboard, err := application.TunnelStatus(); err == nil {
		page.dashboard = dashboard
	} else {
		page.err = err
	}
	if status, err := application.TunnelAdminKeyStatus(); err == nil {
		page.adminStatus = status
	} else if page.err == nil {
		page.err = err
	}
}

func (page *TunnelPage) reloadManagedBrowser() error {
	rows := page.managedRows()
	page.browser = component.NewBrowser(page.ctx, "Managed tunnels", rows, nil)
	if page.width > 0 && page.height > 0 {
		updated, _ := page.browser.Update(tea.WindowSizeMsg{Width: page.width, Height: page.height})
		page.browser = updated.(component.Browser)
	}
	if page.resourceID != "" {
		page.browser.OpenDetail(page.resourceID)
	}
	return nil
}

func (page *TunnelPage) managedRows() []component.Row {
	dashboard, _ := application.TunnelStatus()
	rows := make([]component.Row, 0, len(page.items))
	for _, item := range page.items {
		selected := ""
		if dashboard.Config.ID == item.ID {
			selected = "selected runtime"
		}
		rows = append(rows, component.Row{
			ID: item.ID, Title: item.Name, Description: item.ID + optionalTunnelDescription(item.Description), Meta: selected,
			Search:      strings.Join(append(append(append([]string{item.ID, item.Name, item.Description}, item.OrganizationIDs...), item.WorkspaceIDs...), item.TenantIDs...), " "),
			DetailTitle: "Managed tunnel · " + item.ID,
			DetailTabs: []component.DetailTab{
				{Title: "Overview", Content: detailFields([2]string{"ID", item.ID}, [2]string{"Name", item.Name}, [2]string{"Description", item.Description}, [2]string{"Creator", item.Creator}, [2]string{"Fetched", formatTunnelTime(item.FetchedAt)})},
				{Title: "Scope", Content: detailFields([2]string{"Organizations", joinedOrNone(item.OrganizationIDs)}, [2]string{"Workspaces", joinedOrNone(item.WorkspaceIDs)}, [2]string{"Tenants", joinedOrNone(item.TenantIDs)})},
			},
		})
	}
	return rows
}

func (page *TunnelPage) runtimeView(width int) string {
	cfg, status := page.dashboard.Config, page.dashboard.Status
	metadata := "None"
	if status.Metadata != nil {
		metadata = status.Metadata.Name
		if metadata == "" {
			metadata = status.Metadata.ID
		}
	}
	admin := "not configured"
	if page.adminStatus.Configured {
		admin = "configured · " + tunnelScopeLabel(page.adminStatus.Scope)
	}
	lines := []string{
		component.PageTitle("OpenAI Secure MCP Tunnel", width),
		detailFields(
			[2]string{"Enabled", fmt.Sprint(cfg.Enabled)}, [2]string{"Configured", fmt.Sprint(tunnel.Configured(cfg))}, [2]string{"Tunnel ID", cfg.ID},
			[2]string{"Runtime key", configuredLabel(cfg.APIKey != "")}, [2]string{"Control plane", defaultLabel(cfg.ControlPlaneBaseURL)}, [2]string{"Organization", cfg.OrganizationID},
			[2]string{"Metadata", metadata}, [2]string{"Admin", admin},
		),
		"",
		component.Muted("e Configure · Space Enable/Disable · s Sync · a Admin key · v Verify · d Remove admin · m Managed tunnels"),
		component.Muted("Live process state is handled by the Runtime surface; this page shows persisted tunnel configuration and metadata."),
	}
	return strings.Join(lines, "\n")
}

func (page *TunnelPage) confirmTitle() string {
	if page.command == TunnelAdminKeyRemove {
		return "Remove stored tunnel admin key?"
	}
	return "Delete managed tunnel " + page.targetID + "?"
}

func (page *TunnelPage) confirmDescription() string {
	if page.command == TunnelAdminKeyRemove {
		return "The admin key and verification scope will be removed. Runtime tunnel configuration is unchanged."
	}
	if page.deleteClear {
		return "The remote tunnel will be permanently deleted and the selected local runtime tunnel configuration will also be cleared."
	}
	return "The remote tunnel will be permanently deleted. Local runtime tunnel configuration will be preserved."
}

func (page *TunnelPage) upsertMetadata(value tunnel.Metadata) {
	for index := range page.items {
		if page.items[index].ID == value.ID {
			page.items[index] = value
			return
		}
	}
	page.items = append(page.items, value)
}

func (page *TunnelPage) removeMetadata(id string) {
	result := page.items[:0]
	for _, item := range page.items {
		if item.ID != id {
			result = append(result, item)
		}
	}
	page.items = result
}

func tunnelScopeLabel(scope tunnel.AdminScope) string {
	switch {
	case scope.OrganizationID != "":
		return "organization:" + scope.OrganizationID
	case scope.WorkspaceID != "":
		return "workspace:" + scope.WorkspaceID
	case scope.TenantID != "":
		return "tenant:" + scope.TenantID
	default:
		return "none"
	}
}

func configuredLabel(configured bool) string {
	if configured {
		return "configured"
	}
	return "not configured"
}

func defaultLabel(value string) string {
	if strings.TrimSpace(value) == "" {
		return "default"
	}
	return value
}

func optionalTunnelDescription(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return " · " + value
}

func formatTunnelTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Local().Format(time.RFC3339)
}
