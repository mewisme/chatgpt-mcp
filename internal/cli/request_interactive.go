package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
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

type requestInteractiveModel struct {
	ctx           context.Context
	client        requestInteractiveClient
	keys          interactive.ListKeys
	cursor        interactive.Cursor
	confirm       interactive.Confirmation
	viewport      viewport.Model
	requests      []approval.Request
	filter        string
	filtering     bool
	detail        bool
	detailRequest approval.Request
	loading       bool
	busy          bool
	width         int
	height        int
	now           time.Time
	notice        string
	err           error
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

func newRequestInteractiveModel(ctx context.Context, requests []approval.Request, client requestInteractiveClient) requestInteractiveModel {
	if ctx == nil {
		ctx = context.Background()
	}
	view := viewport.New(viewport.WithWidth(74), viewport.WithHeight(12))
	view.SoftWrap = true
	view.FillHeight = false
	return requestInteractiveModel{ctx: ctx, client: client, keys: interactive.DefaultListKeys(), viewport: view, requests: requestInteractivePending(requests), now: time.Now()}
}

func defaultRequestInteractiveClient() requestInteractiveClient {
	return requestInteractiveClient{list: requestRuntimeApprovalList, view: requestRuntimeApprovalView, approve: requestRuntimeApprovalApprove, deny: requestRuntimeApprovalDeny}
}

func (m requestInteractiveModel) Init() tea.Cmd { return requestInteractiveTickCmd() }

func (m requestInteractiveModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.BackgroundColorMsg:
		interactive.SetDarkBackground(msg.IsDark())
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resizeViewport()
	case requestInteractiveTickMsg:
		m.now = time.Time(msg)
		if m.detail {
			m.syncDetailViewport()
		}
		return m, tea.Batch(requestInteractiveTickCmd(), m.refreshCmd())
	case requestInteractiveListMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.requests = requestInteractivePending(msg.requests)
		m.cursor.Clamp(len(m.filtered()))
		if m.detail {
			if current, ok := requestFind(m.requests, m.detailRequest.ID); ok {
				m.detailRequest = current
				m.syncDetailViewport()
			} else {
				m.detail, m.detailRequest = false, approval.Request{}
			}
		}
	case requestInteractiveDetailMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.detail, m.detailRequest = true, msg.request
		m.syncDetailViewport()
		m.viewport.GotoTop()
	case requestInteractiveResolveMsg:
		m.busy = false
		if msg.err != nil {
			m.err = msg.err
			return m, m.refreshCmd()
		}
		m.err = nil
		m.notice = fmt.Sprintf("%s %s", requestActionTitle(msg.action), msg.request.ID)
		m.requests = requestRemove(m.requests, msg.request.ID)
		m.cursor.Clamp(len(m.filtered()))
		m.detail, m.detailRequest = false, approval.Request{}
		return m, m.refreshCmd()
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
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
	if m.filtering {
		keyValue := tea.Key(msg)
		switch msg.String() {
		case "enter", "esc":
			m.filtering = false
		case "backspace":
			m.filter = trimLastRune(m.filter)
			m.cursor.Clamp(len(m.filtered()))
		case "ctrl+u":
			m.filter = ""
			m.cursor.Index = 0
		case "ctrl+c":
			return m, tea.Quit
		default:
			if keyValue.Text != "" {
				m.filter += keyValue.Text
				m.cursor.Index = 0
			}
		}
		return m, nil
	}
	if m.detail {
		switch {
		case key.Matches(msg, m.keys.Quit):
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			m.detail, m.detailRequest = false, approval.Request{}
		case msg.String() == "esc" || key.Matches(msg, m.keys.Open):
			m.detail, m.detailRequest = false, approval.Request{}
		case key.Matches(msg, m.keys.Approve):
			m.startConfirmation("approve", m.detailRequest)
		case key.Matches(msg, m.keys.Deny):
			m.startConfirmation("deny", m.detailRequest)
		case key.Matches(msg, m.keys.Refresh):
			m.loading = true
			return m, tea.Batch(m.refreshCmd(), m.detailCmd(m.detailRequest.ID))
		default:
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
		return m, nil
	}
	items := m.filtered()
	switch {
	case key.Matches(msg, m.keys.Quit):
		return m, tea.Quit
	case msg.String() == "esc":
		if m.filter != "" {
			m.filter = ""
			m.cursor.Index = 0
			return m, nil
		}
		return m, tea.Quit
	case key.Matches(msg, m.keys.Up):
		m.cursor.Move(-1, len(items))
	case key.Matches(msg, m.keys.Down):
		m.cursor.Move(1, len(items))
	case key.Matches(msg, m.keys.Filter):
		m.filtering = true
	case key.Matches(msg, m.keys.Refresh):
		m.loading = true
		return m, m.refreshCmd()
	case key.Matches(msg, m.keys.Open):
		if selected, ok := m.selected(); ok {
			m.loading = true
			return m, m.detailCmd(selected.ID)
		}
	case key.Matches(msg, m.keys.Approve):
		if selected, ok := m.selected(); ok {
			m.startConfirmation("approve", selected)
		}
	case key.Matches(msg, m.keys.Deny):
		if selected, ok := m.selected(); ok {
			m.startConfirmation("deny", selected)
		}
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
	var builder strings.Builder
	items := m.filtered()
	meta := fmt.Sprintf("%d requests", len(items))
	if m.loading {
		meta = interactive.Accent("⠋") + " " + interactive.Muted("refreshing")
	}
	builder.WriteString(interactive.Header("Pending control approvals", meta, m.contentWidth()))
	if filter := interactive.Filter(m.filter, m.filtering); filter != "" {
		builder.WriteString("\n")
		builder.WriteString(filter)
	}
	if m.err != nil {
		builder.WriteString("\n")
		builder.WriteString(interactive.Banner(m.err.Error(), interactive.ToneDanger))
	}
	if m.notice != "" {
		builder.WriteString("\n")
		builder.WriteString(interactive.Banner(m.notice, interactive.ToneSuccess))
	}
	builder.WriteString("\n\n")
	if m.detail {
		m.writeDetail(&builder)
	} else {
		m.writeList(&builder)
	}
	if m.confirm.Active() {
		builder.WriteString("\n")
		message := fmt.Sprintf("%s %s? [y/N]", requestActionTitle(m.confirm.Action), m.confirm.Target)
		builder.WriteString(interactive.Banner(message, interactive.ToneWarning))
		builder.WriteString("\n")
	}
	view := tea.NewView(builder.String())
	view.AltScreen = true
	return view
}

func (m requestInteractiveModel) writeList(builder *strings.Builder) {
	items := m.filtered()
	if len(items) == 0 {
		message := "No pending approval requests."
		if strings.TrimSpace(m.filter) != "" {
			message = "No pending requests match the current filter."
		}
		builder.WriteString(interactive.Muted(message))
		builder.WriteString("\n")
	} else {
		maxRows := m.listCapacity()
		start, end := requestVisibleWindow(m.cursor.Index, len(items), maxRows)
		for index := start; index < end; index++ {
			request := items[index]
			selected := index == m.cursor.Index
			title := request.Title
			if strings.TrimSpace(title) == "" {
				title = request.ID
			}
			status := interactive.ToneText(strings.ToUpper(string(request.Status)), requestStatusTone(request.Status))
			countdown := requestCountdown(request, m.now)
			right := status
			if countdown != "" && countdown != string(request.Status) {
				right += "  " + interactive.Secondary(countdown)
			}
			left := interactive.CursorMark(selected) + interactive.Primary(requestTruncate(title, max(12, m.contentWidth()-32)), selected)
			builder.WriteString(interactive.TwoColumn(left, right, m.contentWidth()))
			builder.WriteString("\n")
			meta := strings.Join(nonEmptyStrings(shortRequestID(request.ID), request.WorkspaceID, request.TargetTool), " · ")
			builder.WriteString("  ")
			builder.WriteString(interactive.SelectedSecondary(requestTruncate(meta, max(12, m.contentWidth()-2)), selected))
			builder.WriteString("\n")
		}
	}
	builder.WriteString("\n")
	builder.WriteString(interactive.Help(m.contentWidth(),
		interactive.HelpItem{Key: "↑/k", Desc: "up"}, interactive.HelpItem{Key: "↓/j", Desc: "down"}, interactive.HelpItem{Key: "enter", Desc: "details"},
		interactive.HelpItem{Key: "a", Desc: "approve"}, interactive.HelpItem{Key: "d", Desc: "deny"}, interactive.HelpItem{Key: "/", Desc: "filter"},
		interactive.HelpItem{Key: "r", Desc: "refresh"}, interactive.HelpItem{Key: "q", Desc: "quit"},
	))
	builder.WriteString("\n")
}

func (m requestInteractiveModel) writeDetail(builder *strings.Builder) {
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
	builder.WriteString(interactive.TwoColumn(interactive.Title(title), right, m.contentWidth()))
	builder.WriteString("\n\n")
	builder.WriteString(interactive.Panel(m.viewport.View(), max(12, m.contentWidth()-2)))
	builder.WriteString("\n\n")
	builder.WriteString(interactive.Help(m.contentWidth(),
		interactive.HelpItem{Key: "j/k", Desc: "scroll"}, interactive.HelpItem{Key: "pgup/pgdn", Desc: "page"}, interactive.HelpItem{Key: "a", Desc: "approve"},
		interactive.HelpItem{Key: "d", Desc: "deny"}, interactive.HelpItem{Key: "r", Desc: "refresh"}, interactive.HelpItem{Key: "esc", Desc: "back"}, interactive.HelpItem{Key: "ctrl+c", Desc: "quit"},
	))
	builder.WriteString("\n")
}

func (m requestInteractiveModel) filtered() []approval.Request {
	query := strings.ToLower(strings.TrimSpace(m.filter))
	if query == "" {
		return m.requests
	}
	result := make([]approval.Request, 0, len(m.requests))
	for _, request := range m.requests {
		haystack := strings.ToLower(strings.Join([]string{request.ID, string(request.Status), request.WorkspaceID, request.TargetTool, request.Source, request.Title}, " "))
		if strings.Contains(haystack, query) {
			result = append(result, request)
		}
	}
	return result
}

func (m requestInteractiveModel) selected() (approval.Request, bool) {
	items := m.filtered()
	if len(items) == 0 || m.cursor.Index < 0 || m.cursor.Index >= len(items) {
		return approval.Request{}, false
	}
	return items[m.cursor.Index], true
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
	m.viewport.SetHeight(max(5, m.height-12))
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

func (m requestInteractiveModel) listCapacity() int {
	if m.height <= 0 {
		return 8
	}
	return max(3, (m.height-10)/2)
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

func requestVisibleWindow(cursor, length, maxRows int) (int, int) {
	if length <= maxRows {
		return 0, length
	}
	start := cursor - maxRows/2
	if start < 0 {
		start = 0
	}
	if start+maxRows > length {
		start = length - maxRows
	}
	return start, start + maxRows
}

func requestTruncate(value string, width int) string {
	if width <= 0 || utf8.RuneCountInString(value) <= width {
		return value
	}
	if width <= 3 {
		return string([]rune(value)[:width])
	}
	return string([]rune(value)[:width-3]) + "..."
}

func shortRequestID(id string) string {
	if len(id) <= 14 {
		return id
	}
	return id[:14]
}

func trimLastRune(value string) string {
	if value == "" {
		return ""
	}
	_, size := utf8.DecodeLastRuneInString(value)
	return value[:len(value)-size]
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
