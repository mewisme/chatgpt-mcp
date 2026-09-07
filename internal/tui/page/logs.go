package page

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/runtimecontrol"
	"go.mewis.me/chatgpt-mcp/internal/runtimeevent"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
)

const logsBufferCap = 2000
const logsDefaultTail = 200
const logsReconnectDelay = 2 * time.Second

type LogsCommand string

const (
	LogsRefresh LogsCommand = "logs.refresh"
	LogsFilter  LogsCommand = "logs.filter"
	LogsToggle  LogsCommand = "logs.toggle"
	LogsInfo    LogsCommand = "logs.info"
	LogsClear   LogsCommand = "logs.clear"
)

type LogsCommandMsg struct{ Command LogsCommand }

type logsOverlay uint8

const (
	logsOverlayNone logsOverlay = iota
	logsOverlayForm
	logsOverlayConfirm
	logsOverlayInfo
	logsOverlayOperation
)

type logsBootstrapMsg struct {
	generation uint64
	snapshot   application.LogsSnapshot
	info       application.LogsInfo
	state      runtimecontrol.State
	historyErr error
	infoErr    error
}

type logsStreamOpenMsg struct {
	generation uint64
	stream     *runtimecontrol.EventStream
	state      runtimecontrol.State
	err        error
}

type logsStreamEventMsg struct {
	generation uint64
	event      runtimeevent.Event
	err        error
}

type logsReconnectMsg uint64

type logsClearMsg struct {
	operation uint64
	err       error
}

type LogsPage struct {
	ctx          context.Context
	resourceID   string
	section      string
	execution    bool
	exec         logsExecutionFeed
	cancel       context.CancelFunc
	browser      component.Browser
	detail       component.DetailPage
	events       []runtimeevent.Event
	options      application.LogsQueryOptions
	query        runtimeevent.Query
	visibility   logger.Visibility
	loaded       bool
	loading      bool
	paused       bool
	connected    bool
	reconnecting bool
	stream       *runtimecontrol.EventStream
	streamCtx    context.Context
	streamCancel context.CancelFunc
	streamRunID  string
	streamSeq    uint64
	generation   uint64
	clearSeq     uint64
	overlay      logsOverlay
	form         component.Form
	filterForm   *logsFilterFormData
	confirm      component.ConfirmButtons
	info         application.LogsInfo
	progress     *component.Progress
	notice       string
	err          error
	width        int
	height       int
}

func NewLogs(ctx context.Context) (*LogsPage, error) {
	return NewLogsRoute(ctx, "", "")
}

func NewLogsRoute(ctx context.Context, resourceID, section string) (*LogsPage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	pageCtx, cancel := context.WithCancel(ctx)
	page := &LogsPage{ctx: pageCtx, cancel: cancel, resourceID: strings.TrimSpace(resourceID), section: strings.TrimSpace(section), options: application.LogsQueryOptions{Tail: logsDefaultTail}, visibility: logger.VisibilityVerbose, exec: newLogsExecutionFeed()}
	page.browser = component.NewBrowser(pageCtx, "Logs", nil, nil).WithTitleVisible(false)
	page.syncBrowserHelp()
	return page, nil
}

func NewCommandExecutionLogs(ctx context.Context) (*LogsPage, error) {
	page, err := NewLogsRoute(ctx, "", "")
	if err != nil {
		return nil, err
	}
	page.execution = true
	return page, nil
}

func (page *LogsPage) Init() tea.Cmd {
	if page == nil {
		return nil
	}
	if page.execution {
		return page.startExecutionFeed()
	}
	return page.startBootstrap()
}

func (page *LogsPage) Close() {
	if page == nil {
		return
	}
	page.stopStream()
	page.stopExecutionFeed()
	if page.cancel != nil {
		page.cancel()
	}
}

func (page *LogsPage) OverlayActive() bool {
	return page != nil && page.overlay != logsOverlayNone
}
func (page *LogsPage) InputActive() bool {
	return page != nil && (page.overlay == logsOverlayForm || !page.execution && page.resourceID == "" && page.browser.InputActive())
}

func (page *LogsPage) Notice() string {
	if page == nil {
		return ""
	}
	return page.notice
}

func (page *LogsPage) SetNotice(value string) {
	if page != nil {
		page.notice = strings.TrimSpace(value)
	}
}

func (page *LogsPage) Update(message tea.Msg) (Model, tea.Cmd) {
	if page == nil {
		return page, nil
	}
	switch msg := message.(type) {
	case logsExecutionOpenMsg:
		return page, page.finishExecutionFeedOpen(msg)
	case logsExecutionEventMsg:
		return page, page.finishExecutionFeedEvent(msg)
	case logsExecutionReconnectMsg:
		if uint64(msg) != page.exec.generation || page.exec.connected {
			return page, nil
		}
		return page, page.startExecutionFeed()
	case logsExecutionMouseMsg:
		if page.execution {
			page.handleExecutionMouse(msg)
		}
		return page, nil
	case logsBootstrapMsg:
		return page, page.finishBootstrap(msg)
	case logsStreamOpenMsg:
		return page, page.finishStreamOpen(msg)
	case logsStreamEventMsg:
		return page, page.finishStreamEvent(msg)
	case logsReconnectMsg:
		if uint64(msg) != page.generation || page.connected || page.overlay == logsOverlayOperation {
			return page, nil
		}
		return page, page.startBootstrap()
	case logsClearMsg:
		if msg.operation != page.clearSeq {
			return page, nil
		}
		page.overlay, page.progress = logsOverlayNone, nil
		if msg.err != nil {
			page.err = msg.err
			return page, nil
		}
		page.events = nil
		page.notice, page.err = "Runtime logs cleared", nil
		browserCmd := page.rebuildBrowser("")
		if page.connected {
			return page, browserCmd
		}
		return page, tea.Batch(browserCmd, page.startBootstrap())
	case component.FormSubmittedMsg:
		return page, page.submitFilter()
	case component.FormCancelledMsg:
		page.closeOverlay()
		return page, nil
	case component.FormMouseMsg:
		if page.overlay == logsOverlayForm {
			updated, cmd := page.form.Update(msg)
			page.form = updated
			return page, cmd
		}
		return page, nil
	case component.ConfirmChoiceMsg:
		if page.overlay == logsOverlayConfirm {
			page.confirm.Select(msg.Affirmative)
			return page, page.updateClearConfirm(tea.KeyPressMsg{Code: tea.KeyEnter})
		}
		return page, nil
	case LogsCommandMsg:
		return page, page.openCommand(msg.Command)
	case component.BrowserOpenMsg:
		if !page.execution && page.resourceID == "" && msg.Row.ID != "" {
			return page, func() tea.Msg { return NavigateMsg{Path: []string{"logs", msg.Row.ID}} }
		}
		return page, nil
	case tea.WindowSizeMsg:
		page.width, page.height = msg.Width, msg.Height
		var browserCmd tea.Cmd
		switch {
		case page.execution:
			page.resizeExecutionViewport(msg.Width, max(1, msg.Height-lipgloss.Height(component.PageTitleNotice("Logs · Command Execution", page.notice, msg.Width))-1))
		case page.resourceID != "":
			page.detail.Resize(msg.Width, msg.Height)
		default:
			browserCmd = page.resizeBrowser()
		}
		if page.overlay == logsOverlayForm {
			updated, formCmd := page.form.Update(msg)
			page.form = updated
			return page, tea.Batch(browserCmd, formCmd)
		}
		return page, browserCmd
	case tea.KeyPressMsg:
		if page.overlay == logsOverlayOperation {
			if msg.String() == "esc" {
				page.closeOverlay()
			}
			return page, nil
		}
		if page.overlay == logsOverlayForm {
			updated, cmd := page.form.Update(msg)
			page.form = updated
			return page, cmd
		}
		if page.overlay == logsOverlayConfirm {
			return page, page.updateClearConfirm(msg)
		}
		if page.overlay == logsOverlayInfo {
			if msg.String() == "esc" {
				page.closeOverlay()
			}
			return page, nil
		}
		if page.execution {
			return page, page.handleExecutionKey(msg)
		}
		if page.resourceID != "" {
			updated, cmd := page.detail.Update(msg)
			page.detail = updated
			return page, cmd
		}
		if page.browser.InputActive() {
			updated, cmd := page.browser.Update(msg)
			page.browser = updated.(component.Browser)
			return page, cmd
		}
		if cmd, handled := page.handleKey(msg); handled {
			return page, cmd
		}
	}
	if page.overlay == logsOverlayForm {
		updated, cmd := page.form.Update(message)
		page.form = updated
		return page, cmd
	}
	if page.execution {
		return page, nil
	}
	if page.resourceID != "" {
		updated, cmd := page.detail.Update(message)
		page.detail = updated
		return page, cmd
	}
	before := page.selectedID()
	updated, cmd := page.browser.Update(message)
	page.browser = updated.(component.Browser)
	if !page.paused && before != "" && page.selectedID() != page.tailID() {
		page.paused = true
	}
	return page, cmd
}

func (page *LogsPage) View(width, height int) string {
	if page == nil {
		return component.StateView(component.PageError, "Logs unavailable", "")
	}
	page.width, page.height = width, height
	var content string
	if page.execution {
		title := component.PageTitleNotice("Logs · Command Execution", page.notice, width)
		bodyHeight := max(1, height-lipgloss.Height(title))
		content = title + "\n" + page.executionView(width, bodyHeight)
	} else if page.resourceID != "" {
		page.detail.SetFeedback(page.notice, page.err)
		page.detail.Resize(width, height)
		content = page.detail.View()
	} else {
		title := component.PageTitleNotice("Runtime Logs", page.notice, width)
		status := page.statusView(width)
		feedback := ""
		if page.err != nil {
			feedback = component.Banner(page.err.Error(), component.ToneDanger)
		}
		headerHeight := lipgloss.Height(title) + lipgloss.Height(status)
		browserHeight := max(1, height-headerHeight-pageFeedbackHeight(feedback))
		updated, _ := page.browser.Update(tea.WindowSizeMsg{Width: width, Height: browserHeight})
		page.browser = updated.(component.Browser)
		content = title + "\n" + status + "\n" + prependPageFeedback(feedback, page.browser.Content())
	}
	switch page.overlay {
	case logsOverlayForm:
		content = component.CenterOverlay(content, component.Modal(page.form.View(), overlayWidth(width, 86)), width, height)
	case logsOverlayConfirm:
		body := component.Title("Clear runtime logs?") + "\n\n" + component.Muted("Current and rotated runtime logs will be removed. This cannot be undone.") + "\n\n" + page.confirm.View() + "\n" + component.Muted("Enter confirm · Esc cancel")
		content = component.CenterOverlay(content, component.Modal(body, overlayWidth(width, 68)), width, height)
	case logsOverlayInfo:
		body := component.Title("Logs info") + "\n\n" + detailFields([2]string{"Path", page.info.Path}, [2]string{"Files", fmt.Sprintf("%d", page.info.Files)}, [2]string{"Size", humanBytes(page.info.Bytes)}) + "\n\n" + component.Muted("Esc close")
		content = component.CenterOverlay(content, component.Modal(body, overlayWidth(width, 78)), width, height)
	case logsOverlayOperation:
		body := ""
		if page.progress != nil {
			body = page.progress.View()
		}
		content = component.CenterOverlay(content, component.Modal(body, overlayWidth(width, 62)), width, height)
	}
	return content
}

func (page *LogsPage) MouseTargets(originX, originY, z int) []component.MouseTarget {
	if page == nil {
		return nil
	}
	switch page.overlay {
	case logsOverlayForm:
		return formOverlayMouseTargets(page.form, overlayWidth(page.width, 86), page.width, page.height, originX, originY, z+20)
	case logsOverlayConfirm:
		return confirmOverlayMouseTargets(page.confirm, "Clear runtime logs?", "Current and rotated runtime logs will be removed. This cannot be undone.", overlayWidth(page.width, 68), page.width, page.height, originX, originY, z+20)
	case logsOverlayInfo:
		body := component.Title("Logs info") + "\n\n" + detailFields([2]string{"Path", page.info.Path}, [2]string{"Files", fmt.Sprintf("%d", page.info.Files)}, [2]string{"Size", humanBytes(page.info.Bytes)}) + "\n\n" + component.Muted("Esc close")
		return dismissibleOverlayMouseTargets(body, overlayWidth(page.width, 78), page.width, page.height, originX, originY, z+20)
	case logsOverlayOperation:
		return []component.MouseTarget{mouseBlocker(originX, originY, page.width, page.height, z+20)}
	}
	if page.execution {
		titleHeight := lipgloss.Height(component.PageTitleNotice("Logs · Command Execution", page.notice, page.width))
		bodyY := originY + titleHeight
		return page.executionMouseTargets(originX, bodyY, z, page.width, max(1, page.height-titleHeight))
	}
	if page.resourceID != "" {
		return page.detail.MouseTargets(originX, originY, z)
	}
	feedback := ""
	if page.err != nil {
		feedback = component.Banner(page.err.Error(), component.ToneDanger)
	}
	statusHeight := lipgloss.Height(page.statusView(page.width))
	titleHeight := lipgloss.Height(component.PageTitleNotice("Runtime Logs", page.notice, page.width))
	browserY := originY + titleHeight + statusHeight + pageFeedbackHeight(feedback)
	return page.browser.MouseTargets(originX, browserY, z)
}

func (page *LogsPage) handleKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "space":
		return page.togglePause(), true
	case "f":
		return page.openCommand(LogsFilter), true
	case "r":
		return page.openCommand(LogsRefresh), true
	case "i":
		return page.openCommand(LogsInfo), true
	case "d":
		return page.openCommand(LogsClear), true
	case "x":
		return func() tea.Msg { return NavigateMsg{Path: []string{"logs-exec"}} }, true
	default:
		return nil, false
	}
}

func (page *LogsPage) openCommand(command LogsCommand) tea.Cmd {
	page.err, page.notice = nil, ""
	switch command {
	case LogsRefresh:
		return page.startBootstrap()
	case LogsFilter:
		page.form, page.filterForm = newLogsFilterForm(page.options, page.visibility)
		page.overlay = logsOverlayForm
		return page.form.Init()
	case LogsToggle:
		return page.togglePause()
	case LogsInfo:
		info, err := application.LoadLogsInfo()
		if err != nil {
			page.err = err
			return nil
		}
		page.info, page.overlay = info, logsOverlayInfo
		return nil
	case LogsClear:
		page.confirm = component.NewConfirmButtons("Clear", "Cancel", false)
		page.overlay = logsOverlayConfirm
		return nil
	default:
		page.err = fmt.Errorf("unsupported logs action: %s", command)
		return nil
	}
}

func (page *LogsPage) submitFilter() tea.Cmd {
	options, visibility, err := page.filterForm.Options()
	if err != nil {
		page.err = err
		return nil
	}
	page.options, page.visibility = options, visibility
	page.filterForm = nil
	page.overlay = logsOverlayNone
	page.events = nil
	page.paused = false
	return page.startBootstrap()
}

func (page *LogsPage) updateClearConfirm(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		page.closeOverlay()
		return nil
	case "enter":
		if !page.confirm.AffirmativeSelected() {
			page.closeOverlay()
			return nil
		}
		page.clearSeq++
		operation := page.clearSeq
		progress := component.NewProgress("Clearing runtime logs")
		page.progress, page.overlay = &progress, logsOverlayOperation
		return func() tea.Msg { return logsClearMsg{operation: operation, err: application.ClearLogs(page.ctx)} }
	default:
		return page.confirm.Update(msg)
	}
}

func (page *LogsPage) togglePause() tea.Cmd {
	page.paused = !page.paused
	page.notice = ""
	if !page.paused {
		page.browser.SelectLast()
	}
	page.syncBrowserHelp()
	return nil
}

func (page *LogsPage) startBootstrap() tea.Cmd {
	page.stopStream()
	page.generation++
	generation := page.generation
	ctx, cancel := context.WithCancel(page.ctx)
	page.streamCtx, page.streamCancel = ctx, cancel
	page.streamRunID, page.streamSeq = "", 0
	page.loading, page.reconnecting = true, page.loaded
	options, visibility := page.options, page.visibility
	return func() tea.Msg {
		state, _ := runtimecontrol.Load()
		historyOptions := options
		if historyOptions.Session == "" && !historyOptions.All && state.RunID != "" {
			historyOptions.Session = state.RunID
		}
		snapshot, historyErr := application.LoadLogs(historyOptions, visibility, logsBufferCap, time.Now())
		info, infoErr := application.LoadLogsInfo()
		return logsBootstrapMsg{generation: generation, snapshot: snapshot, info: info, state: state, historyErr: historyErr, infoErr: infoErr}
	}
}

func (page *LogsPage) finishBootstrap(msg logsBootstrapMsg) tea.Cmd {
	if msg.generation != page.generation {
		return nil
	}
	page.loading, page.loaded = false, true
	var browserCmd tea.Cmd
	if msg.historyErr != nil {
		page.err = msg.historyErr
	} else {
		page.query = msg.snapshot.Query
		if msg.state.RunID != "" {
			page.streamRunID = msg.state.RunID
			page.streamSeq = msg.snapshot.LatestSequence[msg.state.RunID]
		}
		browserCmd = page.mergeEvents(msg.snapshot.Events)
		page.err = nil
	}
	if msg.infoErr == nil {
		page.info = msg.info
	}
	page.reconnecting = !page.connected
	if page.err == nil && page.reconnecting {
		page.notice = "Journal loaded; connecting live stream"
	}
	return tea.Batch(browserCmd, page.openStreamCmd(msg.generation))
}

func (page *LogsPage) openStreamCmd(generation uint64) tea.Cmd {
	ctx := page.streamCtx
	if ctx == nil {
		ctx = page.ctx
	}
	return func() tea.Msg {
		stream, state, err := runtimecontrol.OpenEvents(ctx)
		return logsStreamOpenMsg{generation: generation, stream: stream, state: state, err: err}
	}
}

func (page *LogsPage) finishStreamOpen(msg logsStreamOpenMsg) tea.Cmd {
	if msg.generation != page.generation {
		if msg.stream != nil {
			_ = msg.stream.Close()
		}
		return nil
	}
	if msg.err != nil {
		page.connected, page.reconnecting = false, true
		page.stream = nil
		if page.err == nil {
			page.notice = "Runtime offline; showing journal history and retrying live stream"
		}
		return page.reconnectCmd(msg.generation)
	}
	page.stream, page.connected, page.reconnecting = msg.stream, true, false
	if page.streamRunID != msg.state.RunID {
		page.streamRunID, page.streamSeq = msg.state.RunID, 0
	}
	if msg.stream.LatestSequence() > page.streamSeq {
		page.notice = "Live stream advanced during journal load; resyncing journal"
		return page.startBootstrap()
	}
	if page.options.Session == "" && !page.options.All && msg.state.RunID != "" && page.query.RunID != msg.state.RunID {
		page.notice = "Runtime session changed; resyncing journal"
		return page.startBootstrap()
	}
	page.notice = ""
	return page.nextEventCmd(msg.generation)
}

func (page *LogsPage) nextEventCmd(generation uint64) tea.Cmd {
	stream := page.stream
	if stream == nil {
		return nil
	}
	return func() tea.Msg {
		event, err := stream.Next()
		return logsStreamEventMsg{generation: generation, event: event, err: err}
	}
}

func (page *LogsPage) finishStreamEvent(msg logsStreamEventMsg) tea.Cmd {
	if msg.generation != page.generation {
		return nil
	}
	if msg.err != nil {
		if msg.err == io.EOF || page.ctx.Err() == nil {
			page.connected, page.reconnecting = false, true
			page.stopStreamOnly()
			page.notice = "Live stream disconnected; reconnecting"
			return page.reconnectCmd(msg.generation)
		}
		return nil
	}
	if page.streamGap(msg.event) {
		page.notice = "Live stream gap detected; resyncing journal"
		return page.startBootstrap()
	}
	if page.options.Session == "" && !page.options.All && msg.event.RunID != "" && page.query.RunID != msg.event.RunID {
		page.query.RunID = msg.event.RunID
		page.events = nil
		page.paused = false
	}
	if page.query.Match(msg.event) && msg.event.Visibility <= page.visibility {
		return tea.Batch(page.appendEvent(msg.event), page.nextEventCmd(msg.generation))
	}
	return page.nextEventCmd(msg.generation)
}

func (page *LogsPage) streamGap(event runtimeevent.Event) bool {
	if event.RunID == "" || event.Sequence == 0 {
		return false
	}
	if page.streamRunID != event.RunID {
		page.streamRunID, page.streamSeq = event.RunID, event.Sequence
		return false
	}
	gap := page.streamSeq > 0 && event.Sequence > page.streamSeq+1
	if event.Sequence > page.streamSeq {
		page.streamSeq = event.Sequence
	}
	return gap
}

func (page *LogsPage) reconnectCmd(generation uint64) tea.Cmd {
	return tea.Tick(logsReconnectDelay, func(time.Time) tea.Msg { return logsReconnectMsg(generation) })
}

func (page *LogsPage) mergeEvents(events []runtimeevent.Event) tea.Cmd {
	selected := page.selectedID()
	merged := append(append([]runtimeevent.Event(nil), page.events...), events...)
	seen := map[string]runtimeevent.Event{}
	for _, event := range merged {
		seen[logEventID(event)] = event
	}
	merged = merged[:0]
	for _, event := range seen {
		merged = append(merged, event)
	}
	sort.SliceStable(merged, func(i, j int) bool {
		if merged[i].Time.Equal(merged[j].Time) {
			if merged[i].RunID == merged[j].RunID {
				return merged[i].Sequence < merged[j].Sequence
			}
			return merged[i].RunID < merged[j].RunID
		}
		return merged[i].Time.Before(merged[j].Time)
	})
	if len(merged) > logsBufferCap {
		merged = append([]runtimeevent.Event(nil), merged[len(merged)-logsBufferCap:]...)
	}
	page.events = merged
	if !page.paused {
		selected = page.tailID()
	}
	return page.rebuildBrowser(selected)
}

func (page *LogsPage) appendEvent(event runtimeevent.Event) tea.Cmd {
	id := logEventID(event)
	for _, current := range page.events {
		if logEventID(current) == id {
			return nil
		}
	}
	selected, tail := page.selectedID(), page.tailID()
	if !page.paused && selected != "" && tail != "" && selected != tail {
		page.paused = true
	}
	page.events = append(page.events, event)
	if len(page.events) > logsBufferCap {
		page.events = append([]runtimeevent.Event(nil), page.events[len(page.events)-logsBufferCap:]...)
	}
	if !page.paused {
		selected = page.tailID()
	}
	return page.rebuildBrowser(selected)
}

func (page *LogsPage) rebuildBrowser(selected string) tea.Cmd {
	if page.resourceID != "" {
		page.syncDetail()
		return nil
	}
	rows := make([]component.Row, 0, len(page.events))
	for _, event := range page.events {
		rows = append(rows, page.logRow(event))
	}
	cmd := page.browser.ReplaceRows(rows, selected)
	if !page.paused && selected == "" {
		page.browser.SelectLast()
	}
	if page.width > 0 && page.height > 0 {
		cmd = tea.Batch(cmd, page.resizeBrowser())
	}
	return cmd
}

func (page *LogsPage) resizeBrowser() tea.Cmd {
	titleHeight := lipgloss.Height(component.PageTitleNotice("Runtime Logs", page.notice, page.width))
	statusHeight := lipgloss.Height(page.statusView(page.width))
	feedback := ""
	if page.err != nil {
		feedback = component.Banner(page.err.Error(), component.ToneDanger)
	}
	height := max(1, page.height-titleHeight-statusHeight-pageFeedbackHeight(feedback))
	updated, cmd := page.browser.Update(tea.WindowSizeMsg{Width: page.width, Height: height})
	page.browser = updated.(component.Browser)
	return cmd
}

func (page *LogsPage) logRow(event runtimeevent.Event) component.Row {
	level := strings.ToUpper(strings.TrimSpace(event.Level))
	componentName := strings.ToUpper(strings.TrimSpace(event.Component))
	titleParts := []string{event.Time.Local().Format("15:04:05")}
	if level != "" {
		titleParts = append(titleParts, level)
	}
	if componentName != "" {
		titleParts = append(titleParts, componentName)
	}
	if event.Name != "" {
		titleParts = append(titleParts, event.Name)
	}
	meta := compactParts(event.WorkspaceID, event.Tool, event.Status)
	fields := application.LogFields(event, page.visibility)
	search := []string{event.RunID, event.Level, event.Kind, event.Name, event.Component, event.Message, event.Error, event.WorkspaceID, event.Tool, event.Method, event.Source, event.Status, event.ServiceID, event.ServiceScope}
	for _, field := range fields {
		value := fmt.Sprint(field.Value)
		search = append(search, field.Key, value)
	}
	return component.Row{ID: logEventID(event), Title: strings.Join(titleParts, "  "), Description: event.Message, Meta: meta, Search: strings.Join(search, " ")}
}

func (page *LogsPage) syncDetail() {
	var event runtimeevent.Event
	found := false
	for _, current := range page.events {
		if logEventID(current) == page.resourceID {
			event, found = current, true
			break
		}
	}
	if !found {
		page.detail = component.NewDetailPage("Log event · "+page.resourceID, "unavailable", component.Muted("Log event not found in the current journal view."))
		page.detail.SetBindings(component.DetailPageBinding{Key: "r", Desc: "refresh", Message: LogsCommandMsg{Command: LogsRefresh}})
		if page.loaded {
			page.err = fmt.Errorf("log event not found: %s", page.resourceID)
		}
		return
	}
	page.err = nil
	overview := detailFields(
		[2]string{"Time", event.Time.Local().Format(time.RFC3339Nano)}, [2]string{"Run", event.RunID}, [2]string{"Sequence", fmt.Sprintf("%d", event.Sequence)},
		[2]string{"PID", fmt.Sprintf("%d", event.PID)}, [2]string{"Level", event.Level}, [2]string{"Kind", event.Kind}, [2]string{"Component", event.Component}, [2]string{"Event", event.Name},
		[2]string{"Workspace", event.WorkspaceID}, [2]string{"Tool", event.Tool}, [2]string{"Method", event.Method}, [2]string{"Source", event.Source}, [2]string{"Status", event.Status}, [2]string{"Duration", durationLabel(event.DurationMS)},
		[2]string{"Message", event.Message}, [2]string{"Error", event.Error}, [2]string{"Service", compactParts(event.ServiceID, event.ServiceScope)},
	)
	fields := application.LogFields(event, page.visibility)
	fieldLines := make([]string, 0, len(fields))
	for _, field := range fields {
		fieldLines = append(fieldLines, component.KeyValue(field.Key, fmt.Sprint(field.Value)))
	}
	if len(fieldLines) == 0 {
		fieldLines = append(fieldLines, component.Muted("No visible structured fields"))
	}
	content := overview
	if page.section == "fields" {
		content = strings.Join(fieldLines, "\n")
	}
	meta := compactParts(event.Level, event.Component, event.RunID, fmt.Sprintf("seq %d", event.Sequence))
	page.detail = component.NewDetailPage("Log event · "+event.Name, meta, content)
	bindings := []component.DetailPageBinding{{Key: "r", Desc: "refresh", Message: LogsCommandMsg{Command: LogsRefresh}}}
	if page.section == "" {
		bindings = append([]component.DetailPageBinding{{Key: "f", Desc: "fields", Message: NavigateMsg{Path: []string{"logs", page.resourceID, "fields"}}}}, bindings...)
	}
	page.detail.SetBindings(bindings...)
	if page.width > 0 && page.height > 0 {
		page.detail.Resize(page.width, page.height)
	}
}

func (page *LogsPage) statusView(width int) string {
	stream := component.ToneText("● LIVE", component.ToneSuccess)
	if page.loading && !page.loaded {
		stream = component.Muted("↻ LOADING")
	} else if page.reconnecting {
		stream = component.ToneText("↻ RECONNECTING", component.ToneWarning)
	} else if !page.connected {
		stream = component.Muted("○ OFFLINE")
	}
	follow := component.ToneText("● ON", component.ToneSuccess)
	if page.paused {
		follow = component.ToneText("○ PAUSED", component.ToneWarning)
	}
	session := page.query.RunID
	if session == "" {
		session = "all / none yet"
	}
	left := component.KeyValue("Stream", stream) + "   " + component.KeyValue("Follow", follow) + "   " + component.KeyValue("View", logsVisibilityValue(page.visibility)) + "   " + component.KeyValue("Events", fmt.Sprintf("%d / %d", len(page.events), logsBufferCap))
	right := component.KeyValue("Session", session)
	return component.TwoColumn(left, right, width)
}

func (page *LogsPage) syncBrowserHelp() {
	toggle := "pause"
	if page.paused {
		toggle = "resume"
	}
	page.browser.SetHelpBindings(
		component.Binding([]string{"space"}, "space", toggle), component.Binding([]string{"f"}, "f", "filters"), component.Binding([]string{"r"}, "r", "refresh"),
		component.Binding([]string{"i"}, "i", "info"), component.Binding([]string{"d"}, "d", "clear"), component.Binding([]string{"x"}, "x", "executions"),
	)
}

func (page *LogsPage) selectedID() string {
	row, ok := page.browser.Selected()
	if !ok {
		return ""
	}
	return row.ID
}
func (page *LogsPage) tailID() string {
	if len(page.events) == 0 {
		return ""
	}
	return logEventID(page.events[len(page.events)-1])
}
func (page *LogsPage) stopStream() {
	page.stopStreamOnly()
	if page.streamCancel != nil {
		page.streamCancel()
		page.streamCancel = nil
	}
	page.streamCtx = nil
}
func (page *LogsPage) stopStreamOnly() {
	if page.stream != nil {
		_ = page.stream.Close()
		page.stream = nil
	}
	page.connected = false
}
func (page *LogsPage) closeOverlay() {
	page.overlay = logsOverlayNone
	page.form = component.Form{}
	page.filterForm = nil
	page.confirm = component.ConfirmButtons{}
	page.progress = nil
}

func logEventID(event runtimeevent.Event) string {
	if event.RunID != "" && event.Sequence > 0 {
		return fmt.Sprintf("%s:%d", event.RunID, event.Sequence)
	}
	return fmt.Sprintf("%d:%s:%s:%s", event.Time.UnixNano(), event.RunID, event.Name, event.Message)
}
func compactParts(values ...string) string {
	result := []string{}
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return strings.Join(result, " · ")
}
func durationLabel(ms int64) string {
	if ms <= 0 {
		return ""
	}
	return fmt.Sprintf("%dms", ms)
}
func shortValue(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if lipgloss.Width(value) <= limit {
		return value
	}
	if limit == 1 {
		return ansi.Truncate(value, 1, "")
	}
	return ansi.Truncate(value, limit-1, "") + "…"
}
func humanBytes(value int64) string {
	if value < 1024 {
		return fmt.Sprintf("%d B", value)
	}
	if value < 1024*1024 {
		return fmt.Sprintf("%.1f KiB", float64(value)/1024)
	}
	return fmt.Sprintf("%.1f MiB", float64(value)/(1024*1024))
}
