package page

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
	"go.mewis.me/chatgpt-mcp/internal/tunnel"
)

type LocalTunnelCommand string

const (
	LocalTunnelEnable  LocalTunnelCommand = "tunnel.local.enable"
	LocalTunnelDisable LocalTunnelCommand = "tunnel.local.disable"
	LocalTunnelStart   LocalTunnelCommand = "tunnel.local.start"
	LocalTunnelStop    LocalTunnelCommand = "tunnel.local.stop"
	LocalTunnelDetach  LocalTunnelCommand = "tunnel.local.detach"
	LocalTunnelRefresh LocalTunnelCommand = "tunnel.local.refresh"
	LocalTunnelAdd     LocalTunnelCommand = "tunnel.local.add"
	LocalTunnelUpdate  LocalTunnelCommand = "tunnel.local.update"
)

type LocalTunnelCommandMsg struct {
	Command    LocalTunnelCommand
	ResourceID string
}

type localTunnelResultMsg struct {
	command LocalTunnelCommand
	id      string
	item    application.LocalTunnel
	items   []application.LocalTunnel
	admins  []application.TunnelAdminProfile
	err     error
}

type TunnelInstancesPage struct {
	ctx        context.Context
	resourceID string
	action     string
	items      []application.LocalTunnel
	admins     []application.TunnelAdminProfile
	browser    component.Browser
	detail     component.DetailPage
	editor     *component.Editor
	form       *tunnelRuntimeFormData
	confirm    component.ConfirmButtons
	confirmID  string
	external   *application.ExternalCommand
	notice     string
	err        error
	width      int
	height     int
}

func NewTunnelInstances(ctx context.Context, resourceID, action string) (*TunnelInstancesPage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	page := &TunnelInstancesPage{ctx: ctx, resourceID: strings.TrimSpace(resourceID), action: strings.TrimSpace(action)}
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

func (page *TunnelInstancesPage) Init() tea.Cmd {
	if page != nil && page.editor != nil {
		return page.editor.Init()
	}
	return nil
}
func (page *TunnelInstancesPage) OverlayActive() bool {
	return page != nil && (page.confirmID != "" || page.external != nil)
}
func (page *TunnelInstancesPage) InputActive() bool {
	return page != nil && (page.editor != nil || page.resourceID == "" && page.action == "" && page.browser.InputActive())
}
func (page *TunnelInstancesPage) Dirty() bool {
	return page != nil && page.editor != nil && page.editor.Dirty()
}
func (page *TunnelInstancesPage) Submitting() bool {
	return page != nil && page.editor != nil && page.editor.Submitting()
}
func (page *TunnelInstancesPage) Notice() string {
	if page == nil {
		return ""
	}
	return page.notice
}
func (page *TunnelInstancesPage) SetNotice(value string) {
	if page != nil {
		page.notice = strings.TrimSpace(value)
	}
}

func (page *TunnelInstancesPage) Update(message tea.Msg) (Model, tea.Cmd) {
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
			return page, func() tea.Msg { return NavigateMsg{Path: []string{"tunnel", id}} }
		}
		return page, nil
	case component.ConfirmChoiceMsg:
		if page.confirmID != "" {
			page.confirm.Select(msg.Affirmative)
			return page, page.updateConfirm(tea.KeyPressMsg{Code: tea.KeyEnter})
		}
		return page, nil
	case LocalTunnelCommandMsg:
		return page, page.runCommand(msg.Command, msg.ResourceID)
	case TunnelCommandMsg:
		if msg.Command == TunnelForeground {
			return page, page.showForeground(msg.ResourceID)
		}
		return page, nil
	case tunnelCopyMsg:
		if msg.err != nil {
			page.notice = "Clipboard unavailable: " + msg.err.Error()
		} else {
			page.notice = "Copied command to clipboard"
		}
		return page, func() tea.Msg { return OperationResult("tunnel.copy", "Tunnel", page.notice, nil) }
	case localTunnelResultMsg:
		return page, page.finishCommand(msg)
	case tea.KeyPressMsg:
		if page.external != nil {
			switch msg.String() {
			case "esc":
				page.external = nil
				return page, nil
			case "c":
				value := ""
				if page.external != nil {
					value = page.external.Command
				}
				return page, func() tea.Msg { return tunnelCopyMsg{err: component.CopyText(value)} }
			}
			return page, nil
		}
		if page.confirmID != "" {
			return page, page.updateConfirm(msg)
		}
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
		if page.resourceID == "" {
			switch msg.String() {
			case "r":
				return page, page.runCommand(LocalTunnelRefresh, "")
			case "n":
				return page, func() tea.Msg { return NavigateMsg{Path: []string{"tunnel", "create"}} }
			case "a":
				return page, func() tea.Msg { return NavigateMsg{Path: []string{"admins"}} }
			case "m":
				return page, func() tea.Msg { return NavigateMsg{Path: []string{"tunnels"}} }
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

func (page *TunnelInstancesPage) View(width, height int) string {
	if page == nil {
		return component.StateView(component.PageError, "Tunnel page unavailable", "")
	}
	page.width, page.height = width, height
	if page.editor != nil {
		page.editor.Resize(width, height)
		return page.editor.View()
	}
	content := page.listView(width, height)
	if page.resourceID != "" {
		page.detail.SetFeedback(page.notice, page.err)
		page.detail.Resize(width, height)
		content = page.detail.View()
	}
	if page.confirmID != "" {
		modalWidth := overlayWidth(width, 64)
		body := confirmOverlayBody(page.confirm, "Detach tunnel "+page.confirmID+"?", "The local tunnel instance and its runtime key will be removed. The remote OpenAI tunnel is unchanged.", modalWidth)
		content = component.CenterOverlay(content, component.Modal(body, modalWidth), width, height)
	}
	if page.external != nil {
		modalWidth := overlayWidth(width, 88)
		body := component.Title("Run outside the TUI") + "\n\n" + component.Muted(page.external.Reason) + "\n\n" + component.RenderCodeBlock(page.external.Command, "bash", component.ModalContentWidth(modalWidth)) + "\n\n" + component.Muted("c copy command · Esc close")
		content = component.CenterOverlay(content, component.Modal(component.WrapModalBody(body, modalWidth), modalWidth), width, height)
	}
	return content
}

func (page *TunnelInstancesPage) MouseTargets(originX, originY, z int) []component.MouseTarget {
	if page == nil {
		return nil
	}
	if page.editor != nil {
		return page.editor.MouseTargets(originX, originY, z)
	}
	if page.confirmID != "" {
		return confirmOverlayMouseTargets(page.confirm, "Detach tunnel "+page.confirmID+"?", "The local tunnel instance and its runtime key will be removed. The remote OpenAI tunnel is unchanged.", overlayWidth(page.width, 64), page.width, page.height, originX, originY, z+20)
	}
	if page.external != nil {
		body := component.Title("Run outside the TUI") + "\n\n" + component.Muted(page.external.Reason) + "\n\n" + component.RenderCodeBlock(page.external.Command, "bash", component.ModalContentWidth(overlayWidth(page.width, 88))) + "\n\n" + component.Muted("c copy command · Esc close")
		return dismissibleOverlayMouseTargets(body, overlayWidth(page.width, 88), page.width, page.height, originX, originY, z+20)
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

func (page *TunnelInstancesPage) reload() error {
	items, err := application.LocalTunnelsContext(page.ctx)
	if err != nil {
		return err
	}
	admins, err := application.TunnelAdminProfiles()
	if err != nil {
		return err
	}
	page.items, page.admins = items, admins
	if page.resourceID != "" {
		return page.syncDetail()
	}
	helpExpanded := page.browser.HelpExpanded()
	selected, _ := page.browser.Selected()
	page.browser = component.NewBrowser(page.ctx, "Tunnel instances", page.rows(), nil).WithTitleVisible(false).WithExternalHelp(true)
	page.browser.SetHelpBindings(
		component.Binding([]string{"n"}, "n", "attach"),
		component.Binding([]string{"r"}, "r", "refresh"),
		component.Binding([]string{"a"}, "a", "admins"),
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

func (page *TunnelInstancesPage) rows() []component.Row {
	ids := make([]string, len(page.items))
	names := make([]string, len(page.items))
	for i, item := range page.items {
		ids[i], names[i] = item.ID, localTunnelName(item)
	}
	labels := tunnel.UniqueLabels(ids, names)
	rows := make([]component.Row, 0, len(page.items))
	for i, item := range page.items {
		state := localTunnelState(item)
		description := ""
		if item.AdminProfileID != "" {
			description = "admin=" + item.AdminProfileID
		}
		rows = append(rows, component.Row{
			ID: item.ID, Title: labels[i], Description: description, Meta: state,
			Search: strings.Join([]string{item.ID, names[i], item.AdminProfileID, item.OrganizationID, item.ControlPlaneBaseURL, state}, " "),
		})
	}
	return rows
}

func localTunnelName(item application.LocalTunnel) string {
	if item.Status.Metadata == nil {
		return ""
	}
	return item.Status.Metadata.Name
}

func localTunnelLabel(item application.LocalTunnel) string {
	return tunnel.DisplayLabel(item.ID, localTunnelName(item))
}

func (page *TunnelInstancesPage) syncDetail() error {
	var item *application.LocalTunnel
	for i := range page.items {
		if page.items[i].ID == page.resourceID {
			item = &page.items[i]
			break
		}
	}
	if item == nil {
		return fmt.Errorf("tunnel %q is not attached", page.resourceID)
	}
	content := detailFields(
		[2]string{"ID", item.ID}, [2]string{"State", localTunnelState(*item)}, [2]string{"Enabled", tunnelYesNo(item.Enabled)},
		[2]string{"Runtime key", tunnelConfiguredIndicator(item.RuntimeKeyConfigured)}, [2]string{"Admin profile", valueOrNone(item.AdminProfileID)},
		[2]string{"Organization", valueOrNone(item.OrganizationID)}, [2]string{"Control plane", defaultLabel(item.ControlPlaneBaseURL)},
	)
	if item.Status.LastError != "" {
		content += "\n" + detailFields([2]string{"Error", item.Status.LastError})
	}
	page.detail = component.NewDetailPage(localTunnelLabel(*item), localTunnelState(*item), content).WithTitleVisible(false)
	bindings := []component.DetailPageBinding{{Key: "r", Desc: "refresh", Message: LocalTunnelCommandMsg{Command: LocalTunnelRefresh, ResourceID: item.ID}}}
	if item.Enabled {
		bindings = append(bindings, component.DetailPageBinding{Key: "space", Desc: "disable", Message: LocalTunnelCommandMsg{Command: LocalTunnelDisable, ResourceID: item.ID}})
	} else {
		bindings = append(bindings, component.DetailPageBinding{Key: "space", Desc: "enable", Message: LocalTunnelCommandMsg{Command: LocalTunnelEnable, ResourceID: item.ID}})
	}
	if item.Status.Running || item.Status.Restarting {
		bindings = append(bindings, component.DetailPageBinding{Key: "s", Desc: "stop", Message: LocalTunnelCommandMsg{Command: LocalTunnelStop, ResourceID: item.ID}})
	} else if item.Enabled && item.RuntimeKeyConfigured {
		bindings = append(bindings, component.DetailPageBinding{Key: "s", Desc: "start", Message: LocalTunnelCommandMsg{Command: LocalTunnelStart, ResourceID: item.ID}})
	}
	if item.Enabled && item.RuntimeKeyConfigured {
		bindings = append(bindings, component.DetailPageBinding{Key: "f", Desc: "run", Message: TunnelCommandMsg{Command: TunnelForeground, ResourceID: item.ID}})
	}
	bindings = append(bindings,
		component.DetailPageBinding{Key: "e", Desc: "edit", Message: NavigateMsg{Path: []string{"tunnel", item.ID, "edit"}}},
		component.DetailPageBinding{Key: "d", Desc: "detach", Message: LocalTunnelCommandMsg{Command: LocalTunnelDetach, ResourceID: item.ID}},
	)
	page.detail.SetBindings(bindings...)
	if page.width > 0 && page.height > 0 {
		page.detail.Resize(page.width, page.height)
	}
	return nil
}

func (page *TunnelInstancesPage) showForeground(id string) tea.Cmd {
	id = strings.TrimSpace(id)
	if id == "" {
		id = page.resourceID
	}
	if id == "" {
		page.err = fmt.Errorf("tunnel id is required")
		return nil
	}
	page.err = nil
	page.external = &application.ExternalCommand{Command: "cgm tunnel run " + id, Reason: "The foreground tunnel owns the terminal. Exit the TUI before starting it."}
	return nil
}

func (page *TunnelInstancesPage) runCommand(command LocalTunnelCommand, id string) tea.Cmd {
	id = strings.TrimSpace(id)
	if command == LocalTunnelDetach {
		page.confirmID = id
		page.confirm = component.NewConfirmButtons("Detach", "Cancel", false)
		return nil
	}
	ctx := page.ctx
	return func() tea.Msg {
		msg := localTunnelResultMsg{command: command, id: id}
		switch command {
		case LocalTunnelEnable:
			msg.item, msg.err = application.SetLocalTunnelEnabled(ctx, id, true)
		case LocalTunnelDisable:
			msg.item, msg.err = application.SetLocalTunnelEnabled(ctx, id, false)
		case LocalTunnelStart:
			msg.item, msg.err = application.StartLocalTunnel(ctx, id)
		case LocalTunnelStop:
			msg.item, msg.err = application.StopLocalTunnel(ctx, id)
		case LocalTunnelRefresh:
			msg.items, msg.err = application.LocalTunnelsContext(ctx)
			if msg.err == nil {
				msg.admins, msg.err = application.TunnelAdminProfiles()
			}
		default:
			msg.err = fmt.Errorf("unsupported local tunnel action: %s", command)
		}
		return msg
	}
}

func (page *TunnelInstancesPage) finishCommand(msg localTunnelResultMsg) tea.Cmd {
	if page.editor != nil {
		page.editor.SetSubmitting(false)
	}
	if msg.err != nil {
		page.err = nil
		if page.editor != nil {
			page.editor.SetSubmitting(false)
		} else {
			page.err = msg.err
		}
		return func() tea.Msg { return OperationResult("tunnel.local.save", "Tunnel", "", msg.err) }
	}
	page.err = nil
	if page.editor != nil && (page.action == "create" || page.action == "edit") {
		page.notice = localTunnelSuccess(msg.command)
		page.acceptLocalEditorSuccess()
		id := msg.item.ID
		if id == "" {
			id = msg.id
		}
		return tea.Batch(
			func() tea.Msg { return NavigateMsg{Path: []string{"tunnel", id}, Replace: true} },
			func() tea.Msg { return OperationResult("tunnel.local.save", "Tunnel", page.notice, nil) },
		)
	}
	if msg.command == LocalTunnelDetach {
		items := page.items[:0]
		for _, item := range page.items {
			if item.ID != msg.id {
				items = append(items, item)
			}
		}
		page.items = items
		page.notice = localTunnelSuccess(msg.command)
		if page.resourceID == msg.id {
			return withOperation("tunnel.local.action", "Tunnel", page.notice, func() tea.Msg { return NavigateMsg{Path: []string{"tunnel"}, Replace: true} })
		}
	} else if msg.command == LocalTunnelRefresh {
		page.items, page.admins = msg.items, msg.admins
		page.notice = fmt.Sprintf("Refreshed %d tunnel instance(s)", len(page.items))
	} else {
		for i := range page.items {
			if page.items[i].ID == msg.item.ID {
				page.items[i] = msg.item
			}
		}
		page.notice = localTunnelSuccess(msg.command)
	}
	if page.resourceID != "" {
		page.err = page.syncDetail()
		return withOperation("tunnel.local.action", "Tunnel", page.notice, nil)
	}
	selected, _ := page.browser.Selected()
	return withOperation("tunnel.local.action", "Tunnel", page.notice, page.browser.ReplaceRows(page.rows(), selected.ID))
}

func (page *TunnelInstancesPage) updateConfirm(msg tea.KeyPressMsg) tea.Cmd {
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
		err := application.DetachLocalTunnel(ctx, id)
		return localTunnelResultMsg{command: LocalTunnelDetach, id: id, err: err}
	}
}

func (page *TunnelInstancesPage) listView(width, height int) string {
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

func (page *TunnelInstancesPage) listHeader(width int) string {
	ready, running, degraded := 0, 0, 0
	for _, item := range page.items {
		if item.Status.Ready {
			ready++
		}
		if item.Status.Running || item.Status.Restarting {
			running++
		}
		if item.Status.LastError != "" {
			degraded++
		}
	}
	summary := fmt.Sprintf("%d attached · %d running · %d ready · %d degraded · %d admin profiles", len(page.items), running, ready, degraded, len(page.admins))
	if len(page.items) == 0 {
		summary = "No tunnels attached"
	}
	return component.PageTitleNotice("OpenAI Secure MCP Tunnels", page.notice, width) + "\n" + component.Muted(summary)
}

func (page *TunnelInstancesPage) listFeedback(width int) string {
	if page.err == nil {
		return ""
	}
	return component.BannerWidth(page.err.Error(), component.ToneDanger, width)
}

func (page *TunnelInstancesPage) resizeBrowser() tea.Cmd {
	if page.resourceID != "" || page.width <= 0 || page.height <= 0 {
		return nil
	}
	headerHeight := lipgloss.Height(page.listHeader(page.width))
	bodyHeight := max(1, page.height-headerHeight)
	updated, _ := page.browser.Update(tea.WindowSizeMsg{Width: page.width, Height: bodyHeight})
	page.browser = updated.(component.Browser)
	help := page.browser.HelpView()
	layout := component.NewSectionLayout("", "", page.listFeedback(page.width), page.width, bodyHeight, lipgloss.Height(help))
	updated, cmd := page.browser.Update(tea.WindowSizeMsg{Width: page.width, Height: layout.BodyHeight})
	page.browser = updated.(component.Browser)
	return cmd
}

func (page *TunnelInstancesPage) initEditor() error {
	create := page.action == "create"
	if create && page.resourceID != "" {
		return fmt.Errorf("local tunnel attach editor does not accept a resource")
	}
	if !create && (page.action != "edit" || page.resourceID == "") {
		return fmt.Errorf("unsupported local tunnel editor action %q", page.action)
	}
	items, err := application.LocalTunnelsContext(page.ctx)
	if err != nil {
		return err
	}
	admins, err := application.TunnelAdminProfiles()
	if err != nil {
		return err
	}
	page.items, page.admins = items, admins
	var item application.LocalTunnel
	if !create {
		found := false
		for _, candidate := range page.items {
			if candidate.ID == page.resourceID {
				item, found = candidate, true
				break
			}
		}
		if !found {
			return fmt.Errorf("tunnel %q is not attached", page.resourceID)
		}
	}
	editor, data := newLocalTunnelEditor(item, create, page.admins)
	page.editor, page.form = &editor, data
	if page.width > 0 && page.height > 0 {
		page.editor.Resize(page.width, page.height)
	}
	return nil
}

func (page *TunnelInstancesPage) submitEditor() tea.Cmd {
	if page == nil || page.editor == nil {
		return nil
	}
	if err := page.editor.Validate(); err != nil {
		page.editor.SetFeedback("", err)
		return nil
	}
	page.editor.SetFeedback("", nil)
	create := page.action == "create"
	instance, err := localInstanceFromForm(page.form, page.resourceID)
	if err != nil {
		page.editor.SetFeedback("", err)
		return nil
	}
	if create && instance.APIKey == "" {
		page.editor.SetFeedback("", fmt.Errorf("runtime API key is required"))
		return nil
	}
	page.editor.SetSubmitting(true)
	ctx := page.ctx
	pending := "Attaching tunnel..."
	if !create {
		pending = "Saving tunnel..."
	}
	return beginOperation("tunnel.local.save", "Tunnel", pending, func() tea.Msg {
		msg := localTunnelResultMsg{id: instance.ID}
		if create {
			msg.command = LocalTunnelAdd
			msg.item, msg.err = application.AttachLocalTunnel(ctx, instance)
		} else {
			msg.command = LocalTunnelUpdate
			msg.item, msg.err = application.UpdateLocalTunnel(ctx, instance)
		}
		return msg
	})
}

func (page *TunnelInstancesPage) editorParentNavigation() tea.Cmd {
	if page != nil && page.resourceID != "" {
		id := page.resourceID
		return func() tea.Msg { return NavigateMsg{Path: []string{"tunnel", id}} }
	}
	return func() tea.Msg { return NavigateMsg{Path: []string{"tunnel"}} }
}

func (page *TunnelInstancesPage) acceptLocalEditorSuccess() {
	if page == nil || page.editor == nil {
		return
	}
	page.editor.Accept()
	page.editor.SetSubmitting(false)
}

func localTunnelState(item application.LocalTunnel) string {
	switch {
	case !item.Enabled:
		return "disabled"
	case !item.RuntimeKeyConfigured:
		return "not configured"
	case item.Status.Ready:
		return "connected"
	case item.Status.Restarting:
		return "reconnecting"
	case item.Status.Running:
		return "connecting"
	case item.Status.LastError != "":
		return "degraded"
	default:
		return "offline"
	}
}

func localTunnelSuccess(command LocalTunnelCommand) string {
	switch command {
	case LocalTunnelEnable:
		return "Tunnel enabled"
	case LocalTunnelDisable:
		return "Tunnel disabled"
	case LocalTunnelStart:
		return "Tunnel started"
	case LocalTunnelStop:
		return "Tunnel stopped"
	case LocalTunnelDetach:
		return "Tunnel detached"
	case LocalTunnelAdd:
		return "Tunnel attached"
	case LocalTunnelUpdate:
		return "Tunnel updated"
	default:
		return "Tunnel updated"
	}
}
