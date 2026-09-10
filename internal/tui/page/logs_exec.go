package page

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"go.mewis.me/chatgpt-mcp/internal/runtimecontrol"
	shellruntime "go.mewis.me/chatgpt-mcp/internal/shell"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
)

const logsExecutionFeedCap = 4000

type logsTab int

const (
	logsTabRuntime logsTab = iota
	logsTabCommandExec
)

var logsTabLabels = []string{"Runtime", "Command Execution"}

type logsExecutionFeed struct {
	viewport          viewport.Model
	events            []shellruntime.ExecutionFeedEvent
	scopeMode         executionScopeMode
	workspaceID       string
	containerID       string
	containerName     string
	containerMembers  map[string]struct{}
	scopeStale        bool
	scopeNotice       string
	scopeEditor       *component.Editor
	scopeForm         *executionScopeFormData
	stream            *runtimecontrol.ExecutionFeedStream
	streamCtx         context.Context
	streamCancel      context.CancelFunc
	generation        uint64
	latestSeq         uint64
	loaded            bool
	loading           bool
	connected         bool
	reconnecting      bool
	unsupported       bool
	paused            bool
	notice            string
	err               error
	restoreYOffset    int
	restoreYOffsetSet bool
}

type logsExecutionOpenMsg struct {
	generation uint64
	stream     *runtimecontrol.ExecutionFeedStream
	err        error
}

type logsExecutionEventMsg struct {
	generation uint64
	event      shellruntime.ExecutionFeedEvent
	err        error
}

type logsExecutionReconnectMsg uint64

type logsExecutionMouseMsg struct{ Wheel int }

func newLogsExecutionFeed() logsExecutionFeed {
	view := viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))
	view.SoftWrap = false
	view.FillHeight = false
	return logsExecutionFeed{viewport: view, scopeMode: executionScopeCombined, containerMembers: map[string]struct{}{}}
}

func (page *LogsPage) switchLogsTab(tab logsTab) tea.Cmd {
	if tab > logsTabCommandExec || page.resourceID != "" {
		return nil
	}
	page.tab = tab
	if tab == logsTabRuntime && !page.loaded && !page.loading {
		return page.startBootstrap()
	}
	if tab == logsTabCommandExec && !page.exec.loaded && !page.exec.loading {
		return page.startExecutionFeed()
	}
	return nil
}

func (page *LogsPage) moveLogsTab(delta int) tea.Cmd {
	next := component.MoveTab(int(page.tab), len(logsTabLabels), delta)
	return page.switchLogsTab(logsTab(next))
}

func (page *LogsPage) startExecutionFeed() tea.Cmd {
	page.stopExecutionFeed()
	page.refreshExecutionScope()
	page.exec.generation++
	generation := page.exec.generation
	ctx, cancel := context.WithCancel(page.ctx)
	page.exec.streamCtx, page.exec.streamCancel = ctx, cancel
	page.exec.loading, page.exec.reconnecting = true, page.exec.loaded
	page.exec.unsupported = false
	page.exec.err = nil
	return func() tea.Msg {
		stream, _, err := runtimecontrol.OpenExecutionFeed(ctx)
		return logsExecutionOpenMsg{generation: generation, stream: stream, err: err}
	}
}

func (page *LogsPage) finishExecutionFeedOpen(msg logsExecutionOpenMsg) tea.Cmd {
	if msg.generation != page.exec.generation {
		if msg.stream != nil {
			_ = msg.stream.Close()
		}
		return nil
	}
	page.exec.loading = false
	if msg.err != nil {
		page.exec.stream = nil
		page.exec.loaded = true
		page.exec.err = nil
		if errors.Is(msg.err, runtimecontrol.ErrExecutionFeedUnsupported) {
			page.exec.connected, page.exec.reconnecting, page.exec.unsupported = false, false, true
			page.exec.notice = "Restart the running server to enable command execution streaming"
			return nil
		}
		page.exec.connected, page.exec.reconnecting, page.exec.unsupported = false, true, false
		page.exec.notice = "Runtime offline; reconnecting command execution stream"
		return page.executionReconnectCmd(msg.generation)
	}
	page.exec.stream, page.exec.connected, page.exec.reconnecting, page.exec.loaded, page.exec.unsupported = msg.stream, true, false, true, false
	snapshot := msg.stream.Snapshot()
	page.exec.latestSeq = snapshot.LatestSequence
	page.exec.events = trimExecutionFeed(snapshot.Events)
	page.refreshExecutionScope()
	page.exec.notice, page.exec.err = "", nil
	page.refreshExecutionViewport()
	page.restoreExecutionViewportOffset()
	return page.nextExecutionEventCmd(msg.generation)
}

func (page *LogsPage) nextExecutionEventCmd(generation uint64) tea.Cmd {
	stream := page.exec.stream
	if stream == nil {
		return nil
	}
	return func() tea.Msg {
		event, err := stream.Next()
		return logsExecutionEventMsg{generation: generation, event: event, err: err}
	}
}

func (page *LogsPage) finishExecutionFeedEvent(msg logsExecutionEventMsg) tea.Cmd {
	if msg.generation != page.exec.generation {
		return nil
	}
	if msg.err != nil {
		if errors.Is(msg.err, runtimecontrol.ErrExecutionFeedOverflow) {
			page.exec.notice = "Command stream overflowed; replaying bounded history"
		} else if msg.err == io.EOF || page.ctx.Err() == nil {
			page.exec.notice = "Command stream disconnected; reconnecting"
		} else {
			return nil
		}
		page.exec.connected, page.exec.reconnecting = false, true
		page.stopExecutionFeedOnly()
		return page.executionReconnectCmd(msg.generation)
	}
	if msg.event.Sequence == 0 || msg.event.Sequence <= page.exec.latestSeq {
		return page.nextExecutionEventCmd(msg.generation)
	}
	if page.exec.latestSeq > 0 && msg.event.Sequence > page.exec.latestSeq+1 {
		page.exec.notice = "Command stream gap detected; replaying bounded history"
		return page.startExecutionFeed()
	}
	page.exec.latestSeq = msg.event.Sequence
	page.exec.events = trimExecutionFeed(append(page.exec.events, msg.event))
	page.exec.notice, page.exec.err = "", nil
	page.refreshExecutionViewport()
	return page.nextExecutionEventCmd(msg.generation)
}

func (page *LogsPage) executionReconnectCmd(generation uint64) tea.Cmd {
	return tea.Tick(logsReconnectDelay, func(time.Time) tea.Msg { return logsExecutionReconnectMsg(generation) })
}

func (page *LogsPage) handleExecutionKey(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "space":
		page.exec.paused = !page.exec.paused
		if !page.exec.paused {
			page.exec.viewport.GotoBottom()
		}
		return nil
	case "r":
		return page.startExecutionFeed()
	case "f":
		return page.openExecutionScopeEditor()
	case "c":
		page.exec.events = nil
		page.exec.notice = "Command stream view cleared"
		page.refreshExecutionViewport()
		return nil
	}
	view, cmd := page.exec.viewport.Update(msg)
	page.exec.viewport = view
	if !page.exec.viewport.AtBottom() {
		page.exec.paused = true
	}
	return cmd
}

func (page *LogsPage) resizeExecutionViewport(width, height int) {
	offset := page.exec.viewport.YOffset()
	page.exec.viewport.SetWidth(max(1, width))
	page.exec.viewport.SetHeight(max(1, height))
	page.exec.viewport.SetContent(component.WrapContent(formatExecutionFeed(page.visibleExecutionEvents(), max(1, width)), max(1, width)))
	if !page.exec.paused {
		page.exec.viewport.GotoBottom()
		return
	}
	maxOffset := max(0, page.exec.viewport.TotalLineCount()-page.exec.viewport.Height())
	page.exec.viewport.SetYOffset(min(offset, maxOffset))
}

func (page *LogsPage) refreshExecutionViewport() {
	offset := page.exec.viewport.YOffset()
	page.exec.viewport.SetContent(component.WrapContent(formatExecutionFeed(page.visibleExecutionEvents(), max(1, page.exec.viewport.Width())), max(1, page.exec.viewport.Width())))
	if !page.exec.paused {
		page.exec.viewport.GotoBottom()
		return
	}
	maxOffset := max(0, page.exec.viewport.TotalLineCount()-page.exec.viewport.Height())
	page.exec.viewport.SetYOffset(min(offset, maxOffset))
}

func (page *LogsPage) restoreExecutionViewportOffset() {
	if page == nil || !page.exec.restoreYOffsetSet {
		return
	}
	page.exec.restoreYOffsetSet = false
	if !page.exec.paused {
		page.exec.viewport.GotoBottom()
		return
	}
	maxOffset := max(0, page.exec.viewport.TotalLineCount()-page.exec.viewport.Height())
	page.exec.viewport.SetYOffset(min(page.exec.restoreYOffset, maxOffset))
}

func (page *LogsPage) executionStatusView(width int) string {
	stream := component.ToneText("● LIVE", component.ToneSuccess)
	if page.exec.loading && !page.exec.loaded {
		stream = component.Muted("↻ LOADING")
	} else if page.exec.unsupported {
		stream = component.ToneText("○ RESTART REQUIRED", component.ToneWarning)
	} else if page.exec.reconnecting {
		stream = component.ToneText("↻ RECONNECTING", component.ToneWarning)
	} else if !page.exec.connected {
		stream = component.Muted("○ OFFLINE")
	}
	follow := component.ToneText("● ON", component.ToneSuccess)
	if page.exec.paused {
		follow = component.ToneText("○ PAUSED", component.ToneWarning)
	}
	left := component.KeyValue("Stream", stream) + "   " + component.KeyValue("Follow", follow) + "   " + component.KeyValue("Events", fmt.Sprintf("%d / %d", len(page.visibleExecutionEvents()), logsExecutionFeedCap))
	return component.TwoColumn(left, component.KeyValue("Mode", page.executionScopeLabel()), width)
}

func (page *LogsPage) executionHelpView(width int) string {
	return component.NewHelpFooter(
		component.Binding([]string{"h", "l", "left", "right"}, "←/→", "tabs"),
		component.Binding([]string{"space"}, "space", executionFollowLabel(page.exec.paused)),
		component.Binding([]string{"f"}, "f", "scope"), component.Binding([]string{"r"}, "r", "reconnect"), component.Binding([]string{"c"}, "c", "clear view"),
	).View(width)
}

func (page *LogsPage) executionBodyView(width, height int) string {
	status := page.executionStatusView(width)
	message := ""
	if page.exec.err != nil {
		message = component.BannerWidth(page.exec.err.Error(), component.ToneDanger, width)
	} else if page.exec.notice != "" {
		message = component.WrapContent(component.Muted(page.exec.notice), width)
	} else if page.exec.scopeNotice != "" {
		message = component.WrapContent(component.Muted(page.exec.scopeNotice), width)
	}
	reserved := lipgloss.Height(status) + 1
	if message != "" {
		reserved += lipgloss.Height(message) + 1
	}
	bodyHeight := max(1, height-reserved)
	page.resizeExecutionViewport(width, bodyHeight)
	body := page.exec.viewport.View()
	if strings.TrimSpace(formatExecutionFeed(page.visibleExecutionEvents(), width)) == "" {
		empty := page.exec.viewport
		empty.SetContent(component.Muted("Waiting for command output"))
		body = empty.View()
	}
	content := status + "\n" + body
	if message != "" {
		content += "\n" + message
	}
	return content
}

func (page *LogsPage) logsTabMouseTargets(originX, originY, z int) []component.MouseTarget {
	_, spans := component.PageTabsLayout(logsTabLabels, int(page.tab), page.notice, page.width)
	targets := make([]component.MouseTarget, 0, len(spans))
	for _, span := range spans {
		tab := logsTab(span.Index)
		targets = append(targets, component.MouseTarget{
			ID: "logs.tab", Rect: component.Rect{X: originX + span.X, Y: originY, Width: span.Width, Height: 1}, Z: z,
			Handle: func(event component.MouseEvent) tea.Msg {
				if event.Button != tea.MouseLeft {
					return nil
				}
				if tab == logsTabCommandExec {
					return tea.KeyPressMsg{Code: '2'}
				}
				return tea.KeyPressMsg{Code: '1'}
			},
		})
	}
	return targets
}

func (page *LogsPage) executionMouseTargets(originX, originY, z, width, height int) []component.MouseTarget {
	return []component.MouseTarget{{
		ID: "logs.exec.scroll", Rect: component.Rect{X: originX, Y: originY, Width: width, Height: height}, Z: z,
		Handle: func(event component.MouseEvent) tea.Msg {
			switch event.Button {
			case tea.MouseWheelUp:
				return logsExecutionMouseMsg{Wheel: -1}
			case tea.MouseWheelDown:
				return logsExecutionMouseMsg{Wheel: 1}
			default:
				return nil
			}
		},
	}}
}

func (page *LogsPage) handleExecutionMouse(msg logsExecutionMouseMsg) {
	if msg.Wheel < 0 {
		page.exec.viewport.ScrollUp(3)
	} else if msg.Wheel > 0 {
		page.exec.viewport.ScrollDown(3)
	}
	page.exec.paused = !page.exec.viewport.AtBottom()
}

func (page *LogsPage) stopExecutionFeed() {
	page.stopExecutionFeedOnly()
	if page.exec.streamCancel != nil {
		page.exec.streamCancel()
		page.exec.streamCancel = nil
	}
	page.exec.streamCtx = nil
}

func (page *LogsPage) stopExecutionFeedOnly() {
	if page.exec.stream != nil {
		_ = page.exec.stream.Close()
		page.exec.stream = nil
	}
	page.exec.connected = false
}

func trimExecutionFeed(events []shellruntime.ExecutionFeedEvent) []shellruntime.ExecutionFeedEvent {
	if len(events) <= logsExecutionFeedCap {
		return append([]shellruntime.ExecutionFeedEvent(nil), events...)
	}
	return append([]shellruntime.ExecutionFeedEvent(nil), events[len(events)-logsExecutionFeedCap:]...)
}

func formatExecutionFeed(events []shellruntime.ExecutionFeedEvent, widths ...int) string {
	width := 80
	if len(widths) > 0 && widths[0] > 0 {
		width = widths[0]
	}
	var output strings.Builder
	currentExecutionID := ""
	endsNewline := true
	write := func(value string) {
		if value == "" {
			return
		}
		output.WriteString(value)
		endsNewline = strings.HasSuffix(value, "\n")
	}
	separate := func() {
		if output.Len() == 0 {
			return
		}
		if !endsNewline {
			write("\n")
		}
		write("\n")
	}
	for _, event := range events {
		if event.Type == shellruntime.ExecutionEventStarted {
			separate()
			write(formatExecutionStart(event, width))
			currentExecutionID = event.ExecutionID
		} else if event.ExecutionID != currentExecutionID {
			separate()
			write(formatExecutionContinue(event, width))
			currentExecutionID = event.ExecutionID
		}
		switch event.Type {
		case shellruntime.ExecutionEventOutput:
			write(ansi.Strip(event.Data))
		case shellruntime.ExecutionEventCompleted:
			if output.Len() > 0 && !endsNewline {
				write("\n")
			}
			write(formatExecutionEnd(event, width))
		}
	}
	return output.String()
}

func formatExecutionStart(event shellruntime.ExecutionFeedEvent, width int) string {
	fields := []executionFrameField{}
	info := event.Execution
	if info == nil {
		if event.WorkspaceID != "" {
			fields = append(fields, executionFrameField{Label: "Workspace", Values: []string{event.WorkspaceID}})
		}
		return executionEventFrame("START", event, "", fields, width)
	}
	if info.WorkspaceID != "" {
		fields = append(fields, executionFrameField{Label: "Workspace", Values: []string{info.WorkspaceID}})
	}
	if info.Source != "" {
		fields = append(fields, executionFrameField{Label: "Source", Values: []string{info.Source}})
	}
	if info.SessionHash != "" {
		fields = append(fields, executionFrameField{Label: "Session", Values: []string{info.SessionHash}})
	}
	if info.CallID != "" {
		fields = append(fields, executionFrameField{Label: "Call", Values: []string{info.CallID}})
	}
	route := []string{}
	if info.ReceivedByInstanceID != "" {
		route = append(route, "received: "+info.ReceivedByInstanceID)
	}
	if info.ExecutedByInstanceID != "" {
		route = append(route, "executed: "+info.ExecutedByInstanceID)
	}
	if len(route) > 0 {
		fields = append(fields, executionFrameField{Label: "Route", Values: route})
	}
	if info.CWD != "" {
		fields = append(fields, executionFrameField{Label: "CWD", Values: []string{info.CWD}})
	}
	frame := executionEventFrame("START", event, "", fields, width)
	if info.Command != "" {
		frame += "$ " + component.WrapContent(ansi.Strip(info.Command), max(1, width-2)) + "\n"
	}
	return frame
}

func formatExecutionContinue(event shellruntime.ExecutionFeedEvent, width int) string {
	workspaceID := event.WorkspaceID
	if event.Execution != nil && event.Execution.WorkspaceID != "" {
		workspaceID = event.Execution.WorkspaceID
	}
	fields := []executionFrameField{}
	if workspaceID != "" {
		fields = append(fields, executionFrameField{Label: "Workspace", Values: []string{workspaceID}})
	}
	return executionEventFrame("CONTINUE", event, executionEventElapsed(event), fields, width)
}

func formatExecutionEnd(event shellruntime.ExecutionFeedEvent, width int) string {
	status := strings.TrimSpace(event.Status)
	if status == "" {
		status = "completed"
	}
	fields := []executionFrameField{{Label: "Status", Values: []string{status}}}
	if event.ExitCode != nil {
		fields = append(fields, executionFrameField{Label: "Exit", Values: []string{fmt.Sprintf("%d", *event.ExitCode)}})
	}
	if duration := executionEventDuration(event); duration != "" {
		fields = append(fields, executionFrameField{Label: "Duration", Values: []string{duration}})
	}
	return executionEventFrame("END", event, "", fields, width)
}

type executionFrameField struct {
	Label  string
	Values []string
}

func executionEventFrame(kind string, event shellruntime.ExecutionFeedEvent, elapsed string, fields []executionFrameField, width int) string {
	if width <= 0 {
		return ""
	}
	if width < 4 {
		return strings.Repeat("─", width) + "\n"
	}
	label := strings.TrimSpace(kind + " " + executionEventClock(event) + " " + ansi.Strip(event.ExecutionID))
	if elapsed != "" {
		label += " +" + elapsed
	}
	labelWidth := max(0, width-6)
	label = ansi.Truncate(label, labelWidth, "…")
	topUsed := 4 + lipgloss.Width(label)
	var output strings.Builder
	output.WriteString("╭─ " + label + " " + strings.Repeat("─", max(0, width-topUsed-1)) + "╮\n")
	innerWidth := max(1, width-4)
	for _, field := range fields {
		if strings.TrimSpace(field.Label) == "" || len(field.Values) == 0 {
			continue
		}
		for _, line := range wrappedExecutionFrameLines(field.Label, field.Values, innerWidth) {
			output.WriteString("│ " + line + strings.Repeat(" ", max(0, innerWidth-lipgloss.Width(line))) + " │\n")
		}
	}
	output.WriteString("╰" + strings.Repeat("─", width-2) + "╯\n")
	return output.String()
}

func wrappedExecutionFrameLines(label string, values []string, width int) []string {
	label = ansi.Strip(strings.TrimSpace(label))
	clean := make([]string, 0, len(values))
	for _, value := range values {
		value = ansi.Strip(strings.TrimSpace(value))
		if value != "" {
			clean = append(clean, value)
		}
	}
	if len(clean) == 0 {
		return nil
	}
	if len(clean) == 1 {
		prefix := label + "  "
		if lipgloss.Width(prefix) >= width {
			bullet := "  • "
			available := max(1, width-lipgloss.Width(bullet))
			wrapped := strings.Split(component.WrapContent(clean[0], available), "\n")
			result := []string{ansi.Truncate(label, width, "")}
			for index, line := range wrapped {
				if index == 0 {
					result = append(result, bullet+line)
				} else {
					result = append(result, strings.Repeat(" ", lipgloss.Width(bullet))+line)
				}
			}
			return result
		}
		available := max(1, width-lipgloss.Width(prefix))
		wrapped := strings.Split(component.WrapContent(clean[0], available), "\n")
		result := make([]string, 0, len(wrapped))
		for index, line := range wrapped {
			if index == 0 {
				result = append(result, prefix+line)
			} else {
				result = append(result, strings.Repeat(" ", lipgloss.Width(prefix))+line)
			}
		}
		return result
	}
	result := []string{label}
	for _, value := range clean {
		prefix := "  • "
		available := max(1, width-lipgloss.Width(prefix))
		wrapped := strings.Split(component.WrapContent(value, available), "\n")
		for index, line := range wrapped {
			if index == 0 {
				result = append(result, prefix+line)
			} else {
				result = append(result, strings.Repeat(" ", lipgloss.Width(prefix))+line)
			}
		}
	}
	return result
}

func executionEventClock(event shellruntime.ExecutionFeedEvent) string {
	value := executionEventTime(event)
	if value.IsZero() {
		return "--:--:--.---"
	}
	return value.Local().Format("15:04:05.000")
}

func executionEventElapsed(event shellruntime.ExecutionFeedEvent) string {
	if event.Execution == nil {
		return ""
	}
	started, err := time.Parse(time.RFC3339Nano, event.Execution.StartedAt)
	if err != nil {
		return ""
	}
	current := executionEventTime(event)
	if current.IsZero() || current.Before(started) {
		return ""
	}
	return current.Sub(started).Round(time.Millisecond).String()
}

func executionEventDuration(event shellruntime.ExecutionFeedEvent) string {
	if event.Execution == nil {
		return ""
	}
	started, err := time.Parse(time.RFC3339Nano, event.Execution.StartedAt)
	if err != nil {
		return ""
	}
	finished := executionEventTime(event)
	if event.Execution.FinishedAt != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, event.Execution.FinishedAt); err == nil {
			finished = parsed
		}
	}
	if finished.IsZero() || finished.Before(started) {
		return ""
	}
	return finished.Sub(started).Round(time.Millisecond).String()
}

func executionEventTime(event shellruntime.ExecutionFeedEvent) time.Time {
	if event.Timestamp != "" {
		if value, err := time.Parse(time.RFC3339Nano, event.Timestamp); err == nil {
			return value
		}
	}
	if event.Execution == nil {
		return time.Time{}
	}
	value := event.Execution.StartedAt
	if event.Type == shellruntime.ExecutionEventCompleted && event.Execution.FinishedAt != "" {
		value = event.Execution.FinishedAt
	}
	parsed, _ := time.Parse(time.RFC3339Nano, value)
	return parsed
}

func executionFollowLabel(paused bool) string {
	if paused {
		return "resume"
	}
	return "pause"
}
