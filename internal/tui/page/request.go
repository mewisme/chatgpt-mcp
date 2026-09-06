package page

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/approval"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
)

const requestRefreshInterval = time.Second
const requestOperationTimeout = 5 * time.Second

type RequestCommand string

const (
	RequestRefresh     RequestCommand = "request.refresh"
	RequestApprove     RequestCommand = "request.approve"
	RequestDeny        RequestCommand = "request.deny"
	RequestShowPending RequestCommand = "request.show.pending"
	RequestShowHistory RequestCommand = "request.show.history"
	RequestShowAll     RequestCommand = "request.show.all"
)

type RequestCommandMsg struct {
	Command    RequestCommand
	ResourceID string
}

type requestMode uint8

const (
	requestModePending requestMode = iota
	requestModeHistory
	requestModeAll
)

type requestOverlay uint8

const (
	requestOverlayNone requestOverlay = iota
	requestOverlayForm
	requestOverlayOperation
)

type requestTickMsg time.Time

type requestListMsg struct {
	requests   []approval.Request
	resource   approval.Request
	resourceOK bool
	err        error
}

type requestResolveMsg struct {
	request approval.Request
	approve bool
	err     error
}

type RequestsPage struct {
	ctx                context.Context
	requests           []approval.Request
	browser            component.Browser
	mode               requestMode
	resourceID         string
	pendingResourceID  string
	loading            bool
	overlay            requestOverlay
	form               component.Form
	resolveForm        *requestResolveFormData
	resolveApprove     bool
	resolveID          string
	operationCancel    context.CancelFunc
	operationCancelled bool
	progress           *component.Progress
	notice             string
	err                error
	width              int
	height             int
}

func NewRequests(ctx context.Context, resourceID string) (*RequestsPage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	resourceID = strings.TrimSpace(resourceID)
	mode := requestModePending
	if resourceID != "" {
		mode = requestModeAll
	}
	page := &RequestsPage{ctx: ctx, mode: mode, resourceID: resourceID, pendingResourceID: resourceID}
	page.rebuildBrowser("")
	return page, nil
}

func (page *RequestsPage) Init() tea.Cmd {
	if page == nil {
		return nil
	}
	page.loading = true
	return tea.Batch(requestTickCmd(), page.refreshCmd())
}

func (page *RequestsPage) OverlayActive() bool {
	return page != nil && (page.overlay != requestOverlayNone || page.browser.DetailOpen())
}

func (page *RequestsPage) InputActive() bool {
	return page != nil && (page.overlay == requestOverlayForm || page.browser.InputActive())
}

func (page *RequestsPage) Update(message tea.Msg) (Model, tea.Cmd) {
	if page == nil {
		return page, nil
	}
	switch msg := message.(type) {
	case requestTickMsg:
		commands := []tea.Cmd{requestTickCmd()}
		if !page.loading && page.overlay != requestOverlayOperation {
			page.loading = true
			commands = append(commands, page.refreshCmd())
		}
		return page, tea.Batch(commands...)
	case requestListMsg:
		page.loading = false
		if msg.err != nil {
			page.err = msg.err
			return page, nil
		}
		page.err = nil
		page.requests = append([]approval.Request(nil), msg.requests...)
		if msg.resourceOK {
			page.upsertRequest(msg.resource)
			page.resourceID = msg.resource.ID
			page.pendingResourceID = ""
		}
		selectedID := page.selectedID()
		if page.resourceID != "" {
			selectedID = page.resourceID
		}
		page.rebuildBrowser(selectedID)
		return page, nil
	case requestResolveMsg:
		if page.operationCancel != nil {
			page.operationCancel()
		}
		page.operationCancel = nil
		page.overlay = requestOverlayNone
		page.progress = nil
		if page.operationCancelled {
			page.operationCancelled = false
			page.notice = "Approval operation cancelled"
			return page, nil
		}
		if msg.err != nil {
			page.err = msg.err
			return page, nil
		}
		page.err = nil
		page.upsertRequest(msg.request)
		page.notice = requestResolveNotice(msg.approve, msg.request)
		page.resolveID = ""
		page.rebuildBrowser(msg.request.ID)
		return page, page.manualRefreshCmd()
	case tea.WindowSizeMsg:
		page.width, page.height = msg.Width, msg.Height
		updated, cmd := page.browser.Update(msg)
		page.browser = updated.(component.Browser)
		if page.overlay == requestOverlayForm {
			form, formCmd := page.form.Update(msg)
			page.form = form
			return page, tea.Batch(cmd, formCmd)
		}
		return page, cmd
	case component.FormSubmittedMsg:
		return page, page.submitResolveForm()
	case component.FormCancelledMsg:
		page.closeOverlay()
		return page, nil
	case component.FormMouseMsg:
		if page.overlay == requestOverlayForm {
			updated, cmd := page.form.Update(msg)
			page.form = updated
			return page, cmd
		}
		return page, nil
	case RequestCommandMsg:
		return page, page.handleCommand(msg.Command, msg.ResourceID)
	case tea.KeyPressMsg:
		if page.overlay == requestOverlayOperation {
			if msg.String() == "esc" {
				page.cancelOperation()
				return page, nil
			}
			if page.progress != nil {
				updated, cmd := page.progress.Update(msg)
				page.progress = &updated
				return page, cmd
			}
			return page, nil
		}
		if page.overlay == requestOverlayForm {
			updated, cmd := page.form.Update(msg)
			page.form = updated
			return page, cmd
		}
		if page.browser.InputActive() {
			updated, cmd := page.browser.Update(msg)
			page.browser = updated.(component.Browser)
			return page, cmd
		}
		if page.browser.DetailOpen() {
			updated, cmd := page.browser.Update(msg)
			page.browser = updated.(component.Browser)
			return page, cmd
		}
		switch msg.String() {
		case "1":
			page.setMode(requestModePending)
			return page, nil
		case "2":
			page.setMode(requestModeHistory)
			return page, nil
		case "3":
			page.setMode(requestModeAll)
			return page, nil
		case "r":
			return page, page.manualRefreshCmd()
		}
	}
	if page.overlay == requestOverlayForm {
		updated, cmd := page.form.Update(message)
		page.form = updated
		return page, cmd
	}
	updated, cmd := page.browser.Update(message)
	page.browser = updated.(component.Browser)
	return page, cmd
}

func (page *RequestsPage) View(width, height int) string {
	if page == nil {
		return component.StateView(component.PageError, "Approval inbox unavailable", "")
	}
	page.width, page.height = width, height
	page.browser.SetTitleNotice(page.notice)
	feedback := ""
	if page.err != nil {
		feedback = component.Banner(page.err.Error(), component.ToneDanger)
	}
	browserHeight := max(1, height-pageFeedbackHeight(feedback))
	updated, _ := page.browser.Update(tea.WindowSizeMsg{Width: width, Height: browserHeight})
	page.browser = updated.(component.Browser)
	content := prependPageFeedback(feedback, page.browser.Content())
	if page.overlay == requestOverlayForm {
		content = component.CenterOverlay(content, component.Modal(page.form.View(), overlayWidth(width, 76)), width, height)
	}
	if page.overlay == requestOverlayOperation {
		body := ""
		if page.progress != nil {
			body = page.progress.View()
		}
		body += "\n\n" + component.Muted("Esc cancel")
		content = component.CenterOverlay(content, component.Modal(body, overlayWidth(width, 68)), width, height)
	}
	return content
}

func (page *RequestsPage) MouseTargets(originX, originY, z int) []component.MouseTarget {
	if page == nil {
		return nil
	}
	switch page.overlay {
	case requestOverlayForm:
		return formOverlayMouseTargets(page.form, overlayWidth(page.width, 76), page.width, page.height, originX, originY, z+20)
	case requestOverlayOperation:
		return []component.MouseTarget{mouseBlocker(originX, originY, page.width, page.height, z+20)}
	default:
		page.browser.SetTitleNotice(page.notice)
		feedback := ""
		if page.err != nil {
			feedback = component.Banner(page.err.Error(), component.ToneDanger)
		}
		return page.browser.MouseTargets(originX, originY+pageFeedbackHeight(feedback), z)
	}
}

func (page *RequestsPage) handleCommand(command RequestCommand, resourceID string) tea.Cmd {
	page.err, page.notice = nil, ""
	switch command {
	case RequestRefresh:
		return page.manualRefreshCmd()
	case RequestShowPending:
		page.setMode(requestModePending)
		return nil
	case RequestShowHistory:
		page.setMode(requestModeHistory)
		return nil
	case RequestShowAll:
		page.setMode(requestModeAll)
		return nil
	case RequestApprove, RequestDeny:
		id := strings.TrimSpace(resourceID)
		if id == "" {
			id = page.selectedID()
		}
		request, ok := page.findRequest(id)
		if !ok {
			page.err = fmt.Errorf("approval request not found: %s", id)
			return nil
		}
		if request.Status != approval.StatusPending {
			page.err = fmt.Errorf("request %s is %s and cannot be resolved", request.ID, request.Status)
			return nil
		}
		page.resolveApprove = command == RequestApprove
		page.resolveID = request.ID
		page.form, page.resolveForm = newRequestResolveForm(request, page.resolveApprove)
		page.overlay = requestOverlayForm
		return page.form.Init()
	default:
		page.err = fmt.Errorf("unsupported request action: %s", command)
		return nil
	}
}

func (page *RequestsPage) submitResolveForm() tea.Cmd {
	if page.resolveForm == nil || page.resolveID == "" {
		page.err = fmt.Errorf("approval resolution form is unavailable")
		page.closeOverlay()
		return nil
	}
	if !page.resolveForm.Confirm {
		page.notice = "Approval request unchanged"
		page.closeOverlay()
		return nil
	}
	id, approve, reason := page.resolveID, page.resolveApprove, strings.TrimSpace(page.resolveForm.Reason)
	page.resolveForm = nil
	ctx, cancel := context.WithTimeout(page.ctx, requestOperationTimeout)
	page.operationCancel = cancel
	page.operationCancelled = false
	progress := component.NewProgress(requestProgressTitle(approve))
	page.progress = &progress
	page.overlay = requestOverlayOperation
	return func() tea.Msg {
		request, err := application.ResolveApprovalRequest(ctx, id, approve, reason)
		return requestResolveMsg{request: request, approve: approve, err: err}
	}
}

func (page *RequestsPage) manualRefreshCmd() tea.Cmd {
	if page.loading {
		return nil
	}
	page.loading = true
	return page.refreshCmd()
}

func (page *RequestsPage) refreshCmd() tea.Cmd {
	resourceID := page.pendingResourceID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(page.ctx, requestOperationTimeout)
		defer cancel()
		requests, err := application.ListApprovalRequests(ctx)
		if err != nil {
			return requestListMsg{err: err}
		}
		result := requestListMsg{requests: requests}
		if resourceID != "" {
			request, err := application.GetApprovalRequest(ctx, resourceID)
			if err != nil {
				result.err = err
				return result
			}
			result.resource, result.resourceOK = request, true
		}
		return result
	}
}

func (page *RequestsPage) cancelOperation() {
	if page.operationCancel != nil {
		page.operationCancel()
	}
	page.operationCancelled = true
	page.overlay = requestOverlayNone
	page.progress = nil
	page.notice = "Approval operation cancellation requested"
}

func (page *RequestsPage) closeOverlay() {
	page.overlay = requestOverlayNone
	page.form = component.Form{}
	page.resolveForm = nil
	page.resolveID = ""
	page.progress = nil
}

func (page *RequestsPage) setMode(mode requestMode) {
	if page.mode == mode {
		return
	}
	selected := page.selectedID()
	page.mode = mode
	page.resourceID = ""
	page.rebuildBrowser(selected)
}

func (page *RequestsPage) rebuildBrowser(selectedID string) {
	detailOpen := page.browser.DetailOpen()
	helpExpanded := page.browser.HelpExpanded()
	if selectedID == "" {
		selectedID = page.selectedID()
	}
	rows := page.requestRows()
	browser := component.NewBrowser(page.ctx, "Approval requests · "+page.modeLabel(), rows, nil)
	browser = browser.WithHelpBindings(component.Binding([]string{"1"}, "1", "pending"), component.Binding([]string{"2"}, "2", "history"), component.Binding([]string{"3"}, "3", "all"), component.Binding([]string{"r"}, "r", "refresh"))
	browser = browser.WithAction(component.RowAction{Key: "a", Desc: "approve", Run: requestRowAction(RequestApprove)})
	browser = browser.WithAction(component.RowAction{Key: "d", Desc: "deny", Run: requestRowAction(RequestDeny)})
	page.browser = browser
	page.browser.SetHelpExpanded(helpExpanded)
	if page.width > 0 && page.height > 0 {
		updated, _ := page.browser.Update(tea.WindowSizeMsg{Width: page.width, Height: page.height})
		page.browser = updated.(component.Browser)
	}
	if selectedID != "" {
		page.browser.SelectID(selectedID)
		if detailOpen || page.resourceID == selectedID {
			page.browser.OpenDetail(selectedID)
		}
	}
}

func (page *RequestsPage) requestRows() []component.Row {
	now := time.Now()
	rows := make([]component.Row, 0, len(page.requests))
	for _, request := range page.requests {
		if !page.modeIncludes(request.Status) {
			continue
		}
		title := strings.TrimSpace(request.Title)
		if title == "" {
			title = request.ID
		}
		meta := strings.ToUpper(string(request.Status))
		if countdown := requestCountdownLabel(request, now); countdown != "" {
			meta += " · " + countdown
		}
		rows = append(rows, component.Row{
			ID: request.ID, Title: title, Description: strings.Join(nonEmptyRequestStrings(shortApprovalRequestID(request.ID), request.WorkspaceID, request.TargetTool), " · "), Meta: meta,
			Search:      strings.Join([]string{request.ID, string(request.Status), request.WorkspaceID, request.TargetTool, request.Source, request.Title}, " "),
			DetailTitle: "Approval request · " + request.ID,
			DetailTabs: []component.DetailTab{
				{Title: "Overview", Content: requestOverview(request)},
				{Title: "Arguments", Content: requestArguments(request)},
				{Title: "Guard", Content: requestGuard(request)},
			},
		})
	}
	return rows
}

func (page *RequestsPage) modeIncludes(status approval.Status) bool {
	switch page.mode {
	case requestModePending:
		return status == approval.StatusPending
	case requestModeHistory:
		return status != approval.StatusPending
	default:
		return true
	}
}

func (page *RequestsPage) modeLabel() string {
	switch page.mode {
	case requestModeHistory:
		return "History"
	case requestModeAll:
		return "All"
	default:
		return "Pending"
	}
}

func (page *RequestsPage) selectedID() string {
	selected, ok := page.browser.Selected()
	if !ok {
		return ""
	}
	return selected.ID
}

func (page *RequestsPage) findRequest(id string) (approval.Request, bool) {
	id = strings.TrimSpace(id)
	for _, request := range page.requests {
		if request.ID == id {
			return request, true
		}
	}
	return approval.Request{}, false
}

func (page *RequestsPage) upsertRequest(value approval.Request) {
	for index := range page.requests {
		if page.requests[index].ID == value.ID {
			page.requests[index] = value
			return
		}
	}
	page.requests = append(page.requests, value)
}

func requestRowAction(command RequestCommand) func(component.Row) (string, tea.Cmd, error) {
	return func(row component.Row) (string, tea.Cmd, error) {
		return "", func() tea.Msg { return RequestCommandMsg{Command: command, ResourceID: row.ID} }, nil
	}
}

func requestTickCmd() tea.Cmd {
	return tea.Tick(requestRefreshInterval, func(now time.Time) tea.Msg { return requestTickMsg(now) })
}

func requestOverview(request approval.Request) string {
	return detailFields(
		[2]string{"Status", string(request.Status)}, [2]string{"Title", request.Title}, [2]string{"Workspace", request.WorkspaceID}, [2]string{"Tool", request.TargetTool},
		[2]string{"Source", request.Source}, [2]string{"Session", request.SessionHash}, [2]string{"Created", requestTime(request.CreatedAt)}, [2]string{"Expires", requestTime(request.ExpiresAt)},
		[2]string{"Resolved", requestTime(request.ResolvedAt)}, [2]string{"Resolved by", request.ResolvedBy}, [2]string{"Reason", request.Reason}, [2]string{"Retry until", requestTime(request.RetryUntil)}, [2]string{"Consumed", requestTime(request.ConsumedAt)},
	)
}

func requestArguments(request approval.Request) string {
	if len(request.Arguments) == 0 {
		return component.Muted("No arguments")
	}
	var value any
	if json.Unmarshal(request.Arguments, &value) == nil {
		if data, err := json.MarshalIndent(value, "", "  "); err == nil {
			return string(data)
		}
	}
	return string(request.Arguments)
}

func requestGuard(request approval.Request) string {
	guard := strings.TrimSpace(string(request.GuardCode))
	if guard == "" {
		guard = "control-plane mutation"
	}
	return detailFields([2]string{"Guard", guard}, [2]string{"Reason", request.GuardReason})
}

func requestTime(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.Local().Format("2006-01-02 15:04:05 MST")
}

func requestCountdownLabel(request approval.Request, now time.Time) string {
	switch request.Status {
	case approval.StatusPending:
		remaining := request.ExpiresAt.Sub(now)
		if remaining <= 0 {
			return "expired"
		}
		return fmt.Sprintf("%ds", int(remaining.Round(time.Second)/time.Second))
	case approval.StatusApproved:
		if request.RetryUntil.IsZero() {
			return ""
		}
		remaining := request.RetryUntil.Sub(now)
		if remaining <= 0 {
			return "expired"
		}
		return fmt.Sprintf("retry %ds", int(remaining.Round(time.Second)/time.Second))
	default:
		return ""
	}
}

func shortApprovalRequestID(id string) string {
	if len(id) <= 14 {
		return id
	}
	return id[:14]
}

func nonEmptyRequestStrings(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func requestProgressTitle(approve bool) string {
	if approve {
		return "Approving control request"
	}
	return "Denying control request"
}

func requestResolveNotice(approve bool, request approval.Request) string {
	if approve {
		return "Approved " + request.ID
	}
	return "Denied " + request.ID
}
