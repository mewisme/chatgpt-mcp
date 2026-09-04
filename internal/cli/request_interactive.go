package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
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
	return requestInteractiveModel{ctx: ctx, client: client, keys: interactive.DefaultListKeys(), requests: append([]approval.Request(nil), requests...), now: time.Now()}
}

func defaultRequestInteractiveClient() requestInteractiveClient {
	return requestInteractiveClient{list: requestRuntimeApprovalList, view: requestRuntimeApprovalView, approve: requestRuntimeApprovalApprove, deny: requestRuntimeApprovalDeny}
}

func (m requestInteractiveModel) Init() tea.Cmd { return requestInteractiveTickCmd() }

func (m requestInteractiveModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case requestInteractiveTickMsg:
		m.now = time.Time(msg)
		return m, tea.Batch(requestInteractiveTickCmd(), m.refreshCmd())
	case requestInteractiveListMsg:
		m.loading = false
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.err = nil
		m.requests = msg.requests
		m.cursor.Clamp(len(m.filtered()))
		if m.detail {
			if current, ok := requestFind(msg.requests, m.detailRequest.ID); ok {
				m.detailRequest = current
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
	case requestInteractiveResolveMsg:
		m.busy = false
		if msg.err != nil {
			m.err = msg.err
			return m, m.refreshCmd()
		}
		m.err = nil
		m.notice = fmt.Sprintf("%s %s", requestActionTitle(msg.action), msg.request.ID)
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
	builder.WriteString("Control approval requests")
	if m.loading {
		builder.WriteString("  refreshing...")
	}
	builder.WriteString("\n")
	if m.filtering || m.filter != "" {
		builder.WriteString("Filter: ")
		builder.WriteString(m.filter)
		if m.filtering {
			builder.WriteString("_")
		}
		builder.WriteString("\n")
	}
	if m.err != nil {
		builder.WriteString("Error: ")
		builder.WriteString(m.err.Error())
		builder.WriteString("\n")
	}
	if m.notice != "" {
		builder.WriteString(m.notice)
		builder.WriteString("\n")
	}
	if m.detail {
		m.writeDetail(&builder)
	} else {
		m.writeList(&builder)
	}
	if m.confirm.Active() {
		builder.WriteString("\n")
		builder.WriteString(requestActionTitle(m.confirm.Action))
		builder.WriteString(" ")
		builder.WriteString(m.confirm.View())
		builder.WriteString("\n")
	}
	view := tea.NewView(builder.String())
	view.AltScreen = true
	return view
}

func (m requestInteractiveModel) writeList(builder *strings.Builder) {
	items := m.filtered()
	if len(items) == 0 {
		builder.WriteString("\nNo requests match the current filter.\n")
	} else {
		maxRows := m.height - 7
		if maxRows < 3 {
			maxRows = 8
		}
		start, end := requestVisibleWindow(m.cursor.Index, len(items), maxRows)
		for index := start; index < end; index++ {
			request := items[index]
			prefix := "  "
			if index == m.cursor.Index {
				prefix = "> "
			}
			line := fmt.Sprintf("%s%-9s %-14s %-16s %s", prefix, requestCountdown(request, m.now), shortRequestID(request.ID), request.WorkspaceID, request.Title)
			builder.WriteString(requestClip(line, m.width))
			builder.WriteString("\n")
		}
	}
	builder.WriteString("\nup/k down/j move  enter/v details  a approve  d deny  / filter  r refresh  q quit\n")
}

func (m requestInteractiveModel) writeDetail(builder *strings.Builder) {
	request := m.detailRequest
	builder.WriteString("\n")
	builder.WriteString(request.Title)
	builder.WriteString("\n")
	builder.WriteString("Request:   " + request.ID + "\n")
	builder.WriteString("Status:    " + string(request.Status) + "\n")
	builder.WriteString("Workspace: " + request.WorkspaceID + "\n")
	builder.WriteString("Tool:      " + request.TargetTool + "\n")
	builder.WriteString("Source:    " + request.Source + "\n")
	builder.WriteString("Expires:   " + requestCountdown(request, m.now) + "\n")
	if request.GuardReason != "" {
		builder.WriteString("Guard:     " + request.GuardReason + "\n")
	}
	if len(request.Arguments) > 0 {
		builder.WriteString("Arguments:\n")
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
	builder.WriteString("\nesc/enter back  a approve  d deny  r refresh  q quit\n")
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

func requestClip(value string, width int) string {
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
