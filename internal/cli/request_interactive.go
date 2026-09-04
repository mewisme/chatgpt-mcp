package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"go.mewis.me/chatgpt-mcp/internal/approval"
	"go.mewis.me/chatgpt-mcp/internal/cli/interactive"
)

const requestInteractiveRefreshInterval = time.Second

type requestInteractiveClient struct {
	list    func(context.Context) ([]approval.Request, error)
	view    func(context.Context, string) (approval.Request, error)
	approve func(context.Context, string, string) (approval.Request, error)
	deny    func(context.Context, string, string) (approval.Request, error)
}

type requestListItem struct {
	request approval.Request
	now     time.Time
}

func (i requestListItem) Title() string {
	if value := strings.TrimSpace(i.request.Title); value != "" {
		return value
	}
	return i.request.ID
}

func (i requestListItem) Description() string {
	parts := nonEmptyStrings(shortRequestID(i.request.ID), i.request.WorkspaceID, i.request.TargetTool, strings.ToUpper(string(i.request.Status)))
	if countdown := requestCountdown(i.request, i.now); countdown != "" && countdown != string(i.request.Status) {
		parts = append(parts, countdown)
	}
	return strings.Join(parts, " · ")
}

func (i requestListItem) FilterValue() string {
	return strings.Join([]string{i.request.ID, string(i.request.Status), i.request.WorkspaceID, i.request.TargetTool, i.request.Source, i.request.Title}, " ")
}

type requestInteractiveModel struct {
	ctx                context.Context
	client             requestInteractiveClient
	list               list.Model
	confirm            interactive.Confirmation
	viewport           viewport.Model
	requests           []approval.Request
	detail             bool
	detailRequest      approval.Request
	loading            bool
	busy               bool
	width              int
	height             int
	now                time.Time
	notice             string
	err                error
	pendingSelectionID string
}

type requestInteractiveListMsg struct {
	requests []approval.Request
	err      error
}
type requestInteractiveDetailMsg struct {
	request approval.Request
	err     error
}
type requestInteractiveResolveMsg struct {
	action  string
	request approval.Request
	err     error
}
type requestInteractiveTickMsg time.Time

var requestOpenBinding = interactive.Binding([]string{"enter", "v"}, "enter", "details")
var requestApproveBinding = interactive.Binding([]string{"a"}, "a", "approve")
var requestDenyBinding = interactive.Binding([]string{"d"}, "d", "deny")
var requestRefreshBinding = interactive.Binding([]string{"r"}, "r", "refresh")

func newRequestInteractiveModel(ctx context.Context, requests []approval.Request, client requestInteractiveClient) requestInteractiveModel {
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now()
	pending := requestInteractivePending(requests)
	listModel := interactive.NewDefaultList("Pending control approvals", requestListItems(pending, now), 80, 20, "request", "requests")
	syncRequestListHelp(&listModel)
	view := viewport.New(viewport.WithWidth(74), viewport.WithHeight(12))
	view.SoftWrap = true
	view.FillHeight = false
	return requestInteractiveModel{ctx: ctx, client: client, list: listModel, viewport: view, requests: pending, now: now}
}

func defaultRequestInteractiveClient() requestInteractiveClient {
	return requestInteractiveClient{list: requestRuntimeApprovalList, view: requestRuntimeApprovalView, approve: requestRuntimeApprovalApprove, deny: requestRuntimeApprovalDeny}
}

func (m requestInteractiveModel) Init() tea.Cmd { return requestInteractiveTickCmd() }

func (m requestInteractiveModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.BackgroundColorMsg:
		interactive.ApplyDefaultListTheme(&m.list, msg.IsDark())
		syncRequestListHelp(&m.list)
		return m, nil
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.list.SetSize(msg.Width, msg.Height)
		m.resizeViewport()
		return m, nil
	case requestInteractiveTickMsg:
		m.now = time.Time(msg)
		if m.detail {
			m.syncDetailViewport()
		}
		selectedID := m.selectedID()
		listCmd := m.syncListItems(selectedID)
		return m, tea.Batch(requestInteractiveTickCmd(), m.refreshCmd(), listCmd)
	case requestInteractiveListMsg:
		m.loading = false
		m.list.StopSpinner()
		if msg.err != nil {
			m.err = msg.err
			return m, m.list.NewStatusMessage("Refresh failed: " + msg.err.Error())
		}
		m.err = nil
		selectedID := m.selectedID()
		m.requests = requestInteractivePending(msg.requests)
		listCmd := m.syncListItems(selectedID)
		if m.detail {
			if current, ok := requestFind(m.requests, m.detailRequest.ID); ok {
				m.detailRequest = current
				m.syncDetailViewport()
			} else {
				m.detail, m.detailRequest = false, approval.Request{}
			}
		}
		return m, listCmd
	case list.FilterMatchesMsg:
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		if m.pendingSelectionID != "" {
			m.restoreSelection(m.pendingSelectionID)
			m.pendingSelectionID = ""
		}
		return m, cmd
	case requestInteractiveDetailMsg:
		m.loading = false
		m.list.StopSpinner()
		if msg.err != nil {
			m.err = msg.err
			return m, m.list.NewStatusMessage(msg.err.Error())
		}
		m.err = nil
		m.detail, m.detailRequest = true, msg.request
		m.syncDetailViewport()
		m.viewport.GotoTop()
		return m, nil
	case requestInteractiveResolveMsg:
		m.busy = false
		if msg.err != nil {
			m.err = msg.err
			return m, tea.Batch(m.list.NewStatusMessage(msg.err.Error()), m.refreshCmd())
		}
		m.err = nil
		m.notice = fmt.Sprintf("%s %s", requestActionTitle(msg.action), msg.request.ID)
		m.requests = requestRemove(m.requests, msg.request.ID)
		m.detail, m.detailRequest = false, approval.Request{}
		listCmd := m.syncListItems("")
		return m, tea.Batch(m.list.NewStatusMessage(m.notice), listCmd, m.refreshCmd())
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(message)
	return m, cmd
}

func (m requestInteractiveModel) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.confirm.Active() {
		switch msg.String() {
		case "y", "Y":
			action, target := m.confirm.Action, m.confirm.Target
			m.confirm.Clear()
			m.busy = true
			return m, m.resolveCmd(action, target)
		case "ctrl+c":
			return m, tea.Quit
		default:
			m.confirm.Clear()
			return m, nil
		}
	}
	if m.detail {
		switch {
		case msg.String() == "ctrl+c":
			return m, tea.Quit
		case msg.String() == "q", msg.String() == "esc", key.Matches(msg, requestOpenBinding):
			m.detail, m.detailRequest = false, approval.Request{}
		case key.Matches(msg, requestApproveBinding):
			m.startConfirmation("approve", m.detailRequest)
		case key.Matches(msg, requestDenyBinding):
			m.startConfirmation("deny", m.detailRequest)
		case key.Matches(msg, requestRefreshBinding):
			m.loading = true
			m.list.StartSpinner()
			return m, tea.Batch(m.refreshCmd(), m.detailCmd(m.detailRequest.ID))
		default:
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
		return m, nil
	}
	if m.list.FilterState() == list.Filtering {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}
	switch {
	case key.Matches(msg, requestOpenBinding):
		if selected, ok := m.selected(); ok {
			m.loading = true
			m.list.StartSpinner()
			return m, m.detailCmd(selected.ID)
		}
	case key.Matches(msg, requestApproveBinding):
		if selected, ok := m.selected(); ok {
			m.startConfirmation("approve", selected)
		}
	case key.Matches(msg, requestDenyBinding):
		if selected, ok := m.selected(); ok {
			m.startConfirmation("deny", selected)
		}
	case key.Matches(msg, requestRefreshBinding):
		m.loading = true
		m.list.StartSpinner()
		return m, m.refreshCmd()
	default:
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *requestInteractiveModel) startConfirmation(action string, request approval.Request) {
	if request.Status != approval.StatusPending || m.busy {
		m.notice = fmt.Sprintf("Request %s is %s and cannot be resolved", request.ID, request.Status)
		return
	}
	m.confirm.Start(action, request.ID)
}

func (m requestInteractiveModel) View() tea.View {
	content := m.list.View()
	if m.detail {
		content = m.detailView()
	} else if m.confirm.Active() {
		message := fmt.Sprintf("%s %s? [y/N]", requestActionTitle(m.confirm.Action), m.confirm.Target)
		content += "\n" + interactive.Banner(message, interactive.ToneWarning) + "\n"
	}
	view := tea.NewView(content)
	view.AltScreen = true
	return view
}

func (m requestInteractiveModel) detailView() string {
	request := m.detailRequest
	title := request.Title
	if strings.TrimSpace(title) == "" {
		title = request.ID
	}
	status := interactive.ToneText(strings.ToUpper(string(request.Status)), requestStatusTone(request.Status))
	right := status
	if countdown := requestCountdown(request, m.now); countdown != "" && countdown != string(request.Status) {
		right += "  " + interactive.Secondary(countdown)
	}
	var builder strings.Builder
	builder.WriteString(interactive.TwoColumn(interactive.Title(title), right, m.contentWidth()))
	if m.err != nil {
		builder.WriteString("\n")
		builder.WriteString(interactive.Banner(m.err.Error(), interactive.ToneDanger))
	}
	if m.notice != "" {
		builder.WriteString("\n")
		builder.WriteString(interactive.Banner(m.notice, interactive.ToneSuccess))
	}
	builder.WriteString("\n\n")
	builder.WriteString(interactive.Panel(m.viewport.View(), max(12, m.contentWidth()-2)))
	builder.WriteString("\n\n")
	builder.WriteString(interactive.DefaultHelp(m.contentWidth(),
		interactive.Binding([]string{"j", "k"}, "j/k", "scroll"), interactive.Binding([]string{"pgup", "pgdown"}, "pgup/pgdn", "page"), requestApproveBinding,
		requestDenyBinding, requestRefreshBinding, interactive.Binding([]string{"esc"}, "esc", "back"), interactive.Binding([]string{"ctrl+c"}, "ctrl+c", "quit"),
	))
	builder.WriteString("\n")
	return builder.String()
}

func (m requestInteractiveModel) selected() (approval.Request, bool) {
	item, ok := m.list.SelectedItem().(requestListItem)
	if !ok {
		return approval.Request{}, false
	}
	return item.request, true
}

func (m requestInteractiveModel) selectedID() string {
	selected, ok := m.selected()
	if !ok {
		return ""
	}
	return selected.ID
}

func (m *requestInteractiveModel) syncListItems(selectedID string) tea.Cmd {
	cmd := m.list.SetItems(requestListItems(m.requests, m.now))
	if cmd != nil {
		m.pendingSelectionID = selectedID
	} else {
		m.restoreSelection(selectedID)
	}
	return cmd
}

func (m *requestInteractiveModel) restoreSelection(id string) {
	if id == "" {
		return
	}
	for index, item := range m.list.VisibleItems() {
		value, ok := item.(requestListItem)
		if ok && value.request.ID == id {
			m.list.Select(index)
			return
		}
	}
}

func (m requestInteractiveModel) refreshCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, requestControlTimeout)
		defer cancel()
		requests, err := m.client.list(ctx)
		return requestInteractiveListMsg{requests: requests, err: err}
	}
}

func (m requestInteractiveModel) detailCmd(id string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, requestControlTimeout)
		defer cancel()
		request, err := m.client.view(ctx, id)
		return requestInteractiveDetailMsg{request: request, err: err}
	}
}

func (m requestInteractiveModel) resolveCmd(action, id string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(m.ctx, requestControlTimeout)
		defer cancel()
		var request approval.Request
		var err error
		if action == "approve" {
			request, err = m.client.approve(ctx, id, "")
		} else {
			request, err = m.client.deny(ctx, id, "")
		}
		return requestInteractiveResolveMsg{action: action, request: request, err: err}
	}
}

func (m *requestInteractiveModel) resizeViewport() {
	m.viewport.SetWidth(max(12, m.contentWidth()-6))
	m.viewport.SetHeight(max(5, m.height-6))
}

func (m *requestInteractiveModel) syncDetailViewport() {
	if !m.detail {
		return
	}
	request := m.detailRequest
	var builder strings.Builder
	builder.WriteString(interactive.KeyValue("Request", request.ID))
	builder.WriteString("\n")
	builder.WriteString(interactive.KeyValue("Status", interactive.ToneText(string(request.Status), requestStatusTone(request.Status))))
	builder.WriteString("\n")
	builder.WriteString(interactive.KeyValue("Workspace", request.WorkspaceID))
	builder.WriteString("\n")
	builder.WriteString(interactive.KeyValue("Tool", request.TargetTool))
	builder.WriteString("\n")
	if request.Source != "" {
		builder.WriteString(interactive.KeyValue("Source", request.Source))
		builder.WriteString("\n")
	}
	builder.WriteString(interactive.KeyValue("Expires", requestCountdown(request, m.now)))
	builder.WriteString("\n")
	if request.GuardReason != "" {
		builder.WriteString(interactive.KeyValue("Guard", request.GuardReason))
		builder.WriteString("\n")
	}
	if len(request.Arguments) > 0 {
		builder.WriteString("\n")
		builder.WriteString(interactive.Title("Arguments"))
		builder.WriteString("\n")
		var value any
		if json.Unmarshal(request.Arguments, &value) == nil {
			if data, err := json.MarshalIndent(value, "", "  "); err == nil {
				builder.Write(data)
				builder.WriteString("\n")
			}
		} else {
			builder.Write(request.Arguments)
			builder.WriteString("\n")
		}
	}
	m.viewport.SetContent(strings.TrimSuffix(builder.String(), "\n"))
}

func (m requestInteractiveModel) contentWidth() int {
	if m.width <= 0 {
		return 80
	}
	return max(20, m.width-2)
}

func syncRequestListHelp(model *list.Model) {
	bindings := []key.Binding{requestOpenBinding, requestApproveBinding, requestDenyBinding, requestRefreshBinding}
	model.AdditionalShortHelpKeys = func() []key.Binding { return append([]key.Binding(nil), bindings...) }
	model.AdditionalFullHelpKeys = func() []key.Binding { return append([]key.Binding(nil), bindings...) }
}

func requestListItems(requests []approval.Request, now time.Time) []list.Item {
	items := make([]list.Item, 0, len(requests))
	for _, request := range requests {
		items = append(items, requestListItem{request: request, now: now})
	}
	return items
}

func requestInteractiveTickCmd() tea.Cmd {
	return tea.Tick(requestInteractiveRefreshInterval, func(now time.Time) tea.Msg { return requestInteractiveTickMsg(now) })
}

func requestFind(requests []approval.Request, id string) (approval.Request, bool) {
	for _, request := range requests {
		if request.ID == id {
			return request, true
		}
	}
	return approval.Request{}, false
}

func requestInteractivePending(requests []approval.Request) []approval.Request {
	result := make([]approval.Request, 0, len(requests))
	for _, request := range requests {
		if request.Status == approval.StatusPending {
			result = append(result, request)
		}
	}
	return result
}

func requestRemove(requests []approval.Request, id string) []approval.Request {
	result := requests[:0]
	for _, request := range requests {
		if request.ID != id {
			result = append(result, request)
		}
	}
	return result
}

func requestCountdown(request approval.Request, now time.Time) string {
	switch request.Status {
	case approval.StatusPending:
		remaining := time.Until(request.ExpiresAt)
		if !now.IsZero() {
			remaining = request.ExpiresAt.Sub(now)
		}
		if remaining <= 0 {
			return "expired"
		}
		return fmt.Sprintf("%ds", int(remaining.Round(time.Second)/time.Second))
	case approval.StatusApproved:
		if request.RetryUntil.IsZero() {
			return "approved"
		}
		remaining := request.RetryUntil.Sub(now)
		if remaining <= 0 {
			return "expired"
		}
		return fmt.Sprintf("retry %ds", int(remaining.Round(time.Second)/time.Second))
	default:
		return string(request.Status)
	}
}

func requestStatusTone(status approval.Status) interactive.Tone {
	switch status {
	case approval.StatusPending:
		return interactive.ToneWarning
	case approval.StatusApproved, approval.StatusConsumed:
		return interactive.ToneSuccess
	case approval.StatusDenied:
		return interactive.ToneDanger
	default:
		return interactive.ToneNeutral
	}
}

func shortRequestID(id string) string {
	if len(id) <= 14 {
		return id
	}
	return id[:14]
}

func requestActionTitle(action string) string {
	if action == "" {
		return ""
	}
	return strings.ToUpper(action[:1]) + action[1:]
}

func nonEmptyStrings(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}
