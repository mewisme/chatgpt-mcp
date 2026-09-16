package page

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
)

type TunnelAdminCommand string

const (
	TunnelAdminRefresh TunnelAdminCommand = "tunnel.admin.refresh"
	TunnelAdminAdd     TunnelAdminCommand = "tunnel.admin.add"
	TunnelAdminUpdate  TunnelAdminCommand = "tunnel.admin.update"
	TunnelAdminVerify  TunnelAdminCommand = "tunnel.admin.verify"
	TunnelAdminRemove  TunnelAdminCommand = "tunnel.admin.remove"
)

type TunnelAdminCommandMsg struct {
	Command    TunnelAdminCommand
	ResourceID string
}

type tunnelAdminResultMsg struct {
	command TunnelAdminCommand
	id      string
	item    application.TunnelAdminProfile
	items   []application.TunnelAdminProfile
	count   int
	err     error
}

type TunnelAdminsPage struct {
	ctx        context.Context
	resourceID string
	action     string
	items      []application.TunnelAdminProfile
	browser    component.Browser
	detail     component.DetailPage
	editor     *component.Editor
	form       *tunnelAdminProfileFormData
	confirm    component.ConfirmButtons
	confirmID  string
	notice     string
	err        error
	width      int
	height     int
}

func NewTunnelAdmins(ctx context.Context, resourceID, action string) (*TunnelAdminsPage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	page := &TunnelAdminsPage{ctx: ctx, resourceID: strings.TrimSpace(resourceID), action: strings.TrimSpace(action)}
	if page.action != "" {
		if err := page.initEditor(); err != nil {
			return nil, err
		}
		return page, nil
	}
	if err := page.reload(); err != nil {
		return nil, err
	}
	return page, nil
}

func (page *TunnelAdminsPage) Init() tea.Cmd {
	if page != nil && page.editor != nil {
		return page.editor.Init()
	}
	return nil
}
func (page *TunnelAdminsPage) OverlayActive() bool { return page != nil && page.confirmID != "" }
func (page *TunnelAdminsPage) InputActive() bool {
	return page != nil && (page.editor != nil || page.resourceID == "" && page.action == "" && page.browser.InputActive())
}
func (page *TunnelAdminsPage) Dirty() bool {
	return page != nil && page.editor != nil && page.editor.Dirty()
}
func (page *TunnelAdminsPage) Submitting() bool {
	return page != nil && page.editor != nil && page.editor.Submitting()
}
func (page *TunnelAdminsPage) Notice() string {
	if page == nil {
		return ""
	}
	return page.notice
}
func (page *TunnelAdminsPage) SetNotice(value string) {
	if page != nil {
		page.notice = strings.TrimSpace(value)
	}
}

func (page *TunnelAdminsPage) Update(message tea.Msg) (Model, tea.Cmd) {
	if page == nil {
		return page, nil
	}
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		page.width, page.height = msg.Width, msg.Height
		if page.editor != nil {
			page.editor.Resize(msg.Width, msg.Height)
		} else if page.resourceID != "" {
			page.detail.Resize(msg.Width, msg.Height)
		} else {
			return page, page.resizeBrowser()
		}
		return page, nil
	case component.EditorSubmitMsg:
		return page, page.submitEditor()
	case component.EditorCancelMsg:
		return page, page.editorParentNavigation()
	case component.BrowserOpenMsg:
		if page.editor == nil && page.resourceID == "" && msg.Row.ID != "" {
			id := msg.Row.ID
			return page, func() tea.Msg { return NavigateMsg{Path: []string{"admins", id}} }
		}
		return page, nil
	case component.ConfirmChoiceMsg:
		if page.confirmID != "" {
			page.confirm.Select(msg.Affirmative)
			return page, page.updateConfirm(tea.KeyPressMsg{Code: tea.KeyEnter})
		}
		return page, nil
	case TunnelAdminCommandMsg:
		return page, page.runCommand(msg.Command, msg.ResourceID)
	case tunnelAdminResultMsg:
		return page, page.finishCommand(msg)
	case tea.KeyPressMsg:
		if page.editor != nil {
			updated, cmd := page.editor.Update(msg)
			page.editor = &updated
			return page, cmd
		}
		if page.confirmID != "" {
			return page, page.updateConfirm(msg)
		}
		if page.resourceID == "" && page.browser.InputActive() {
			updated, cmd := page.browser.Update(msg)
			page.browser = updated.(component.Browser)
			return page, cmd
		}
		if page.resourceID == "" {
			switch msg.String() {
			case "r":
				return page, page.runCommand(TunnelAdminRefresh, "")
			case "a":
				return page, func() tea.Msg { return NavigateMsg{Path: []string{"admins", "create"}} }
			case "m":
				row, ok := page.browser.Selected()
				if !ok || strings.TrimSpace(row.ID) == "" {
					return page, nil
				}
				id := row.ID
				return page, func() tea.Msg { return NavigateMsg{Path: []string{"admins", id, "managed"}} }
			}
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

func (page *TunnelAdminsPage) View(width, height int) string {
	if page == nil {
		return component.StateView(component.PageError, "Admin profiles unavailable", "")
	}
	page.width, page.height = width, height
	if page.editor != nil {
		page.editor.Resize(width, height)
		return page.editor.View()
	}
	var content string
	if page.resourceID != "" {
		page.detail.SetFeedback(page.notice, page.err)
		page.detail.Resize(width, height)
		content = page.detail.View()
	} else {
		content = page.listView(width, height)
	}
	if page.confirmID != "" {
		modalWidth := overlayWidth(width, 64)
		body := confirmOverlayBody(page.confirm, "Remove admin profile "+page.confirmID+"?", "The management credential is removed locally. Attached tunnels that still reference it must be detached first.", modalWidth)
		content = component.CenterOverlay(content, component.Modal(body, modalWidth), width, height)
	}
	return content
}

func (page *TunnelAdminsPage) MouseTargets(originX, originY, z int) []component.MouseTarget {
	if page == nil {
		return nil
	}
	if page.editor != nil {
		return page.editor.MouseTargets(originX, originY, z)
	}
	if page.confirmID != "" {
		return confirmOverlayMouseTargets(page.confirm, "Remove admin profile "+page.confirmID+"?", "The management credential is removed locally. Attached tunnels that still reference it must be detached first.", overlayWidth(page.width, 64), page.width, page.height, originX, originY, z+20)
	}
	if page.resourceID != "" {
		return page.detail.MouseTargets(originX, originY, z)
	}
	headerHeight := lipgloss.Height(page.listHeader(page.width))
	help := page.browser.HelpView()
	bodyHeight := max(1, page.height-headerHeight)
	layout := component.NewSectionLayout("", "", page.listFeedback(page.width), page.width, bodyHeight, lipgloss.Height(help))
	browserY := originY + headerHeight + layout.BodyY
	targets := page.browser.MouseTargets(originX, browserY, z)
	helpY := originY + headerHeight + bodyHeight - lipgloss.Height(help)
	return append(targets, page.browser.HelpMouseTargets(originX, helpY, z+2)...)
}

func (page *TunnelAdminsPage) reload() error {
	items, err := application.TunnelAdminProfiles()
	if err != nil {
		return err
	}
	page.items = items
	if page.resourceID != "" {
		return page.syncDetail()
	}
	helpExpanded := page.browser.HelpExpanded()
	selected, _ := page.browser.Selected()
	page.browser = component.NewBrowser(page.ctx, "Admin profiles", page.rows(), nil).WithTitleVisible(false).WithExternalHelp(true)
	page.browser.SetHelpBindings(
		component.Binding([]string{"r"}, "r", "refresh"),
		component.Binding([]string{"a"}, "a", "add"),
		component.Binding([]string{"m"}, "m", "managed"),
	)
	page.browser.SetHelpExpanded(helpExpanded)
	if selected.ID != "" {
		page.browser.SelectID(selected.ID)
	}
	if page.width > 0 && page.height > 0 {
		page.resizeBrowser()
	}
	return nil
}

func (page *TunnelAdminsPage) rows() []component.Row {
	rows := make([]component.Row, 0, len(page.items))
	for _, item := range page.items {
		rows = append(rows, component.Row{
			ID: item.ID, Title: item.ID, Description: adminProfileScopeSummary(item), Meta: adminProfileAccessSummary(item),
			Search: strings.Join([]string{item.ID, item.OrganizationID, item.WorkspaceID, item.TenantID, item.ControlPlaneBaseURL}, " "),
		})
	}
	return rows
}

func (page *TunnelAdminsPage) syncDetail() error {
	item, err := page.profile(page.resourceID)
	if err != nil {
		return err
	}
	content := detailFields(
		[2]string{"ID", item.ID},
		[2]string{"Key", tunnelConfiguredIndicator(item.KeyConfigured)},
		[2]string{"Access", adminProfileAccessSummary(item)},
		[2]string{"Scope", adminProfileScopeSummary(item)},
		[2]string{"Control plane", defaultLabel(item.ControlPlaneBaseURL)},
	)
	page.detail = component.NewDetailPage(item.ID, adminProfileAccessSummary(item), content).WithTitleVisible(false)
	page.detail.SetBindings(
		component.DetailPageBinding{Key: "e", Desc: "edit", Message: TunnelAdminCommandMsg{Command: TunnelAdminUpdate, ResourceID: item.ID}},
		component.DetailPageBinding{Key: "v", Desc: "verify", Message: TunnelAdminCommandMsg{Command: TunnelAdminVerify, ResourceID: item.ID}},
		component.DetailPageBinding{Key: "d", Desc: "remove", Message: TunnelAdminCommandMsg{Command: TunnelAdminRemove, ResourceID: item.ID}},
		component.DetailPageBinding{Key: "m", Desc: "managed", Message: NavigateMsg{Path: []string{"admins", item.ID, "managed"}}},
	)
	if page.width > 0 && page.height > 0 {
		page.detail.Resize(page.width, page.height)
	}
	return nil
}

func (page *TunnelAdminsPage) profile(id string) (application.TunnelAdminProfile, error) {
	for _, item := range page.items {
		if item.ID == id {
			return item, nil
		}
	}
	return application.TunnelAdminProfile{}, fmt.Errorf("admin profile %q not found", id)
}

func (page *TunnelAdminsPage) initEditor() error {
	create := page.action == "create"
	if create && page.resourceID != "" {
		return fmt.Errorf("admin profile create editor does not accept a resource")
	}
	if !create && (page.action != "edit" || page.resourceID == "") {
		return fmt.Errorf("unsupported admin profile editor action %q", page.action)
	}
	var profile application.TunnelAdminProfile
	if !create {
		items, err := application.TunnelAdminProfiles()
		if err != nil {
			return err
		}
		page.items = items
		profile, err = page.profile(page.resourceID)
		if err != nil {
			return err
		}
	}
	editor, data := newTunnelAdminProfileEditor(profile, create)
	page.editor, page.form = &editor, data
	if page.width > 0 && page.height > 0 {
		page.editor.Resize(page.width, page.height)
	}
	return nil
}

func (page *TunnelAdminsPage) submitEditor() tea.Cmd {
	if page == nil || page.editor == nil {
		return nil
	}
	if err := page.editor.Validate(); err != nil {
		page.editor.SetFeedback("", err)
		return nil
	}
	page.editor.SetFeedback("", nil)
	create := page.action == "create"
	id := page.resourceID
	if create {
		id = page.form.ID
	}
	admin, err := adminProfileFromForm(page.form, id)
	if err != nil {
		page.editor.SetFeedback("", err)
		return nil
	}
	if !create && admin.AdminKey == "" {
		// keep blank key so UpdateTunnelAdminProfile preserves the secret
	} else if create && admin.AdminKey == "" {
		page.editor.SetFeedback("", fmt.Errorf("admin API key is required"))
		return nil
	}
	page.editor.SetSubmitting(true)
	ctx := page.ctx
	return func() tea.Msg {
		msg := tunnelAdminResultMsg{id: admin.ID}
		if create {
			msg.command = TunnelAdminAdd
			msg.item, msg.count, msg.err = application.AddTunnelAdminProfile(ctx, admin)
		} else {
			msg.command = TunnelAdminUpdate
			msg.item, msg.count, msg.err = application.UpdateTunnelAdminProfile(ctx, admin)
		}
		return msg
	}
}

func (page *TunnelAdminsPage) editorParentNavigation() tea.Cmd {
	if page != nil && page.resourceID != "" {
		id := page.resourceID
		return func() tea.Msg { return NavigateMsg{Path: []string{"admins", id}} }
	}
	return func() tea.Msg { return NavigateMsg{Path: []string{"admins"}} }
}

func (page *TunnelAdminsPage) runCommand(command TunnelAdminCommand, id string) tea.Cmd {
	id = strings.TrimSpace(id)
	switch command {
	case TunnelAdminUpdate:
		if id == "" {
			page.err = fmt.Errorf("admin profile id is required")
			return nil
		}
		return func() tea.Msg { return NavigateMsg{Path: []string{"admins", id, "edit"}} }
	case TunnelAdminRemove:
		page.confirmID = id
		page.confirm = component.NewConfirmButtons("Remove", "Cancel", false)
		return nil
	case TunnelAdminRefresh, TunnelAdminVerify:
	default:
		page.err = fmt.Errorf("unsupported admin profile action: %s", command)
		return nil
	}
	ctx := page.ctx
	return func() tea.Msg {
		msg := tunnelAdminResultMsg{command: command, id: id}
		switch command {
		case TunnelAdminRefresh:
			msg.items, msg.err = application.TunnelAdminProfiles()
		case TunnelAdminVerify:
			msg.item, msg.count, msg.err = application.VerifyTunnelAdminProfile(ctx, id)
		}
		return msg
	}
}

func (page *TunnelAdminsPage) finishCommand(msg tunnelAdminResultMsg) tea.Cmd {
	if page.editor != nil {
		page.editor.SetSubmitting(false)
	}
	if msg.err != nil {
		page.err = msg.err
		if page.editor != nil {
			page.editor.SetFeedback("", msg.err)
		}
		return nil
	}
	page.err = nil
	switch msg.command {
	case TunnelAdminAdd, TunnelAdminUpdate:
		page.notice = fmt.Sprintf("Admin profile %s verified · %d tunnel(s) readable", msg.item.ID, msg.count)
		page.acceptAdminEditorSuccess()
		return tea.Batch(
			func() tea.Msg { return NavigateMsg{Path: []string{"admins", msg.item.ID}, Replace: true} },
			func() tea.Msg {
				return ToastMsg{Title: "Admin Profile", Message: page.notice, Tone: component.ToneSuccess}
			},
		)
	case TunnelAdminVerify:
		for i := range page.items {
			if page.items[i].ID == msg.item.ID {
				page.items[i] = msg.item
			}
		}
		page.notice = fmt.Sprintf("Verified %s · %d tunnel(s) readable", msg.item.ID, msg.count)
	case TunnelAdminRemove:
		items := page.items[:0]
		for _, item := range page.items {
			if item.ID != msg.id {
				items = append(items, item)
			}
		}
		page.items = items
		page.notice = "Admin profile removed"
		if page.resourceID == msg.id {
			return func() tea.Msg { return NavigateMsg{Path: []string{"admins"}, Replace: true} }
		}
	case TunnelAdminRefresh:
		page.items = msg.items
		page.notice = fmt.Sprintf("Refreshed %d admin profile(s)", len(page.items))
	}
	if page.resourceID != "" {
		page.err = page.syncDetail()
		return nil
	}
	selected, _ := page.browser.Selected()
	return page.browser.ReplaceRows(page.rows(), selected.ID)
}

func (page *TunnelAdminsPage) acceptAdminEditorSuccess() {
	if page == nil || page.editor == nil {
		return
	}
	// Keep the editor mounted until navigation replaces the page. Clearing it
	// left create/update routes with a nil browser and panicked in listView.
	page.editor.Accept()
	page.editor.SetSubmitting(false)
}

func (page *TunnelAdminsPage) updateConfirm(msg tea.KeyPressMsg) tea.Cmd {
	if msg.String() == "esc" || msg.String() == "enter" && !page.confirm.AffirmativeSelected() {
		page.confirmID = ""
		page.confirm = component.ConfirmButtons{}
		return nil
	}
	if msg.String() != "enter" {
		return page.confirm.Update(msg)
	}
	id := page.confirmID
	page.confirmID = ""
	page.confirm = component.ConfirmButtons{}
	ctx := page.ctx
	return func() tea.Msg {
		err := application.RemoveTunnelAdminProfile(ctx, id)
		return tunnelAdminResultMsg{command: TunnelAdminRemove, id: id, err: err}
	}
}

func (page *TunnelAdminsPage) listView(width, height int) string {
	if page.resourceID != "" {
		return ""
	}
	header := page.listHeader(width)
	bodyHeight := max(1, height-lipgloss.Height(header))
	updated, _ := page.browser.Update(tea.WindowSizeMsg{Width: width, Height: bodyHeight})
	page.browser = updated.(component.Browser)
	help := page.browser.HelpView()
	layout := component.NewSectionLayout("", "", page.listFeedback(width), width, bodyHeight, lipgloss.Height(help))
	updated, _ = page.browser.Update(tea.WindowSizeMsg{Width: width, Height: layout.BodyHeight})
	page.browser = updated.(component.Browser)
	return header + "\n" + component.BottomHelp(layout.View(page.browser.BodyContent()), help, width, bodyHeight)
}

func (page *TunnelAdminsPage) listHeader(width int) string {
	verified := 0
	for _, item := range page.items {
		if item.ReadAccess || item.ManageAccess {
			verified++
		}
	}
	summary := fmt.Sprintf("%d profiles · %d verified", len(page.items), verified)
	if len(page.items) == 0 {
		summary = "No admin profiles yet · press a to add one, then open Managed Tunnels"
	}
	return component.PageTitleNotice("Tunnel Admin Profiles", page.notice, width) + "\n" + component.Muted(summary)
}

func (page *TunnelAdminsPage) listFeedback(width int) string {
	if page.err == nil {
		return ""
	}
	return component.BannerWidth(page.err.Error(), component.ToneDanger, width)
}

func (page *TunnelAdminsPage) resizeBrowser() tea.Cmd {
	headerHeight := lipgloss.Height(page.listHeader(page.width))
	bodyHeight := max(1, page.height-headerHeight)
	help := page.browser.HelpView()
	layout := component.NewSectionLayout("", "", page.listFeedback(page.width), page.width, bodyHeight, lipgloss.Height(help))
	updated, cmd := page.browser.Update(tea.WindowSizeMsg{Width: page.width, Height: layout.BodyHeight})
	page.browser = updated.(component.Browser)
	return cmd
}

func adminProfileAccessSummary(item application.TunnelAdminProfile) string {
	switch {
	case item.ManageAccess:
		return "manage"
	case item.ReadAccess:
		return "read"
	case item.KeyConfigured:
		return "unverified"
	default:
		return "missing key"
	}
}

func adminProfileScopeSummary(item application.TunnelAdminProfile) string {
	switch {
	case item.OrganizationID != "":
		return "organization:" + item.OrganizationID
	case item.WorkspaceID != "":
		return "workspace:" + item.WorkspaceID
	case item.TenantID != "":
		return "tenant:" + item.TenantID
	default:
		return "none"
	}
}
