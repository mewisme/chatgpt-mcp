package page

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"

	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/runtimecontrol"
	shellruntime "go.mewis.me/chatgpt-mcp/internal/shell"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

type executionScopeMode string

const (
	executionScopeCombined  executionScopeMode = "combined"
	executionScopeWorkspace executionScopeMode = "workspace"
	executionScopeContainer executionScopeMode = "container"
)

type executionWorkspaceView string

const (
	executionWorkspaceCommands executionWorkspaceView = "commands"
	executionWorkspaceProcess  executionWorkspaceView = "process"
)

type executionSetting string

const (
	executionSettingMode   executionSetting = "mode"
	executionSettingBuffer executionSetting = "buffer"
)

type executionScopeFormData struct {
	Setting       string
	Mode          string
	WorkspaceID   string
	ContainerID   string
	View          string
	ProcessID     string
	Processes     map[string][]shellruntime.ProcessInfo
	BufferPreset  string
	BufferCustom  string
	BufferCurrent int
}

const executionFeedCustomPreset = "custom"

var executionFeedSizePresets = []struct {
	Label string
	Bytes int
}{
	{Label: "1 MB", Bytes: 1_000_000},
	{Label: "2 MB", Bytes: 2_000_000},
	{Label: "5 MB", Bytes: 5_000_000},
	{Label: "10 MB (default)", Bytes: 10_000_000},
	{Label: "25 MB", Bytes: 25_000_000},
	{Label: "50 MB", Bytes: 50_000_000},
}

type logsExecutionConfigMsg struct {
	operation uint64
	bytes     int
	err       error
}

func newExecutionScopeEditor(ctx context.Context, feed logsExecutionFeed) (component.Editor, *executionScopeFormData, error) {
	cfg, err := config.Load()
	if err != nil {
		return component.Editor{}, nil, err
	}
	manager := workspace.NewManager(workspace.DefaultStorePath())
	workspaces, err := manager.List()
	if err != nil {
		return component.Editor{}, nil, err
	}
	containers, err := manager.ListContainers()
	if err != nil {
		return component.Editor{}, nil, err
	}
	bufferBytes := cfg.Shell.ExecutionFeedMaxBytes
	if bufferBytes <= 0 {
		bufferBytes = config.DefaultExecutionFeedMaxBytes
	}
	bufferPreset := executionFeedCustomPreset
	for _, preset := range executionFeedSizePresets {
		if preset.Bytes == bufferBytes {
			bufferPreset = strconv.Itoa(preset.Bytes)
			break
		}
	}
	data := &executionScopeFormData{Setting: string(executionSettingMode), Mode: string(normalizeExecutionScopeMode(feed.scopeMode)), WorkspaceID: feed.workspaceID, ContainerID: feed.containerID, View: string(normalizeExecutionWorkspaceView(feed.workspaceView)), ProcessID: feed.processID, Processes: map[string][]shellruntime.ProcessInfo{}, BufferPreset: bufferPreset, BufferCustom: strconv.Itoa(bufferBytes), BufferCurrent: bufferBytes}
	workspaceOptions := make([]huh.Option[string], 0, len(workspaces))
	for _, item := range workspaces {
		workspaceOptions = append(workspaceOptions, huh.NewOption(item.Path+" · "+item.ID, item.ID))
		if values, listErr := runtimecontrol.ListProcesses(ctx, item.ID); listErr == nil {
			data.Processes[item.ID] = values
		}
	}
	if len(workspaceOptions) == 0 {
		workspaceOptions = append(workspaceOptions, huh.NewOption("No registered workspaces", ""))
	}
	containerOptions := make([]huh.Option[string], 0, len(containers))
	for _, item := range containers {
		containerOptions = append(containerOptions, huh.NewOption(fmt.Sprintf("%s · %s · %d workspaces", item.Name, item.ID, len(item.WorkspaceIDs)), item.ID))
	}
	if len(containerOptions) == 0 {
		containerOptions = append(containerOptions, huh.NewOption("No workspace containers", ""))
	}
	bufferOptions := make([]huh.Option[string], 0, len(executionFeedSizePresets)+1)
	for _, preset := range executionFeedSizePresets {
		bufferOptions = append(bufferOptions, huh.NewOption(preset.Label, strconv.Itoa(preset.Bytes)))
	}
	bufferOptions = append(bufferOptions, huh.NewOption("Custom", executionFeedCustomPreset))
	form := component.NewEditorForm(
		component.Group(component.Select("Setting", &data.Setting,
			huh.NewOption("Mode", string(executionSettingMode)),
			huh.NewOption("Buffer", string(executionSettingBuffer)),
		)),
		component.Group(component.Select("Mode", &data.Mode,
			huh.NewOption("Combined", string(executionScopeCombined)),
			huh.NewOption("Workspace", string(executionScopeWorkspace)),
			huh.NewOption("Container", string(executionScopeContainer)),
		)).WithHideFunc(func() bool { return data.Setting != string(executionSettingMode) }),
		component.Group(component.Select("Workspace", &data.WorkspaceID, workspaceOptions...)).WithHideFunc(func() bool {
			return data.Setting != string(executionSettingMode) || data.Mode != string(executionScopeWorkspace)
		}),
		component.Group(component.Select("View", &data.View,
			huh.NewOption("Run commands", string(executionWorkspaceCommands)),
			huh.NewOption("View process", string(executionWorkspaceProcess)),
		)).WithHideFunc(func() bool {
			return data.Setting != string(executionSettingMode) || data.Mode != string(executionScopeWorkspace)
		}),
		component.Group(huh.NewSelect[string]().Title("Process").Value(&data.ProcessID).OptionsFunc(func() []huh.Option[string] {
			return executionProcessOptions(data.Processes[data.WorkspaceID])
		}, &data.WorkspaceID)).WithHideFunc(func() bool {
			return data.Setting != string(executionSettingMode) || data.Mode != string(executionScopeWorkspace) || data.View != string(executionWorkspaceProcess)
		}),
		component.Group(component.Select("Container", &data.ContainerID, containerOptions...)).WithHideFunc(func() bool {
			return data.Setting != string(executionSettingMode) || data.Mode != string(executionScopeContainer)
		}),
		component.Group(component.Select("Event buffer", &data.BufferPreset, bufferOptions...)).WithHideFunc(func() bool { return data.Setting != string(executionSettingBuffer) }),
		component.Group(component.Input("Custom event buffer", &data.BufferCustom).Description("64 KB-100 MB; examples: 512 KB, 5 MB, 1.5 MiB").Validate(func(value string) error {
			_, err := parseExecutionFeedSize(value)
			return err
		})).WithHideFunc(func() bool {
			return data.Setting != string(executionSettingBuffer) || data.BufferPreset != executionFeedCustomPreset
		}),
	)
	editor := component.NewEditor("apply", component.EditorSection{ID: "scope", Title: "Command Execution", Description: "Choose whether to configure the output mode or retained event buffer.", Form: form}).WithSubmitMode(component.EditorSubmitOnComplete)
	return editor, data, nil
}

func parseExecutionFeedSize(raw string) (int, error) {
	value := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(raw), " ", ""))
	if value == "" {
		return 0, fmt.Errorf("event buffer is required")
	}
	multiplier := float64(1)
	number := value
	matched := false
	for _, unit := range []struct {
		Suffix     string
		Multiplier float64
	}{
		{Suffix: "MIB", Multiplier: 1024 * 1024},
		{Suffix: "KIB", Multiplier: 1024},
		{Suffix: "MB", Multiplier: 1_000_000},
		{Suffix: "KB", Multiplier: 1_000},
		{Suffix: "B", Multiplier: 1},
	} {
		if strings.HasSuffix(value, unit.Suffix) {
			number = strings.TrimSuffix(value, unit.Suffix)
			multiplier = unit.Multiplier
			matched = true
			break
		}
	}
	if !matched && strings.IndexFunc(value, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return 0, fmt.Errorf("use bytes or B, KB, MB, KiB, MiB")
	}
	parsed, err := strconv.ParseFloat(number, 64)
	if err != nil || math.IsNaN(parsed) || math.IsInf(parsed, 0) || parsed <= 0 {
		return 0, fmt.Errorf("invalid event buffer size")
	}
	bytesValue := parsed * multiplier
	if math.IsInf(bytesValue, 0) || bytesValue < float64(config.MinExecutionFeedMaxBytes) || bytesValue > float64(config.MaxExecutionFeedMaxBytes) {
		return 0, fmt.Errorf("event buffer must be between 64 KB and 100 MB")
	}
	result := int(math.Round(bytesValue))
	if err := config.ValidateExecutionFeedMaxBytes(result); err != nil {
		return 0, fmt.Errorf("event buffer must be between 64 KB and 100 MB")
	}
	return result, nil
}

func (data *executionScopeFormData) executionFeedMaxBytes() (int, error) {
	if data == nil {
		return 0, fmt.Errorf("event buffer is unavailable")
	}
	if data.BufferPreset == executionFeedCustomPreset {
		return parseExecutionFeedSize(data.BufferCustom)
	}
	value, err := strconv.Atoi(data.BufferPreset)
	if err != nil {
		return 0, fmt.Errorf("invalid event buffer preset")
	}
	if err := config.ValidateExecutionFeedMaxBytes(value); err != nil {
		return 0, fmt.Errorf("invalid event buffer preset")
	}
	return value, nil
}

func normalizeExecutionWorkspaceView(view executionWorkspaceView) executionWorkspaceView {
	if view == executionWorkspaceProcess {
		return view
	}
	return executionWorkspaceCommands
}

func executionProcessOptions(processes []shellruntime.ProcessInfo) []huh.Option[string] {
	options := make([]huh.Option[string], 0, len(processes))
	for i := len(processes) - 1; i >= 0; i-- {
		item := processes[i]
		status := "running"
		if !item.Running {
			status = "exited"
			if item.ExitCode != nil {
				status = fmt.Sprintf("exited %d", *item.ExitCode)
			}
		}
		command := strings.TrimSpace(item.Command)
		if len(command) > 56 {
			command = command[:53] + "..."
		}
		options = append(options, huh.NewOption(fmt.Sprintf("%s · pid %d · %s", status, item.PID, command), item.ID))
	}
	if len(options) == 0 {
		return []huh.Option[string]{huh.NewOption("No managed processes", "")}
	}
	return options
}

func normalizeExecutionScopeMode(mode executionScopeMode) executionScopeMode {
	switch mode {
	case executionScopeWorkspace, executionScopeContainer:
		return mode
	default:
		return executionScopeCombined
	}
}

func (page *LogsPage) initExecutionScopeEditor() error {
	if page == nil {
		return fmt.Errorf("command execution settings are unavailable")
	}
	editor, data, err := newExecutionScopeEditor(page.ctx, page.exec)
	if err != nil {
		return err
	}
	page.exec.scopeEditor, page.exec.scopeForm = &editor, data
	page.resizeExecutionScopeEditor()
	return nil
}

func (page *LogsPage) openExecutionScopeEditor() tea.Cmd {
	if page == nil || page.exec.scopeEditor != nil {
		return nil
	}
	if err := page.initExecutionScopeEditor(); err != nil {
		page.exec.err = err
		return nil
	}
	page.action = "settings"
	return tea.Batch(page.exec.scopeEditor.Init(), func() tea.Msg {
		return NavigateMsg{Path: []string{"logs-exec", "settings"}, Replace: true, PreservePage: true}
	})
}

func (page *LogsPage) leaveExecutionScopeEditor(commands ...tea.Cmd) tea.Cmd {
	if page == nil {
		return nil
	}
	page.exec.scopeEditor, page.exec.scopeForm, page.action = nil, nil, ""
	commands = append(commands, tea.ClearScreen, func() tea.Msg { return NavigateMsg{Path: []string{"logs-exec"}, Replace: true, PreservePage: true} })
	return tea.Batch(commands...)
}

func (page *LogsPage) closeExecutionScopeEditor() tea.Cmd { return page.leaveExecutionScopeEditor() }

func (page *LogsPage) submitExecutionScopeEditor() tea.Cmd {
	if page == nil || page.exec.scopeEditor == nil || page.exec.scopeForm == nil {
		return nil
	}
	setting := executionSetting(strings.TrimSpace(page.exec.scopeForm.Setting))
	if setting != executionSettingMode && setting != executionSettingBuffer {
		page.exec.scopeEditor.SetFeedback("", fmt.Errorf("setting must be mode or buffer"))
		return nil
	}
	if setting == executionSettingBuffer {
		bufferBytes, err := page.exec.scopeForm.executionFeedMaxBytes()
		if err != nil {
			page.exec.scopeEditor.SetFeedback("", err)
			return nil
		}
		if bufferBytes != page.exec.scopeForm.BufferCurrent {
			page.exec.configMutationSeq++
			operation := page.exec.configMutationSeq
			ctx := page.ctx
			page.exec.scopeEditor.SetSubmitting(true)
			return func() tea.Msg {
				_, err := application.SetConfigField(ctx, "shell.execution_feed_max_bytes", strconv.Itoa(bufferBytes))
				return logsExecutionConfigMsg{operation: operation, bytes: bufferBytes, err: err}
			}
		}
		page.exec.err = nil
		page.refreshExecutionViewport()
		return page.leaveExecutionScopeEditor()
	}
	mode := normalizeExecutionScopeMode(executionScopeMode(strings.TrimSpace(page.exec.scopeForm.Mode)))
	manager := workspace.NewManager(workspace.DefaultStorePath())
	workspaceID, containerID := strings.TrimSpace(page.exec.scopeForm.WorkspaceID), strings.TrimSpace(page.exec.scopeForm.ContainerID)
	workspaceView := normalizeExecutionWorkspaceView(executionWorkspaceView(strings.TrimSpace(page.exec.scopeForm.View)))
	processID := strings.TrimSpace(page.exec.scopeForm.ProcessID)
	processExecutionID := ""
	processRunning := false
	oldWorkspaceID, oldProcessID, oldProcessExecutionID, oldProcessRunning := page.exec.workspaceID, page.exec.processID, page.exec.processExecutionID, page.exec.processRunning
	members := map[string]struct{}{}
	containerName := ""
	switch mode {
	case executionScopeWorkspace:
		if workspaceID == "" {
			page.exec.scopeEditor.SetFeedback("", fmt.Errorf("workspace is required"))
			return nil
		}
		item, err := manager.Get(workspaceID)
		if err != nil {
			page.exec.scopeEditor.SetFeedback("", err)
			return nil
		}
		workspaceID = item.ID
		if workspaceView == executionWorkspaceProcess {
			if processID == "" {
				page.exec.scopeEditor.SetFeedback("", fmt.Errorf("process is required"))
				return nil
			}
			found := false
			for _, process := range page.exec.scopeForm.Processes[workspaceID] {
				if process.ID == processID {
					found = true
					processExecutionID = process.ExecutionID
					processRunning = process.Running
					break
				}
			}
			if !found {
				page.exec.scopeEditor.SetFeedback("", fmt.Errorf("selected process is unavailable"))
				return nil
			}
		}
	case executionScopeContainer:
		if containerID == "" {
			page.exec.scopeEditor.SetFeedback("", fmt.Errorf("container is required"))
			return nil
		}
		context, err := manager.ResolveContainer(containerID)
		if err != nil {
			page.exec.scopeEditor.SetFeedback("", err)
			return nil
		}
		containerID, containerName = context.Container.ID, context.Container.Name
		for _, item := range context.Workspaces {
			members[item.ID] = struct{}{}
		}
	}
	page.exec.scopeMode, page.exec.workspaceID, page.exec.containerID = mode, workspaceID, containerID
	page.exec.workspaceView, page.exec.processID = workspaceView, processID
	page.exec.processExecutionID, page.exec.processRunning = processExecutionID, processRunning
	if mode != executionScopeWorkspace || workspaceView != executionWorkspaceProcess {
		page.exec.processID = ""
		page.exec.processExecutionID = ""
		page.exec.processRunning = false
	}
	page.exec.containerName, page.exec.containerMembers, page.exec.scopeStale, page.exec.scopeNotice = containerName, members, false, ""
	page.exec.err = nil
	page.refreshExecutionViewport()
	if oldProcessID != "" && (oldWorkspaceID != page.exec.workspaceID || oldProcessID != page.exec.processID || page.exec.workspaceView != executionWorkspaceProcess) {
		return page.leaveExecutionScopeEditor(cleanupFinishedProcessCmd(page.ctx, oldWorkspaceID, oldProcessID, oldProcessExecutionID, oldProcessRunning))
	}
	return page.leaveExecutionScopeEditor()
}

func (page *LogsPage) finishExecutionConfig(msg logsExecutionConfigMsg) tea.Cmd {
	if page == nil || msg.operation != page.exec.configMutationSeq {
		return nil
	}
	if msg.err != nil {
		if page.exec.scopeEditor != nil {
			page.exec.scopeEditor.SetSubmitting(false)
			page.exec.scopeEditor.SetFeedback("", msg.err)
		} else {
			page.exec.err = msg.err
		}
		return nil
	}
	page.exec.feedMaxBytes = msg.bytes
	page.exec.events, page.exec.feedBytes = trimExecutionFeed(page.exec.events, msg.bytes)
	page.exec.notice = "Command execution event buffer updated · " + executionBytesLabel(msg.bytes)
	if page.exec.scopeEditor == nil || page.exec.scopeForm == nil {
		page.refreshExecutionViewport()
		return nil
	}
	page.exec.scopeEditor.SetSubmitting(false)
	page.exec.scopeForm.BufferCurrent = msg.bytes
	cmd := page.submitExecutionScopeEditor()
	return cmd
}

var deleteFinishedProcess = runtimecontrol.DeleteFinishedProcess

func cleanupFinishedProcessCmd(ctx context.Context, workspaceID, processID, executionID string, running bool) tea.Cmd {
	if strings.TrimSpace(workspaceID) == "" || strings.TrimSpace(processID) == "" || strings.TrimSpace(executionID) == "" || running {
		return nil
	}
	return func() tea.Msg {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer cancel()
		_ = deleteFinishedProcess(cleanupCtx, workspaceID, processID)
		return nil
	}
}

func (page *LogsPage) detachSelectedProcessCmd() tea.Cmd {
	if page == nil || page.exec.workspaceView != executionWorkspaceProcess {
		return nil
	}
	cmd := cleanupFinishedProcessCmd(page.ctx, page.exec.workspaceID, page.exec.processID, page.exec.processExecutionID, page.exec.processRunning)
	page.exec.workspaceView, page.exec.processID, page.exec.processExecutionID, page.exec.processRunning = executionWorkspaceCommands, "", "", false
	page.refreshExecutionViewport()
	return cmd
}

func (page *LogsPage) cleanupSelectedFinishedProcess() {
	if page == nil || page.exec.workspaceView != executionWorkspaceProcess || page.exec.processRunning || page.exec.workspaceID == "" || page.exec.processID == "" || page.exec.processExecutionID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = deleteFinishedProcess(ctx, page.exec.workspaceID, page.exec.processID)
}

func (page *LogsPage) refreshExecutionScope() {
	if page == nil {
		return
	}
	mode := normalizeExecutionScopeMode(page.exec.scopeMode)
	page.exec.scopeMode = mode
	page.exec.scopeStale, page.exec.scopeNotice = false, ""
	if mode == executionScopeCombined {
		return
	}
	manager := workspace.NewManager(workspace.DefaultStorePath())
	if mode == executionScopeWorkspace {
		item, err := manager.Get(page.exec.workspaceID)
		if err != nil {
			page.exec.scopeStale = true
			page.exec.scopeNotice = "Selected workspace unavailable: " + page.exec.workspaceID
			return
		}
		page.exec.workspaceID = item.ID
		return
	}
	context, err := manager.ResolveContainer(page.exec.containerID)
	if err != nil {
		page.exec.scopeStale = true
		page.exec.scopeNotice = "Selected workspace container unavailable: " + page.exec.containerID
		page.exec.containerMembers = map[string]struct{}{}
		return
	}
	page.exec.containerID, page.exec.containerName = context.Container.ID, context.Container.Name
	page.exec.containerMembers = make(map[string]struct{}, len(context.Workspaces))
	for _, item := range context.Workspaces {
		page.exec.containerMembers[item.ID] = struct{}{}
	}
}

func (page *LogsPage) visibleExecutionEvents() []shellruntime.ExecutionFeedEvent {
	if page == nil || len(page.exec.events) == 0 {
		return nil
	}
	if page.exec.scopeStale {
		return nil
	}
	result := make([]shellruntime.ExecutionFeedEvent, 0, len(page.exec.events))
	for _, event := range page.exec.events {
		if page.executionEventVisible(event) {
			result = append(result, event)
		}
	}
	return result
}

func (page *LogsPage) executionEventVisible(event shellruntime.ExecutionFeedEvent) bool {
	if page == nil || page.exec.scopeStale {
		return false
	}
	switch normalizeExecutionScopeMode(page.exec.scopeMode) {
	case executionScopeWorkspace:
		if event.WorkspaceID != page.exec.workspaceID {
			return false
		}
		if normalizeExecutionWorkspaceView(page.exec.workspaceView) == executionWorkspaceProcess {
			return page.exec.processExecutionID != "" && event.ExecutionID == page.exec.processExecutionID
		}
		return !executionEventIsProcess(event)
	case executionScopeContainer:
		_, ok := page.exec.containerMembers[event.WorkspaceID]
		return ok && !executionEventIsProcess(event)
	default:
		return !executionEventIsProcess(event)
	}
}

func executionEventIsProcess(event shellruntime.ExecutionFeedEvent) bool {
	return event.Execution != nil && event.Execution.Tool == "start_process"
}

func (page *LogsPage) executionScopeLabel() string {
	if page == nil {
		return string(executionScopeCombined)
	}
	switch normalizeExecutionScopeMode(page.exec.scopeMode) {
	case executionScopeWorkspace:
		if normalizeExecutionWorkspaceView(page.exec.workspaceView) == executionWorkspaceProcess {
			return "workspace · " + page.exec.workspaceID + " · process · " + page.exec.processID
		}
		return "workspace · " + page.exec.workspaceID + " · commands"
	case executionScopeContainer:
		name := strings.TrimSpace(page.exec.containerName)
		if name == "" {
			name = page.exec.containerID
		}
		return fmt.Sprintf("container · %s · %d workspaces", name, len(page.exec.containerMembers))
	default:
		return string(executionScopeCombined)
	}
}

func (page *LogsPage) executionScopeEditorView(width, height int) string {
	if page == nil || page.exec.scopeEditor == nil {
		return ""
	}
	page.width, page.height = width, height
	page.resizeExecutionScopeEditor()
	return page.exec.scopeEditor.View()
}

func (page *LogsPage) resizeExecutionScopeEditor() {
	if page == nil || page.exec.scopeEditor == nil || page.width <= 0 || page.height <= 0 {
		return
	}
	page.exec.scopeEditor.Resize(page.width, page.height)
}

func (page *LogsPage) executionScopeEditorMouseTargets(originX, originY, z int) []component.MouseTarget {
	if page == nil || page.exec.scopeEditor == nil {
		return nil
	}
	return page.exec.scopeEditor.MouseTargets(originX, originY, z)
}
