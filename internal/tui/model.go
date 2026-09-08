package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/approval"
	"go.mewis.me/chatgpt-mcp/internal/tui/action"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
	tuipage "go.mewis.me/chatgpt-mcp/internal/tui/page"
	"go.mewis.me/chatgpt-mcp/internal/tui/palette"
	"go.mewis.me/chatgpt-mcp/internal/tui/quickopen"
	tuistate "go.mewis.me/chatgpt-mcp/internal/tui/state"
)

type overlayKind uint8

const (
	overlayNone overlayKind = iota
	overlayCommands
)

const navbarMinHeight = 9
const approvalPollInterval = time.Second
const toastDuration = 3 * time.Second

type approvalStage uint8

const (
	approvalStageNone approvalStage = iota
	approvalStageChoice
	approvalStageResolving
)

type approvalPollMsg struct {
	requests []approval.Request
	err      error
}

type approvalPollTickMsg struct{}

type approvalResolvedMsg struct {
	id      string
	approve bool
	err     error
}

type toastDismissMsg struct {
	id    uint64
	timer uint64
}

type toastCloseMsg struct{}

type toastHoverMsg struct {
	id      uint64
	hovered bool
}

type toastState struct {
	id      uint64
	timer   uint64
	title   string
	message string
	tone    component.Tone
	hovered bool
}

type Model struct {
	ctx              context.Context
	router           Router
	actions          *action.Registry
	palette          *palette.Model
	homeCommands     *palette.Model
	overlay          overlayKind
	commandResources map[string]quickopen.Resource
	stateRoot        string
	state            tuistate.State
	notice           string
	currentPage      tuipage.Model
	theme            theme
	width            int
	height           int
	approvals        []approval.Request
	approvalStage    approvalStage
	approvalChoice   component.ConfirmButtons
	approvalApprove  bool
	approvalErr      error
	approvalList     func(context.Context) ([]approval.Request, error)
	approvalResolve  func(context.Context, string, bool, string) (approval.Request, error)
	approvalNow      func() time.Time
	toast            toastState
	toastSeq         uint64
}

func NewModel(initial Route) Model {
	return NewModelWithContext(context.Background(), initial)
}

func NewModelWithContext(ctx context.Context, initial Route) Model {
	return NewModelWithState(ctx, initial, "")
}

func NewModelWithState(ctx context.Context, initial Route, root string) Model {
	if ctx == nil {
		ctx = context.Background()
	}
	state := tuistate.Default()
	if root != "" {
		if loaded, err := tuistate.Load(root); err == nil {
			state = loaded
		}
	}
	model := Model{ctx: ctx, router: NewRouter(initial), actions: defaultActionRegistry(), stateRoot: root, state: state, theme: newTheme(true), approvalList: application.ListApprovalRequests, approvalResolve: application.ResolveApprovalRequest, approvalNow: time.Now}
	model.loadPage(initial)
	return model
}

func (model Model) Init() tea.Cmd {
	commands := []tea.Cmd{model.pollApprovalsCmd()}
	if model.currentPage != nil {
		commands = append(commands, model.currentPage.Init())
	}
	return tea.Batch(commands...)
}

func (model Model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tuipage.ToastMsg:
		return model, model.showToast(msg.Title, msg.Message, msg.Tone)
	case toastCloseMsg:
		model.dismissToast()
		return model, nil
	case toastHoverMsg:
		if msg.id != model.toast.id || model.toast.id == 0 || msg.hovered == model.toast.hovered {
			return model, nil
		}
		model.toast.hovered = msg.hovered
		model.toast.timer++
		if msg.hovered {
			return model, nil
		}
		return model, model.toastTimerCmd()
	case toastDismissMsg:
		if msg.id == model.toast.id && msg.timer == model.toast.timer && !model.toast.hovered {
			model.dismissToast()
		}
		return model, nil
	case approvalPollMsg:
		model.applyApprovalPoll(msg)
		return model, model.approvalTickCmd()
	case approvalPollTickMsg:
		model.expireElapsedApprovals(model.approvalTime())
		return model, model.pollApprovalsCmd()
	case approvalResolvedMsg:
		return model.finishApprovalResolution(msg)
	case tea.BackgroundColorMsg:
		model.theme = newTheme(msg.IsDark())
		component.SetDarkBackground(msg.IsDark())
		if model.homeCommands != nil {
			updated, _ := model.homeCommands.Update(msg)
			model.homeCommands = &updated
		}
		if model.currentPage != nil {
			updated, cmd := model.currentPage.Update(msg)
			model.currentPage = updated
			if model.palette == nil && cmd != nil {
				return model, cmd
			}
		}
		if model.palette != nil {
			updated, cmd := model.palette.Update(msg)
			model.palette = &updated
			return model, cmd
		}
	case tea.WindowSizeMsg:
		model.width, model.height = msg.Width, msg.Height
		if model.currentPage != nil {
			metrics := model.frameMetrics(msg.Width, msg.Height)
			updated, cmd := model.currentPage.Update(tea.WindowSizeMsg{Width: metrics.contentWidth, Height: metrics.bodyHeight})
			model.currentPage = updated
			if cmd != nil {
				return model, cmd
			}
		}
	case palette.ClosedMsg:
		model.closeOverlay()
		return model, nil
	case component.ConfirmChoiceMsg:
		if model.approvalActive() {
			return model.updateApprovalChoice(msg)
		}
		return model, nil
	case palette.SelectedMsg:
		if resource, ok := model.commandResources[msg.ID]; ok {
			model.closeOverlay()
			route, err := ParseRoute(resource.Path)
			if err != nil {
				return model, model.showToast("Commands", err.Error(), component.ToneDanger)
			}
			model.router.Navigate(route)
			model.loadPage(route)
			return model, model.initCurrentPage()
		}
		homeSelection := model.router.Current().Kind == RouteHome && model.overlay == overlayNone && model.homeCommands != nil
		if !homeSelection {
			model.closeOverlay()
		}
		recentCmd := model.recordRecent(msg.ID)
		if homeSelection {
			model.resetHomeCommands()
		}
		cmd, err := model.actions.Execute(model.ctx, msg.ID, actionContext(model.router.Current()))
		if err != nil {
			return model, tea.Batch(recentCmd, model.showToast("Command", err.Error(), component.ToneDanger))
		}
		return model, tea.Batch(recentCmd, cmd)
	case palette.MouseScrollMsg:
		if model.palette != nil {
			updated, cmd := model.palette.Update(msg)
			model.palette = &updated
			return model, cmd
		}
		if model.router.Current().Kind == RouteHome && model.homeCommands != nil {
			updated, cmd := model.homeCommands.Update(msg)
			model.homeCommands = &updated
			return model, cmd
		}
		return model, nil
	case navigateMsg:
		if msg.sibling {
			model.switchPage(msg.route)
		} else {
			model.navigate(msg.route)
		}
		return model, model.initCurrentPage()
	case tuipage.NavigateMsg:
		route, err := ParseRoute(msg.Path)
		if err != nil {
			return model, model.showToast("Navigation", err.Error(), component.ToneDanger)
		}
		if msg.Replace {
			model.switchPage(route)
		} else {
			model.navigate(route)
		}
		return model, model.initCurrentPage()
	case tuipage.WorkspaceCommandMsg:
		if err := model.ensureWorkspacePage(msg.Command, msg.ResourceID); err != nil {
			return model, model.showToast("Workspaces", err.Error(), component.ToneDanger)
		}
		return model.updatePage(msg)
	case tuipage.MCPCommandMsg:
		if err := model.ensureMCPPage(msg.ResourceID); err != nil {
			return model, model.showToast("MCP", err.Error(), component.ToneDanger)
		}
		return model.updatePage(msg)
	case tuipage.TunnelCommandMsg:
		if err := model.ensureTunnelPage(msg.Command, msg.ResourceID); err != nil {
			return model, model.showToast("Tunnel", err.Error(), component.ToneDanger)
		}
		return model.updatePage(msg)
	case tuipage.RequestCommandMsg:
		if err := model.ensureRequestPage(msg.ResourceID); err != nil {
			return model, model.showToast("Requests", err.Error(), component.ToneDanger)
		}
		return model.updatePage(msg)
	case tuipage.LogsCommandMsg:
		if err := model.ensureLogsPage(); err != nil {
			return model, model.showToast("Logs", err.Error(), component.ToneDanger)
		}
		return model.updatePage(msg)
	case tuipage.SystemCommandMsg:
		if err := model.ensureRuntimePage(); err != nil {
			return model, model.showToast("Runtime", err.Error(), component.ToneDanger)
		}
		return model.updatePage(msg)
	case tuipage.ConfigCommandMsg:
		if err := model.ensureConfigPage(); err != nil {
			return model, model.showToast("Config", err.Error(), component.ToneDanger)
		}
		return model.updatePage(msg)
	case component.FormSubmittedMsg, component.FormCancelledMsg:
		return model.updatePage(msg)
	case tea.MouseClickMsg, tea.MouseReleaseMsg, tea.MouseWheelMsg, tea.MouseMotionMsg:
		return model, nil
	case tea.KeyPressMsg:
		if model.toast.id != 0 {
			switch msg.String() {
			case "enter", "esc":
				model.dismissToast()
			}
			return model, nil
		}
		if model.approvalActive() {
			return model.updateApprovalKey(msg)
		}
		if model.palette != nil {
			updated, cmd := model.palette.Update(msg)
			model.palette = &updated
			return model, cmd
		}
		if model.currentPage != nil && (model.currentPage.OverlayActive() || model.currentPage.InputActive()) {
			return model.updatePage(msg)
		}
		if model.router.Current().Kind == RouteHome && model.homeCommands != nil {
			switch msg.String() {
			case "alt+left":
				model.switchPage(cycleHeaderRoute(model.router.Current(), -1))
				return model, model.initCurrentPage()
			case "alt+right":
				model.switchPage(cycleHeaderRoute(model.router.Current(), 1))
				return model, model.initCurrentPage()
			case "esc":
				return model, tea.Quit
			case "ctrl+k":
				return model, nil
			default:
				updated, cmd := model.homeCommands.Update(msg)
				model.homeCommands = &updated
				return model, cmd
			}
		}
		if isCommandsKey(msg) {
			return model, model.openCommands()
		}
		switch msg.String() {
		case "alt+left":
			model.switchPage(cycleHeaderRoute(model.router.Current(), -1))
			return model, model.initCurrentPage()
		case "alt+right":
			model.switchPage(cycleHeaderRoute(model.router.Current(), 1))
			return model, model.initCurrentPage()
		case "esc":
			if model.router.Back() {
				model.loadPage(model.router.Current())
				return model, model.initCurrentPage()
			}
			model.switchPage(Route{Kind: RouteHome})
			return model, nil
		case "backspace":
			if model.router.Back() {
				model.loadPage(model.router.Current())
				return model, model.initCurrentPage()
			}
		default:
			if selected, ok := model.actions.MatchShortcut(msg, actionContext(model.router.Current())); ok {
				cmd, err := model.actions.Execute(model.ctx, selected.ID, actionContext(model.router.Current()))
				if err == nil {
					return model, cmd
				}
			}
		}
	}
	if model.currentPage != nil {
		return model.updatePage(message)
	}
	return model, nil
}

func (model Model) View() tea.View {
	content, targets := model.render()
	if model.palette != nil {
		width, height := model.layoutSize()
		paletteWidth := max(1, min(78, width-4))
		foreground := model.palette.View(paletteWidth)
		x, y := max(0, (width-lipgloss.Width(foreground))/2), max(0, (height-lipgloss.Height(foreground))/2)
		content = centerOverlay(content, foreground, width, height)
		targets = append(targets, model.palette.MouseTargets(x, y, 100, paletteWidth)...)
	}
	if model.approvalActive() {
		width, height := model.layoutSize()
		modalWidth := max(1, min(88, width-4))
		body := model.approvalDialogView(component.ModalContentWidth(modalWidth))
		foreground := component.Modal(body, modalWidth)
		overlayTargets, x, y := component.CenteredOverlayTargets(foreground, width, height, 0, 0, 199, tea.KeyPressMsg{Code: tea.KeyEscape})
		content = centerOverlay(content, foreground, width, height)
		targets = append(targets, overlayTargets...)
		buttons := model.approvalButtonsView()
		if buttons != "" {
			if rect, ok := component.FindRenderedRect(foreground, buttons); ok {
				var buttonTargets []component.MouseTarget
				if model.approvalStage == approvalStageChoice {
					buttonTargets = model.approvalChoice.MouseTargets(x+rect.X, y+rect.Y, 201)
				}
				targets = append(targets, buttonTargets...)
			}
		}
	}
	if model.toast.id != 0 {
		width, height := model.layoutSize()
		dialog := component.NewToastDialog(model.toast.title, model.toast.message, model.toast.tone)
		modalWidth := max(1, min(72, width-4))
		foreground := component.Modal(dialog.ViewWidth(component.ModalContentWidth(modalWidth)), modalWidth)
		overlayTargets, x, y := component.CenteredOverlayTargets(foreground, width, height, 0, 0, 299, toastCloseMsg{})
		id := model.toast.id
		overlayTargets[0].Handle = func(event component.MouseEvent) tea.Msg {
			if event.Motion {
				return toastHoverMsg{id: id, hovered: false}
			}
			if event.Button == tea.MouseLeft {
				return toastCloseMsg{}
			}
			return nil
		}
		overlayTargets[1].Handle = func(event component.MouseEvent) tea.Msg {
			if event.Motion {
				return toastHoverMsg{id: id, hovered: true}
			}
			return nil
		}
		content = centerOverlay(content, foreground, width, height)
		targets = append(targets, overlayTargets...)
		if rect, ok := component.FindRenderedRect(foreground, dialog.CloseButtonView()); ok {
			targets = append(targets, component.MouseTarget{
				ID: "toast.close", Rect: component.Rect{X: x + rect.X, Y: y + rect.Y, Width: rect.Width, Height: rect.Height}, Z: 301,
				Handle: func(event component.MouseEvent) tea.Msg {
					if event.Motion {
						return toastHoverMsg{id: id, hovered: true}
					}
					if event.Button == tea.MouseLeft {
						return toastCloseMsg{}
					}
					return nil
				},
			})
		}
	}
	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = "ChatGPT MCP · " + model.router.Current().Title()
	view.MouseMode = tea.MouseModeCellMotion
	view.OnMouse = func(message tea.MouseMsg) tea.Cmd { return component.DispatchMouse(targets, message) }
	return view
}

func (model Model) pollApprovalsCmd() tea.Cmd {
	list := model.approvalList
	ctx := model.ctx
	if list == nil {
		return nil
	}
	return func() tea.Msg {
		requests, err := list(ctx)
		return approvalPollMsg{requests: requests, err: err}
	}
}

func (model Model) approvalTickCmd() tea.Cmd {
	return tea.Tick(approvalPollInterval, func(time.Time) tea.Msg { return approvalPollTickMsg{} })
}

func (model *Model) applyApprovalPoll(msg approvalPollMsg) {
	if model == nil || msg.err != nil {
		return
	}
	now := model.approvalTime()
	pending := make([]approval.Request, 0, len(msg.requests))
	for _, request := range msg.requests {
		if request.Status == approval.StatusPending && !approvalRequestExpired(request, now) {
			pending = append(pending, request)
		}
	}
	activeID := model.activeApprovalID()
	if activeID != "" {
		pending = moveApprovalFirst(pending, activeID)
	}
	model.approvals = pending
	if len(pending) == 0 {
		model.resetApprovalDialog()
		return
	}
	if activeID == "" || pending[0].ID != activeID {
		model.openApprovalChoice()
	}
}

func (model Model) approvalActive() bool {
	return len(model.approvals) > 0 && model.approvalStage != approvalStageNone
}

func (model Model) activeApproval() (approval.Request, bool) {
	if len(model.approvals) == 0 {
		return approval.Request{}, false
	}
	return model.approvals[0], true
}

func (model Model) activeApprovalID() string {
	request, ok := model.activeApproval()
	if !ok {
		return ""
	}
	return request.ID
}

func (model *Model) openApprovalChoice() {
	if model == nil || len(model.approvals) == 0 {
		return
	}
	model.approvalStage = approvalStageChoice
	model.approvalChoice = component.NewConfirmButtons("Approve", "Deny", false)
	model.approvalApprove = false
	model.approvalErr = nil
}

func (model *Model) resetApprovalDialog() {
	if model == nil {
		return
	}
	model.approvalStage = approvalStageNone
	model.approvalChoice = component.ConfirmButtons{}
	model.approvalApprove = false
	model.approvalErr = nil
}

func (model Model) updateApprovalChoice(msg component.ConfirmChoiceMsg) (tea.Model, tea.Cmd) {
	if model.approvalStage == approvalStageChoice {
		model.approvalChoice.Select(msg.Affirmative)
		return model.resolveApprovalSelection(model.approvalChoice.AffirmativeSelected())
	}
	return model, nil
}

func (model Model) updateApprovalKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch model.approvalStage {
	case approvalStageChoice:
		switch msg.String() {
		case "a":
			return model.resolveApprovalSelection(true)
		case "d":
			return model.resolveApprovalSelection(false)
		case "enter":
			return model.resolveApprovalSelection(model.approvalChoice.AffirmativeSelected())
		case "esc":
			return model, nil
		default:
			return model, model.approvalChoice.Update(msg)
		}
	case approvalStageResolving:
		return model, nil
	default:
		return model, nil
	}
}

func (model Model) resolveApprovalSelection(approve bool) (tea.Model, tea.Cmd) {
	request, ok := model.activeApproval()
	if !ok || model.approvalResolve == nil {
		model.resetApprovalDialog()
		return model, nil
	}
	if approvalRequestExpired(request, model.approvalTime()) {
		model.expireApproval(request.ID)
		return model, model.pollApprovalsCmd()
	}
	id, resolve, ctx := request.ID, model.approvalResolve, model.ctx
	model.approvalApprove = approve
	model.approvalStage = approvalStageResolving
	model.approvalErr = nil
	return model, func() tea.Msg {
		_, err := resolve(ctx, id, approve, "")
		return approvalResolvedMsg{id: id, approve: approve, err: err}
	}
}

func (model Model) finishApprovalResolution(msg approvalResolvedMsg) (tea.Model, tea.Cmd) {
	if msg.id != model.activeApprovalID() {
		return model, nil
	}
	if msg.err != nil {
		model.openApprovalChoice()
		model.approvalErr = msg.err
		return model, nil
	}
	model.approvals = removeApprovalRequest(model.approvals, msg.id)
	action := "Denied"
	if msg.approve {
		action = "Approved"
	}
	if len(model.approvals) == 0 {
		model.resetApprovalDialog()
	} else {
		model.openApprovalChoice()
	}
	return model, model.showToast("Approval request", action+" "+msg.id, component.ToneSuccess)
}

func (model Model) approvalButtonsView() string {
	if model.approvalStage == approvalStageChoice {
		return model.approvalChoice.View()
	}
	return ""
}

func (model Model) approvalDialogView(width int) string {
	request, ok := model.activeApproval()
	if !ok {
		return ""
	}
	title := strings.TrimSpace(request.Title)
	if title == "" {
		title = request.ID
	}
	lines := []string{
		component.WrapContent(component.Title("Approval request"), width), "",
		component.WrapKeyValue("Title", title, width), component.WrapKeyValue("Request", request.ID, width), component.WrapKeyValue("Workspace", request.WorkspaceID, width), component.WrapKeyValue("Tool", request.TargetTool, width),
	}
	if request.Source != "" {
		lines = append(lines, component.WrapKeyValue("Source", request.Source, width))
	}
	if request.GuardCode != "" {
		lines = append(lines, component.WrapKeyValue("Guard", string(request.GuardCode), width))
	}
	if !request.ExpiresAt.IsZero() {
		expires := request.ExpiresAt.Local().Format("15:04:05")
		countdown := approvalCountdown(request.ExpiresAt, model.approvalTime())
		lines = append(lines, component.WrapKeyValue("Expires in", countdown+" · "+expires, width))
	}
	lines = append(lines, "", component.Label("Arguments"), component.WrapContent(approvalArguments(request.Arguments), width), "")
	if model.approvalErr != nil {
		lines = append(lines, component.BannerWidth(model.approvalErr.Error(), component.ToneDanger, width), "")
	}
	switch model.approvalStage {
	case approvalStageChoice:
		lines = append(lines, model.approvalChoice.View(), component.WrapContent(component.Muted("a approve · d deny · ←/→ choose · Enter submit"), width))
	case approvalStageResolving:
		lines = append(lines, component.WrapContent(component.Muted("Resolving request..."), width))
	}
	if len(model.approvals) > 1 {
		lines = append(lines, "", component.WrapContent(component.Muted(fmt.Sprintf("%d more pending request(s)", len(model.approvals)-1)), width))
	}
	return strings.Join(lines, "\n")
}

func approvalArguments(raw json.RawMessage) string {
	if len(raw) == 0 {
		return component.Muted("None")
	}
	var value any
	if json.Unmarshal(raw, &value) == nil {
		if data, err := json.MarshalIndent(value, "", "  "); err == nil {
			return string(data)
		}
	}
	return string(raw)
}

func moveApprovalFirst(requests []approval.Request, id string) []approval.Request {
	for index, request := range requests {
		if request.ID != id || index == 0 {
			continue
		}
		active := requests[index]
		copy(requests[1:index+1], requests[:index])
		requests[0] = active
		break
	}
	return requests
}

func removeApprovalRequest(requests []approval.Request, id string) []approval.Request {
	result := requests[:0]
	for _, request := range requests {
		if request.ID != id {
			result = append(result, request)
		}
	}
	return result
}

func (model Model) approvalTime() time.Time {
	if model.approvalNow != nil {
		return model.approvalNow()
	}
	return time.Now()
}

func (model *Model) expireElapsedApprovals(now time.Time) {
	if model == nil || len(model.approvals) == 0 {
		return
	}
	remaining := model.approvals[:0]
	activeID := model.activeApprovalID()
	activeExpired := false
	for _, request := range model.approvals {
		if approvalRequestExpired(request, now) {
			activeExpired = activeExpired || request.ID == activeID
			continue
		}
		remaining = append(remaining, request)
	}
	model.approvals = remaining
	if len(remaining) == 0 {
		model.resetApprovalDialog()
		return
	}
	if activeExpired {
		model.openApprovalChoice()
	}
}

func (model *Model) expireApproval(id string) {
	if model == nil || id == "" {
		return
	}
	wasActive := model.activeApprovalID() == id
	model.approvals = removeApprovalRequest(model.approvals, id)
	if len(model.approvals) == 0 {
		model.resetApprovalDialog()
		return
	}
	if wasActive {
		model.openApprovalChoice()
	}
}

func approvalRequestExpired(request approval.Request, now time.Time) bool {
	return !request.ExpiresAt.IsZero() && !now.Before(request.ExpiresAt)
}

func approvalCountdown(expiresAt, now time.Time) string {
	if expiresAt.IsZero() || !now.Before(expiresAt) {
		return "00:00:00"
	}
	remaining := expiresAt.Sub(now)
	seconds := int64((remaining + time.Second - 1) / time.Second)
	hours := seconds / 3600
	minutes := seconds % 3600 / 60
	seconds %= 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

func (model *Model) openCommands() tea.Cmd {
	if model == nil {
		return nil
	}
	context := actionContext(model.router.Current())
	actions, resources, err := model.commandActions(context)
	if err != nil {
		return model.showToast("Commands", err.Error(), component.ToneDanger)
	}
	value := palette.NewWithOptions(actions, context, palette.Options{Title: "Commands", Hint: "Ctrl+K", Placeholder: "Type a command or resource", Recent: model.state.RecentActions})
	model.palette = &value
	model.overlay = overlayCommands
	model.commandResources = resources
	return nil
}

func isCommandsKey(message tea.KeyPressMsg) bool {
	return message.String() == "ctrl+k"
}

func (model Model) commandActions(context action.Context) ([]action.Action, map[string]quickopen.Resource, error) {
	actions := model.actions.Actions(context)
	resources, err := loadQuickOpenResources()
	if err != nil {
		return actions, nil, err
	}
	resourceActions, index := quickopen.Actions(resources)
	return append(actions, resourceActions...), index, nil
}

func (model *Model) closeOverlay() {
	if model == nil {
		return
	}
	model.palette = nil
	model.overlay = overlayNone
	model.commandResources = nil
}

func (model *Model) recordRecent(id string) tea.Cmd {
	if model == nil {
		return nil
	}
	tuistate.RecordRecent(&model.state, id)
	if model.stateRoot != "" {
		if err := tuistate.Save(model.stateRoot, model.state); err != nil {
			return model.showToast("TUI state", err.Error(), component.ToneDanger)
		}
	}
	return nil
}

func (model *Model) navigate(route Route) {
	if model == nil {
		return
	}
	model.router.Navigate(route)
	model.loadPage(route)
}

func (model *Model) switchPage(route Route) {
	if model == nil {
		return
	}
	model.router.Switch(route)
	model.loadPage(route)
}

func (model *Model) loadPage(route Route) {
	if model == nil {
		return
	}
	if page, ok := model.currentPage.(interface{ Close() }); ok {
		page.Close()
	}
	model.currentPage = nil
	model.homeCommands = nil
	if route.Kind == RouteHome {
		model.resetHomeCommands()
		return
	}
	var value tuipage.Model
	var err error
	switch route.Kind {
	case RouteWorkspaces:
		value, err = tuipage.NewWorkspacesRoute(model.ctx, route.ResourceID, route.Section)
	case RouteContainers:
		value, err = tuipage.NewContainersRoute(model.ctx, route.ResourceID, route.Section)
	case RouteMCP:
		value, err = tuipage.NewMCPRoute(model.ctx, route.ResourceID, route.Section)
	case RouteTunnel:
		value, err = tuipage.NewTunnelDashboard(model.ctx)
	case RouteTunnels:
		value, err = tuipage.NewManagedTunnelsRoute(model.ctx, route.ResourceID, route.Section)
	case RouteRequests:
		value, err = tuipage.NewRequestsRouteMode(model.ctx, route.Mode, route.ResourceID, route.Section)
	case RouteLogs:
		value, err = tuipage.NewLogsRoute(model.ctx, route.ResourceID, route.Section)
	case RouteLogsExec:
		value, err = tuipage.NewCommandExecutionLogs(model.ctx)
	case RouteRuntime:
		value, err = tuipage.NewRuntimeRoute(model.ctx, route.ResourceID)
	case RouteAbout:
		value, err = tuipage.NewAbout(model.ctx)
	case RouteConfig:
		value, err = tuipage.NewConfigRoute(model.ctx, route.ResourceID)
	}
	if err != nil {
		model.notice = err.Error()
		return
	}
	model.currentPage = value
	if model.currentPage != nil && model.width > 0 && model.height > 0 {
		metrics := model.frameMetrics(model.width, model.height)
		updated, _ := model.currentPage.Update(tea.WindowSizeMsg{Width: metrics.contentWidth, Height: metrics.bodyHeight})
		model.currentPage = updated
	}
}

func (model *Model) ensureLogsPage() error {
	if model.router.Current().Kind != RouteLogs {
		model.navigate(Route{Kind: RouteLogs})
	}
	if model.currentPage == nil {
		return fmt.Errorf("logs viewer is unavailable")
	}
	return nil
}

func (model *Model) ensureRuntimePage() error {
	if model.router.Current().Kind != RouteRuntime {
		model.navigate(Route{Kind: RouteRuntime})
	}
	if model.currentPage == nil {
		return fmt.Errorf("runtime/system page is unavailable")
	}
	return nil
}

func (model Model) initCurrentPage() tea.Cmd {
	if model.currentPage == nil {
		return nil
	}
	return model.currentPage.Init()
}

func (model *Model) ensureMCPPage(resourceID string) error {
	if model.router.Current().Kind != RouteMCP || (resourceID != "" && model.router.Current().ResourceID != resourceID) {
		model.navigate(Route{Kind: RouteMCP, ResourceID: resourceID})
	}
	if model.currentPage == nil {
		return fmt.Errorf("MCP page is unavailable")
	}
	return nil
}

func (model *Model) ensureTunnelPage(command tuipage.TunnelCommand, resourceID string) error {
	managed := command == tuipage.TunnelManagedRefresh || command == tuipage.TunnelManagedCreate || command == tuipage.TunnelManagedUpdate || command == tuipage.TunnelManagedConfigure || command == tuipage.TunnelManagedDelete
	kind := RouteTunnel
	if managed {
		kind = RouteTunnels
	}
	if model.router.Current().Kind != kind || (resourceID != "" && model.router.Current().ResourceID != resourceID) {
		model.navigate(Route{Kind: kind, ResourceID: resourceID})
	}
	if model.currentPage == nil {
		return fmt.Errorf("tunnel page is unavailable")
	}
	return nil
}

func (model *Model) ensureRequestPage(resourceID string) error {
	if model.router.Current().Kind != RouteRequests || (resourceID != "" && model.router.Current().ResourceID != resourceID) {
		model.navigate(Route{Kind: RouteRequests, ResourceID: resourceID})
	}
	if model.currentPage == nil {
		return fmt.Errorf("approval inbox is unavailable")
	}
	return nil
}

func (model *Model) ensureConfigPage() error {
	if model.router.Current().Kind != RouteConfig {
		model.navigate(Route{Kind: RouteConfig})
	}
	if model.currentPage == nil {
		return fmt.Errorf("config center is unavailable")
	}
	return nil
}

func (model Model) updatePage(message tea.Msg) (tea.Model, tea.Cmd) {
	if model.currentPage == nil {
		return model, nil
	}
	before := pageNotice(model.currentPage)
	updated, cmd := model.currentPage.Update(message)
	model.currentPage = updated
	after := pageNotice(model.currentPage)
	if after != "" && after != before {
		if page, ok := model.currentPage.(tuipage.ToastNoticeModel); ok && !page.ShouldToastNotice() {
			return model, cmd
		}
		if page, ok := model.currentPage.(tuipage.NoticeModel); ok {
			page.SetNotice("")
		}
		return model, tea.Batch(cmd, model.showToast(model.router.Current().Title(), after, component.ToneNeutral))
	}
	return model, cmd
}

func pageNotice(value tuipage.Model) string {
	if page, ok := value.(tuipage.NoticeModel); ok {
		return strings.TrimSpace(page.Notice())
	}
	return ""
}

func (model *Model) showToast(title, message string, tone component.Tone) tea.Cmd {
	title = strings.TrimSpace(title)
	message = strings.TrimSpace(message)
	if title == "" && message == "" {
		return nil
	}
	model.toastSeq++
	model.toast = toastState{id: model.toastSeq, timer: 1, title: title, message: message, tone: tone}
	return model.toastTimerCmd()
}

func (model *Model) toastTimerCmd() tea.Cmd {
	if model == nil || model.toast.id == 0 || model.toast.hovered {
		return nil
	}
	id, timer := model.toast.id, model.toast.timer
	return tea.Tick(toastDuration, func(time.Time) tea.Msg { return toastDismissMsg{id: id, timer: timer} })
}

func (model *Model) dismissToast() {
	if model != nil {
		model.toast = toastState{}
	}
}

func (model *Model) ensureWorkspacePage(command tuipage.WorkspaceCommand, resourceID string) error {
	container := command == tuipage.WorkspaceContainerCreate || command == tuipage.WorkspaceContainerRename || command == tuipage.WorkspaceContainerDelete || command == tuipage.WorkspaceContainerMembers
	kind := RouteWorkspaces
	if container {
		kind = RouteContainers
	}
	if model.router.Current().Kind != kind || (resourceID != "" && model.router.Current().ResourceID != resourceID) {
		model.navigate(Route{Kind: kind, ResourceID: resourceID})
	}
	if model.currentPage == nil {
		return fmt.Errorf("workspace page is unavailable")
	}
	return nil
}

type mousePage interface {
	MouseTargets(originX, originY, z int) []component.MouseTarget
}

type frameMetrics struct {
	contentWidth int
	contentX     int
	bodyY        int
	bodyHeight   int
	showNavbar   bool
	showFooter   bool
}

func (model Model) render() (string, []component.MouseTarget) {
	width, height := model.layoutSize()
	if width <= 0 || height <= 0 {
		return "", nil
	}
	metrics := model.frameMetrics(width, height)
	targets := []component.MouseTarget{}
	border := lipgloss.NewStyle().Foreground(model.theme.border.GetBorderLeftForeground())
	lines := make([]string, 0, height)
	lines = append(lines, model.topBorder(width, border))
	if metrics.showNavbar {
		header, headerTargets := model.header(metrics.contentWidth, metrics.contentX, 1)
		targets = append(targets, headerTargets...)
		lines = append(lines, frameLine(header, width, border))
		lines = append(lines, frameDivider(width, border))
	}
	body := fitFrameContent(model.page(metrics.contentWidth, metrics.bodyHeight), metrics.contentWidth, metrics.bodyHeight)
	for _, line := range body {
		lines = append(lines, frameLine(line, width, border))
	}
	if metrics.showFooter {
		lines = append(lines, frameDivider(width, border))
		lines = append(lines, frameLine(fitFrameLine(model.shortcutFooter(), metrics.contentWidth), width, border))
	}
	if len(lines) < height {
		lines = append(lines, bottomBorder(width, border))
	}
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}
	if len(lines) > height {
		lines = lines[:height]
	}
	if page, ok := model.currentPage.(mousePage); ok {
		targets = append(targets, page.MouseTargets(metrics.contentX, metrics.bodyY, 10)...)
	}
	if model.router.Current().Kind == RouteHome && model.homeCommands != nil {
		panelWidth := homeCommandPanelWidth(metrics.contentWidth)
		panel := model.homeCommands.View(panelWidth)
		x := metrics.contentX + max(0, (metrics.contentWidth-lipgloss.Width(panel))/2)
		y := metrics.bodyY + max(0, (metrics.bodyHeight-lipgloss.Height(panel))/2)
		targets = append(targets, model.homeCommands.MouseTargets(x, y, 10, panelWidth)...)
	}
	for index := range lines {
		lines[index] = fitTerminalLine(lines[index], width)
	}
	return strings.Join(lines, "\n"), targets
}

func (model Model) topBorder(width int, border lipgloss.Style) string {
	if width <= 0 {
		return ""
	}
	if width == 1 {
		return border.Render("─")
	}
	if width == 2 {
		return border.Render("╭╮")
	}
	label := " " + model.theme.title.Render("ChatGPT MCP") + " "
	used := 2 + lipgloss.Width(label) + 1
	if used > width {
		return border.Render("╭" + strings.Repeat("─", width-2) + "╮")
	}
	return border.Render("╭─") + label + border.Render(strings.Repeat("─", max(0, width-used))+"╮")
}

func bottomBorder(width int, border lipgloss.Style) string {
	if width <= 0 {
		return ""
	}
	if width == 1 {
		return border.Render("─")
	}
	if width == 2 {
		return border.Render("╰╯")
	}
	return border.Render("╰" + strings.Repeat("─", width-2) + "╯")
}

func (model Model) shortcutFooter() string {
	width, _ := model.layoutSize()
	width, _ = frameContentMetrics(width)
	if width <= 0 {
		return ""
	}
	bindings := []key.Binding{
		component.Binding([]string{"ctrl+k"}, "ctrl+k", "commands"),
		component.Binding([]string{"alt+left", "alt+right"}, "alt+←/→", "pages"),
	}
	if model.router.Current().Kind == RouteHome {
		bindings = append(bindings, component.Binding([]string{"esc"}, "esc", "quit"))
	} else if len(model.router.stack) > 1 {
		bindings = append(bindings, component.Binding([]string{"esc"}, "esc", "back"))
	} else {
		bindings = append(bindings, component.Binding([]string{"esc"}, "esc", "home"))
	}
	return component.DefaultHelp(width, bindings...)
}

func (model Model) header(width, originX, originY int) (string, []component.MouseTarget) {
	owner := headerOwner(model.router.Current().Kind)
	parts := make([]string, 0, len(headerPages))
	targets := make([]component.MouseTarget, 0, len(headerPages))
	x := 0
	for index, page := range headerPages {
		style := component.NavItemStyle(page.Kind == owner)
		cellWidth := width / len(headerPages)
		if index < width%len(headerPages) {
			cellWidth++
		}
		label := ansi.Truncate(page.Label, max(1, cellWidth), "")
		button := style.Padding(0).Width(cellWidth).Align(lipgloss.Center).Render(label)
		kind := page.Kind
		targets = append(targets, component.MouseTarget{
			ID: "app.header." + string(kind), Rect: component.Rect{X: originX + x, Y: originY, Width: cellWidth, Height: 1}, Z: 1,
			Handle: func(event component.MouseEvent) tea.Msg {
				if event.Button != tea.MouseLeft {
					return nil
				}
				return navigateMsg{route: Route{Kind: kind}, sibling: true}
			},
		})
		parts = append(parts, button)
		x += cellWidth
	}
	return fitFrameLine(strings.Join(parts, ""), width), targets
}

func (model Model) page(width, height int) string {
	if model.currentPage != nil {
		return model.currentPage.View(width, height)
	}
	route := model.router.Current()
	if route.Kind == RouteHome {
		return model.homeView(width, height)
	}
	description := component.WrapContent(model.theme.muted.Render(routeDescription(route)), width)
	notice := ""
	if model.notice != "" {
		notice = "\n\n" + component.WrapContent(model.theme.muted.Render(model.notice), width)
	}
	ready := component.WrapContent(model.theme.subtle.Render("Command Center shell is ready. Domain actions will be added through the shared action registry."), width)
	return component.PageTitle(route.Title(), width) + "\n" + description + notice + "\n\n" + ready
}

func (model Model) homeView(width, height int) string {
	if model.homeCommands == nil {
		return component.CenterLayout(component.Muted("Command panel unavailable"), width, height)
	}
	return component.CenterLayout(model.homeCommands.View(homeCommandPanelWidth(width)), width, height)
}

func homeCommandPanelWidth(width int) int {
	if width <= 0 {
		return 72
	}
	return max(1, min(78, width-4))
}

func (model *Model) resetHomeCommands() {
	if model == nil {
		return
	}
	ctx := actionContext(Route{Kind: RouteHome})
	actions, resources, err := model.commandActions(ctx)
	if err != nil {
		actions = model.actions.Actions(ctx)
		resources = nil
	}
	value := palette.NewWithOptions(actions, ctx, palette.Options{
		Title: "Commands", Hint: "Ctrl+K", Placeholder: "Type a command or resource",
		Footer: "↑/↓ navigate  ·  Enter run  ·  Esc exit  ·  Alt+←/→ pages", Recent: model.state.RecentActions,
	})
	model.homeCommands = &value
	model.commandResources = resources
}

func (model Model) layoutSize() (int, int) {
	width, height := model.width, model.height
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	return width, height
}

func frameContentMetrics(width int) (contentWidth, originX int) {
	switch {
	case width >= 4:
		return width - 4, 2
	case width == 3:
		return 1, 1
	default:
		return 0, 0
	}
}

func (model Model) frameMetrics(width, height int) frameMetrics {
	contentWidth, contentX := frameContentMetrics(width)
	showNavbar := model.showNavbar(contentWidth, height)
	showFooter := height >= 6 && contentWidth >= 16
	fixedHeight := 2
	bodyY := 1
	if showNavbar {
		fixedHeight += 2
		bodyY = 3
	}
	if showFooter {
		fixedHeight += 2
	}
	return frameMetrics{
		contentWidth: contentWidth,
		contentX:     contentX,
		bodyY:        bodyY,
		bodyHeight:   max(0, height-fixedHeight),
		showNavbar:   showNavbar,
		showFooter:   showFooter,
	}
}

func (model Model) showNavbar(contentWidth, height int) bool {
	if height < navbarMinHeight || contentWidth <= 0 || len(headerPages) == 0 {
		return false
	}
	maxLabelWidth := 0
	for _, page := range headerPages {
		maxLabelWidth = max(maxLabelWidth, lipgloss.Width(page.Label))
	}
	return contentWidth/len(headerPages) >= maxLabelWidth
}

func frameLine(content string, width int, border lipgloss.Style) string {
	if width <= 0 {
		return ""
	}
	if width == 1 {
		return border.Render("│")
	}
	if width == 2 {
		return border.Render("││")
	}
	if width == 3 {
		return border.Render("│") + fitFrameLine(content, 1) + border.Render("│")
	}
	return border.Render("│") + " " + fitFrameLine(content, width-4) + " " + border.Render("│")
}

func frameDivider(width int, border lipgloss.Style) string {
	if width <= 0 {
		return ""
	}
	if width == 1 {
		return border.Render("─")
	}
	if width == 2 {
		return border.Render("├┤")
	}
	return border.Render("├" + strings.Repeat("─", width-2) + "┤")
}

func fitFrameContent(content string, width, height int) []string {
	lines := strings.Split(content, "\n")
	result := make([]string, height)
	for index := range result {
		if index < len(lines) {
			result[index] = fitFrameLine(lines[index], width)
		} else {
			result[index] = strings.Repeat(" ", width)
		}
	}
	return result
}

func fitFrameLine(line string, width int) string {
	if width <= 0 {
		return ""
	}
	line = ansi.Truncate(line, width, "")
	return line + strings.Repeat(" ", max(0, width-lipgloss.Width(line)))
}

func fitTerminalLine(line string, width int) string {
	if width <= 0 {
		return ""
	}
	line = ansi.Truncate(line, width, "")
	return line + strings.Repeat(" ", max(0, width-lipgloss.Width(line)))
}

func routeDescription(route Route) string {
	if route.ResourceID != "" {
		return fmt.Sprintf("Deep-linked resource: %s", route.ResourceID)
	}
	switch route.Kind {
	case RouteHome:
		return "Keyboard-first command center for chatgpt-mcp."
	case RouteWorkspaces:
		return "Browse registered workspaces and workspace containers."
	case RouteContainers:
		return "Browse the Containers tab inside Workspaces."
	case RouteMCP:
		return "Manage configured upstream MCP servers."
	case RouteTunnel:
		return "Manage runtime and OpenAI Secure MCP Tunnel state."
	case RouteTunnels:
		return "Browse and manage tunnels available through the OpenAI Tunnel Management API."
	case RouteRequests:
		return "Review control approval requests."
	case RouteLogs:
		return "Inspect runtime history and live events."
	case RouteLogsExec:
		return "Inspect live command execution output."
	case RouteConfig:
		return "Browse and manage validated runtime configuration."
	case RouteRuntime:
		return "Inspect and control the local managed runtime."
	case RouteAbout:
		return "Build and runtime information."
	default:
		return ""
	}
}
