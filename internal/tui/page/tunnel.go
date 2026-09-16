package page

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

const tunnelOperationTimeout = 30 * time.Second

type TunnelCommand string

const (
	TunnelForeground       TunnelCommand = "tunnel.foreground"
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

type tunnelOverlayKind uint8

const (
	tunnelOverlayNone tunnelOverlayKind = iota
	tunnelOverlayOperation
)

type tunnelCopyMsg struct{ err error }

type tunnelOperationMsg struct {
	command        TunnelCommand
	targetID       string
	items          []tunnel.Metadata
	adminsByTunnel map[string][]string
	result         application.ManagedTunnelResult
	err            error
}

type TunnelPage struct {
	ctx                context.Context
	resourceID         string
	section            string
	action             string
	adminProfileID     string
	adminProfiles      []application.TunnelAdminProfile
	adminsByTunnel     map[string][]string
	items              []tunnel.Metadata
	browser            component.Browser
	detail             component.DetailPage
	overlay            tunnelOverlayKind
	editor             *component.Editor
	progress           *component.Progress
	command            TunnelCommand
	targetID           string
	operationCancel    context.CancelFunc
	operationCancelled bool
	managedForm        *managedTunnelFormData
	pendingInit        tea.Cmd
	configureForm      *managedConfigureFormData
	deleteForm         *managedDeleteFormData
	notice             string
	err                error
	width              int
	height             int
}

func NewManagedTunnels(ctx context.Context, resourceID string) (*TunnelPage, error) {
	return NewManagedTunnelsRouteAction(ctx, resourceID, "", "")
}

func NewManagedTunnelsRoute(ctx context.Context, resourceID, section string) (*TunnelPage, error) {
	return NewManagedTunnelsRouteAction(ctx, resourceID, section, "")
}

func NewManagedTunnelsRouteAction(ctx context.Context, resourceID, section, action string) (*TunnelPage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	items, err := config.ListTunnelMetadata()
	if err != nil {
		return nil, err
	}
	adminProfiles, err := application.TunnelAdminProfiles()
	if err != nil {
		return nil, err
	}
	page := &TunnelPage{ctx: ctx, resourceID: strings.TrimSpace(resourceID), section: strings.TrimSpace(section), action: strings.TrimSpace(action), items: items, adminProfiles: adminProfiles, adminsByTunnel: map[string][]string{}}
	if (page.action == "create" || page.action == "edit" || page.action == "delete") && !hasManagedAdminProfile(adminProfiles, true) {
		return nil, fmt.Errorf("no tunnel admin profile has verified Manage access")
	}
	if page.action == "configure" && !hasManagedAdminProfile(adminProfiles, false) {
		return nil, fmt.Errorf("no tunnel admin profile has verified Read access")
	}
	if page.action == "" {
		if err := page.reloadManagedBrowser(); err != nil {
			return nil, err
		}
	}
	if page.action != "" {
		if err := page.initManagedEditorRoute(); err != nil {
			return nil, err
		}
	}
	return page, nil
}

func NewManagedTunnelsForAdmin(ctx context.Context, profileID string) (*TunnelPage, error) {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		return nil, fmt.Errorf("admin profile id is required")
	}
	page, err := NewManagedTunnelsRouteAction(ctx, "", "", "")
	if err != nil {
		return nil, err
	}
	var scoped []application.TunnelAdminProfile
	for _, profile := range page.adminProfiles {
		if profile.ID != profileID {
			continue
		}
		if !profile.ReadAccess && !profile.ManageAccess {
			return nil, fmt.Errorf("admin profile %q is missing verified Read access", profileID)
		}
		scoped = append(scoped, profile)
		break
	}
	if len(scoped) == 0 {
		return nil, fmt.Errorf("admin profile %q not found", profileID)
	}
	page.adminProfileID = profileID
	page.adminProfiles = scoped
	page.items = nil
	page.adminsByTunnel = map[string][]string{}
	if err := page.reloadManagedBrowser(); err != nil {
		return nil, err
	}
	cmd, err := page.openCommand(TunnelManagedRefresh, "")
	if err != nil {
		return nil, err
	}
	page.pendingInit = cmd
	return page, nil
}

func hasManagedAdminProfile(profiles []application.TunnelAdminProfile, manage bool) bool {
	for _, profile := range profiles {
		if manage && profile.ManageAccess || !manage && (profile.ReadAccess || profile.ManageAccess) {
			return true
		}
	}
	return false
}

func (page *TunnelPage) Init() tea.Cmd {
	if page != nil && page.pendingInit != nil {
		cmd := page.pendingInit
		page.pendingInit = nil
		return cmd
	}
	if page != nil && page.editor != nil {
		return page.editor.Init()
	}
	return nil
}

func (page *TunnelPage) OverlayActive() bool {
	return page != nil && page.overlay != tunnelOverlayNone
}

func (page *TunnelPage) InputActive() bool {
	return page != nil && (page.editor != nil || page.resourceID == "" && page.browser.InputActive())
}

func (page *TunnelPage) Dirty() bool { return page != nil && page.editor != nil && page.editor.Dirty() }
func (page *TunnelPage) Submitting() bool {
	return page != nil && page.editor != nil && page.editor.Submitting()
}

func (page *TunnelPage) Notice() string {
	if page == nil {
		return ""
	}
	return page.notice
}

func (page *TunnelPage) SetNotice(value string) {
	if page != nil {
		page.notice = strings.TrimSpace(value)
	}
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
		if page.editor != nil {
			page.resizeEditor()
			return page, nil
		}
		var cmd tea.Cmd
		if page.resourceID != "" {
			page.detail.Resize(msg.Width, msg.Height)
		} else {
			updated, browserCmd := page.browser.Update(msg)
			page.browser = updated.(component.Browser)
			cmd = browserCmd
		}
		return page, cmd
	case component.EditorSubmitMsg:
		if page.editor != nil {
			return page, page.submitEditor()
		}
		return page, nil
	case component.EditorCancelMsg:
		if page.editor != nil {
			return page, page.editorParentNavigation()
		}
		return page, nil
	case component.FormMouseMsg:
		if page.editor != nil {
			updated, cmd := page.editor.Update(msg)
			page.editor = &updated
			return page, cmd
		}
		return page, nil
	case TunnelCommandMsg:
		cmd, err := page.openCommand(msg.Command, msg.ResourceID)
		if err != nil {
			page.err = err
		}
		return page, cmd
	case component.BrowserOpenMsg:
		if page.resourceID == "" && msg.Row.ID != "" {
			return page, func() tea.Msg { return NavigateMsg{Path: []string{"tunnels", msg.Row.ID}} }
		}
		return page, nil
	case tea.KeyPressMsg:
		if page.editor != nil {
			updated, cmd := page.editor.Update(msg)
			page.editor = &updated
			return page, cmd
		}
		if page.resourceID == "" && page.browser.InputActive() {
			updated, cmd := page.browser.Update(msg)
			page.browser = updated.(component.Browser)
			return page, cmd
		}
		if cmd, handled := page.handleKey(msg); handled {
			return page, cmd
		}
	}
	if page.editor != nil {
		updated, cmd := page.editor.Update(message)
		page.editor = &updated
		return page, cmd
	}
	if page.resourceID != "" {
		updated, cmd := page.detail.Update(message)
		page.detail = updated
		return page, cmd
	}
	updated, cmd := page.browser.Update(message)
	page.browser = updated.(component.Browser)
	return page, cmd
}

func (page *TunnelPage) View(width, height int) string {
	if page == nil {
		return component.StateView(component.PageError, "Tunnel page unavailable", "")
	}
	page.width, page.height = width, height
	feedback := ""
	if page.err != nil {
		feedback = component.BannerWidth(page.err.Error(), component.ToneDanger, width)
	}
	content := ""
	if page.editor != nil {
		content = page.editorView(width, height)
	} else if page.resourceID != "" {
		page.detail.SetFeedback(page.notice, page.err)
		page.detail.Resize(width, height)
		content = page.detail.View()
	} else {
		feedback = page.managedFeedback(width, feedback)
		browserHeight := max(1, height-pageFeedbackHeight(feedback))
		updated, _ := page.browser.Update(tea.WindowSizeMsg{Width: width, Height: browserHeight})
		page.browser = updated.(component.Browser)
		content = prependPageFeedback(feedback, page.browser.Content())
	}
	if page.overlay == tunnelOverlayOperation {
		body := ""
		if page.progress != nil {
			body = page.progress.View()
		}
		body += "\n\n" + component.Muted("Esc cancel")
		content = component.CenterOverlay(content, component.Modal(body, overlayWidth(width, 72)), width, height)
	}
	return content
}

func (page *TunnelPage) MouseTargets(originX, originY, z int) []component.MouseTarget {
	if page == nil {
		return nil
	}
	if page.overlay == tunnelOverlayOperation {
		return []component.MouseTarget{mouseBlocker(originX, originY, page.width, page.height, z+20)}
	}
	if page.editor != nil {
		return page.editor.MouseTargets(originX, originY, z)
	}
	if page.resourceID != "" {
		return page.detail.MouseTargets(originX, originY, z)
	}
	feedback := ""
	if page.err != nil {
		feedback = component.BannerWidth(page.err.Error(), component.ToneDanger, page.width)
	}
	feedback = page.managedFeedback(page.width, feedback)
	return page.browser.MouseTargets(originX, originY+pageFeedbackHeight(feedback), z)
}

func (page *TunnelPage) handleKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "r":
		if !hasManagedAdminProfile(page.adminProfiles, false) {
			page.err = fmt.Errorf("add and verify an admin profile first (press p)")
			return nil, true
		}
		cmd, err := page.openCommand(TunnelManagedRefresh, "")
		page.err = err
		return cmd, true
	case "a":
		if !hasManagedAdminProfile(page.adminProfiles, true) {
			if len(page.adminProfiles) == 0 {
				return func() tea.Msg { return NavigateMsg{Path: []string{"admins", "create"}} }, true
			}
			page.err = fmt.Errorf("no admin profile has verified Manage access; open Admin Profiles (p) and verify")
			return nil, true
		}
		cmd, err := page.openCommand(TunnelManagedCreate, "")
		page.err = err
		return cmd, true
	case "p":
		return func() tea.Msg { return NavigateMsg{Path: []string{"admins"}} }, true
	case "t":
		if !hasManagedAdminProfile(page.adminProfiles, false) {
			page.err = fmt.Errorf("add and verify an admin profile first (press p)")
			return nil, true
		}
		if page.resourceID != "" {
			return nil, false
		}
		row, ok := page.browser.Selected()
		if !ok || strings.TrimSpace(row.ID) == "" {
			return nil, true
		}
		cmd, err := page.openCommand(TunnelManagedConfigure, row.ID)
		page.err = err
		return cmd, true
	}
	return nil, false
}

func (page *TunnelPage) openCommand(command TunnelCommand, resourceID string) (tea.Cmd, error) {
	page.err, page.notice = nil, ""
	page.command, page.targetID = command, strings.TrimSpace(resourceID)
	switch command {
	case TunnelManagedRefresh:
		id := page.targetID
		profileID := page.adminProfileID
		return page.startOperation(command, id, "Refreshing managed tunnels", func(ctx context.Context) tunnelOperationMsg {
			discovered, err := application.DiscoverManagedTunnels(ctx, profileID)
			if err != nil {
				return tunnelOperationMsg{command: command, targetID: id, err: err}
			}
			items := make([]tunnel.Metadata, 0, len(discovered))
			adminsByTunnel := make(map[string][]string, len(discovered))
			var selected tunnel.Metadata
			for _, item := range discovered {
				items = append(items, item.Metadata)
				adminsByTunnel[item.Metadata.ID] = append([]string(nil), item.AdminProfiles...)
				_, _ = config.SaveTunnelMetadata(item.Metadata)
				if item.Metadata.ID == id {
					selected = item.Metadata
				}
			}
			if id != "" && selected.ID == "" {
				return tunnelOperationMsg{command: command, targetID: id, err: fmt.Errorf("managed tunnel %q was not found", id)}
			}
			return tunnelOperationMsg{command: command, targetID: id, items: items, adminsByTunnel: adminsByTunnel, result: application.ManagedTunnelResult{Metadata: selected}}
		}), nil
	case TunnelManagedCreate:
		return func() tea.Msg { return NavigateMsg{Path: []string{"tunnels", "create"}} }, nil
	case TunnelManagedUpdate:
		if page.targetID == "" {
			return nil, fmt.Errorf("managed tunnel id is required")
		}
		return func() tea.Msg { return NavigateMsg{Path: []string{"tunnels", page.targetID, "edit"}} }, nil
	case TunnelManagedConfigure:
		if page.targetID == "" {
			return nil, fmt.Errorf("managed tunnel id is required")
		}
		return func() tea.Msg { return NavigateMsg{Path: []string{"tunnels", page.targetID, "configure"}} }, nil
	case TunnelManagedDelete:
		if page.targetID == "" {
			return nil, fmt.Errorf("managed tunnel id is required")
		}
		return func() tea.Msg { return NavigateMsg{Path: []string{"tunnels", page.targetID, "delete"}} }, nil
	default:
		return nil, fmt.Errorf("unsupported tunnel action: %s", command)
	}
}

func (page *TunnelPage) startOperation(command TunnelCommand, targetID, title string, run func(context.Context) tunnelOperationMsg) tea.Cmd {
	ctx, cancel := context.WithTimeout(page.ctx, tunnelOperationTimeout)
	page.command, page.targetID = command, targetID
	page.operationCancel = cancel
	page.operationCancelled = false
	page.err = nil
	return beginOperation("tunnel.managed.save", "Managed Tunnel", title, func() tea.Msg { return run(ctx) })
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
		return func() tea.Msg {
			return cancelledOperation("tunnel.managed.save", "Managed Tunnel", "Operation cancelled")
		}
	}
	page.overlay = tunnelOverlayNone
	page.progress = nil
	if msg.err != nil {
		page.err = nil
		if page.editor != nil {
			page.editor.SetSubmitting(false)
		}
		return func() tea.Msg { return OperationResult("tunnel.managed.save", "Managed Tunnel", "", msg.err) }
	}
	var notice string
	switch msg.command {
	case TunnelManagedRefresh:
		if len(msg.adminsByTunnel) > 0 {
			if msg.targetID == "" {
				page.adminsByTunnel = msg.adminsByTunnel
			} else {
				if page.adminsByTunnel == nil {
					page.adminsByTunnel = map[string][]string{}
				}
				page.adminsByTunnel[msg.targetID] = append([]string(nil), msg.adminsByTunnel[msg.targetID]...)
			}
		}
		if msg.targetID == "" {
			page.items = append([]tunnel.Metadata(nil), msg.items...)
			notice = fmt.Sprintf("Refreshed %d managed tunnel(s)", len(page.items))
		} else {
			page.upsertMetadata(msg.result.Metadata)
			notice = "Managed tunnel refreshed"
		}
		_ = page.reloadManagedBrowser()
	case TunnelManagedUpdate:
		page.upsertMetadata(msg.result.Metadata)
		if page.managedForm != nil && page.managedForm.AdminProfileID != "" {
			page.rememberAdminProfiles(msg.result.Metadata.ID, page.managedForm.AdminProfileID)
		}
		_ = page.reloadManagedBrowser()
		notice = "Managed tunnel updated"
		if page.editor != nil && page.action == "edit" {
			page.acceptManagedEditorSuccess(msg.result.Metadata)
			return page.managedEditorSuccess(notice, msg.result.Metadata.ID)
		}
	case TunnelManagedCreate:
		page.upsertMetadata(msg.result.Metadata)
		if page.managedForm != nil && page.managedForm.AdminProfileID != "" {
			page.rememberAdminProfiles(msg.result.Metadata.ID, page.managedForm.AdminProfileID)
		}
		_ = page.reloadManagedBrowser()
		notice = "Managed tunnel created"
		if page.editor != nil && page.action == "create" {
			page.acceptManagedEditorSuccess(msg.result.Metadata)
			return page.managedEditorSuccess(notice, msg.result.Metadata.ID)
		}
		return withOperation("tunnel.managed.save", "Managed Tunnel", notice, func() tea.Msg { return NavigateMsg{Path: []string{"tunnels", msg.result.Metadata.ID}} })
	case TunnelManagedConfigure:
		page.upsertMetadata(msg.result.Metadata)
		if page.configureForm != nil && page.configureForm.AdminProfileID != "" {
			page.rememberAdminProfiles(msg.result.Metadata.ID, page.configureForm.AdminProfileID)
		}
		_ = page.reloadManagedBrowser()
		notice = "Managed tunnel attached to runtime"
		if page.editor != nil && page.action == "configure" {
			page.acceptManagedConfigureSuccess()
			return page.managedEditorSuccess(notice, msg.result.Metadata.ID)
		}
	case TunnelManagedDelete:
		page.removeMetadata(msg.targetID)
		if page.adminsByTunnel != nil {
			delete(page.adminsByTunnel, msg.targetID)
		}
		page.resourceID = ""
		_ = page.reloadManagedBrowser()
		return withOperation("tunnel.managed.save", "Managed Tunnel", "Managed tunnel deleted", func() tea.Msg { return NavigateMsg{Path: []string{"tunnels"}, Replace: true} })
	}
	if notice != "" {
		return func() tea.Msg { return OperationResult("tunnel.managed.save", "Managed Tunnel", notice, nil) }
	}
	return nil
}

func (page *TunnelPage) cancelOperation() {
	if page.operationCancel != nil {
		page.operationCancel()
	}
	page.operationCancelled = true
	page.overlay = tunnelOverlayNone
	page.progress = nil
}

func (page *TunnelPage) reloadManagedBrowser() error {
	if page.resourceID != "" {
		return page.syncManagedDetail()
	}
	helpExpanded := page.browser.HelpExpanded()
	rows := page.managedRows()
	bindings := []key.Binding{component.Binding([]string{"p"}, "p", "admins")}
	if hasManagedAdminProfile(page.adminProfiles, false) {
		refreshLabel := "refresh all"
		if page.adminProfileID != "" {
			refreshLabel = "refresh"
		}
		bindings = append(bindings, component.Binding([]string{"r"}, "r", refreshLabel), component.Binding([]string{"t"}, "t", "attach"))
	}
	if hasManagedAdminProfile(page.adminProfiles, true) {
		bindings = append(bindings, component.Binding([]string{"a"}, "a", "add"))
	} else if len(page.adminProfiles) == 0 {
		bindings = append(bindings, component.Binding([]string{"a"}, "a", "add profile"))
	}
	page.browser = component.NewBrowser(page.ctx, "Managed tunnels", rows, nil).WithTitleVisible(false).WithHelpBindings(bindings...)
	page.browser.SetHelpExpanded(helpExpanded)
	if page.width > 0 && page.height > 0 {
		updated, _ := page.browser.Update(tea.WindowSizeMsg{Width: page.width, Height: page.height})
		page.browser = updated.(component.Browser)
	}
	return nil
}

func (page *TunnelPage) managedFeedback(width int, feedback string) string {
	if page == nil || strings.TrimSpace(page.notice) == "" {
		return feedback
	}
	return prependPageFeedback(feedback, component.BannerWidth(page.notice, component.ToneSuccess, width))
}

func (page *TunnelPage) managedRows() []component.Row {
	attached := attachedTunnelIDs()
	ids := make([]string, len(page.items))
	names := make([]string, len(page.items))
	for i, item := range page.items {
		ids[i], names[i] = item.ID, item.Name
	}
	labels := tunnel.UniqueLabels(ids, names)
	rows := make([]component.Row, 0, len(page.items))
	for i, item := range page.items {
		meta := strings.Join(page.adminsByTunnel[item.ID], ",")
		if attached[item.ID] {
			if meta != "" {
				meta = "attached · " + meta
			} else {
				meta = "attached"
			}
		}
		description := strings.TrimSpace(item.Description)
		if admins := page.adminsByTunnel[item.ID]; len(admins) > 0 {
			if description != "" {
				description += " · "
			}
			description += "admin=" + strings.Join(admins, ",")
		}
		rows = append(rows, component.Row{
			ID: item.ID, Title: labels[i], Description: description, Meta: meta,
			Search: strings.Join(append(append(append(append([]string{item.ID, item.Name, item.Description}, item.OrganizationIDs...), item.WorkspaceIDs...), item.TenantIDs...), page.adminsByTunnel[item.ID]...), " "),
		})
	}
	return rows
}

func (page *TunnelPage) syncManagedDetail() error {
	var item *tunnel.Metadata
	for index := range page.items {
		if page.items[index].ID == page.resourceID {
			item = &page.items[index]
			break
		}
	}
	if item == nil {
		return fmt.Errorf("managed tunnel not found in local cache: %s", page.resourceID)
	}
	content := ""
	switch page.section {
	case "":
		content = detailFields(
			[2]string{"ID", item.ID}, [2]string{"Name", item.Name}, [2]string{"Description", item.Description},
			[2]string{"Creator", item.Creator}, [2]string{"Admin profiles", joinedOrNone(page.adminsByTunnel[item.ID])},
			[2]string{"Fetched", formatTunnelTime(item.FetchedAt)},
		)
	case "scope":
		content = detailFields([2]string{"Organizations", joinedOrNone(item.OrganizationIDs)}, [2]string{"Workspaces", joinedOrNone(item.WorkspaceIDs)}, [2]string{"Tenants", joinedOrNone(item.TenantIDs)})
	default:
		return fmt.Errorf("unsupported managed tunnel child section: %s", page.section)
	}
	attached := attachedTunnelIDs()
	meta := ""
	if attached[item.ID] {
		meta = "attached"
	}
	detailTitle := tunnel.DisplayLabel(item.ID, item.Name)
	if page.section == "scope" {
		detailTitle = "Scope"
	}
	page.detail = component.NewDetailPage(detailTitle, meta, content).WithTitleVisible(false)
	bindings := make([]component.DetailPageBinding, 0, 5)
	if page.section == "" {
		bindings = append(bindings, component.DetailPageBinding{Key: "s", Desc: "scope", Message: NavigateMsg{Path: []string{"tunnels", item.ID, "scope"}}})
	}
	if hasManagedAdminProfile(page.adminProfiles, false) {
		bindings = append(bindings, component.DetailPageBinding{Key: "r", Desc: "refresh", Message: TunnelCommandMsg{Command: TunnelManagedRefresh, ResourceID: item.ID}})
		if !attached[item.ID] {
			bindings = append(bindings, component.DetailPageBinding{Key: "t", Desc: "attach", Message: TunnelCommandMsg{Command: TunnelManagedConfigure, ResourceID: item.ID}})
		}
	}
	if hasManagedAdminProfile(page.adminProfiles, true) {
		bindings = append(bindings, component.DetailPageBinding{Key: "e", Desc: "update", Message: TunnelCommandMsg{Command: TunnelManagedUpdate, ResourceID: item.ID}}, component.DetailPageBinding{Key: "d", Desc: "delete", Message: TunnelCommandMsg{Command: TunnelManagedDelete, ResourceID: item.ID}})
	}
	page.detail.SetBindings(bindings...)
	if page.width > 0 && page.height > 0 {
		page.detail.Resize(page.width, page.height)
	}
	return nil
}

func (page *TunnelPage) rememberAdminProfiles(id string, profiles ...string) {
	if page == nil || strings.TrimSpace(id) == "" {
		return
	}
	if page.adminsByTunnel == nil {
		page.adminsByTunnel = map[string][]string{}
	}
	seen := map[string]bool{}
	next := make([]string, 0, len(profiles)+len(page.adminsByTunnel[id]))
	for _, profile := range append(append([]string{}, page.adminsByTunnel[id]...), profiles...) {
		profile = strings.TrimSpace(profile)
		if profile == "" || seen[profile] {
			continue
		}
		seen[profile] = true
		next = append(next, profile)
	}
	page.adminsByTunnel[id] = next
}

func (page *TunnelPage) profilesForManaged(id string, manage bool) []application.TunnelAdminProfile {
	allowed := page.adminsByTunnel[strings.TrimSpace(id)]
	if len(allowed) == 0 {
		if profile := attachedAdminProfile(id); profile != "" {
			allowed = []string{profile}
		}
	}
	if len(allowed) == 0 {
		return page.adminProfiles
	}
	allow := make(map[string]bool, len(allowed))
	for _, profile := range allowed {
		allow[profile] = true
	}
	filtered := make([]application.TunnelAdminProfile, 0, len(page.adminProfiles))
	for _, profile := range page.adminProfiles {
		if !allow[profile.ID] {
			continue
		}
		if manage && !profile.ManageAccess || !manage && !profile.ReadAccess && !profile.ManageAccess {
			continue
		}
		filtered = append(filtered, profile)
	}
	return filtered
}

func attachedAdminProfile(id string) string {
	items, err := application.LocalTunnels()
	if err != nil {
		return ""
	}
	id = strings.TrimSpace(id)
	for _, item := range items {
		if item.ID == id {
			return strings.TrimSpace(item.AdminProfileID)
		}
	}
	return ""
}

func attachedTunnelIDs() map[string]bool {
	items, err := application.LocalTunnels()
	if err != nil {
		return map[string]bool{}
	}
	result := make(map[string]bool, len(items))
	for _, item := range items {
		result[item.ID] = true
	}
	return result
}

func tunnelYesNo(value bool) string {
	if value {
		return component.ToneText("yes", component.ToneSuccess)
	}
	return component.Muted("no")
}

func tunnelConfiguredIndicator(value bool) string {
	if value {
		return component.ToneText("configured", component.ToneSuccess)
	}
	return component.Muted("not configured")
}

func valueOrNone(value string) string {
	if strings.TrimSpace(value) == "" {
		return component.Muted("None")
	}
	return value
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

func defaultLabel(value string) string {
	if strings.TrimSpace(value) == "" {
		return "default"
	}
	return value
}

func formatTunnelTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.Local().Format(time.RFC3339)
}
