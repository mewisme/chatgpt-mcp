package page

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"

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

type executionScopeFormData struct {
	Mode        string
	WorkspaceID string
	ContainerID string
	View        string
	ProcessID   string
	Processes   map[string][]shellruntime.ProcessInfo
}

func newExecutionScopeEditor(ctx context.Context, feed logsExecutionFeed) (component.Editor, *executionScopeFormData, error) {
	manager := workspace.NewManager(workspace.DefaultStorePath())
	workspaces, err := manager.List()
	if err != nil {
		return component.Editor{}, nil, err
	}
	containers, err := manager.ListContainers()
	if err != nil {
		return component.Editor{}, nil, err
	}
	data := &executionScopeFormData{Mode: string(normalizeExecutionScopeMode(feed.scopeMode)), WorkspaceID: feed.workspaceID, ContainerID: feed.containerID, View: string(normalizeExecutionWorkspaceView(feed.workspaceView)), ProcessID: feed.processID, Processes: map[string][]shellruntime.ProcessInfo{}}
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
	form := component.NewEditorForm(
		component.Group(component.Select("Mode", &data.Mode,
			huh.NewOption("Combined", string(executionScopeCombined)),
			huh.NewOption("Workspace", string(executionScopeWorkspace)),
			huh.NewOption("Container", string(executionScopeContainer)),
		)),
		component.Group(component.Select("Workspace", &data.WorkspaceID, workspaceOptions...)).WithHideFunc(func() bool { return data.Mode != string(executionScopeWorkspace) }),
		component.Group(component.Select("View", &data.View,
			huh.NewOption("Run commands", string(executionWorkspaceCommands)),
			huh.NewOption("View process", string(executionWorkspaceProcess)),
		)).WithHideFunc(func() bool { return data.Mode != string(executionScopeWorkspace) }),
		component.Group(huh.NewSelect[string]().Title("Process").Value(&data.ProcessID).OptionsFunc(func() []huh.Option[string] {
			return executionProcessOptions(data.Processes[data.WorkspaceID])
		}, &data.WorkspaceID)).WithHideFunc(func() bool {
			return data.Mode != string(executionScopeWorkspace) || data.View != string(executionWorkspaceProcess)
		}),
		component.Group(component.Select("Container", &data.ContainerID, containerOptions...)).WithHideFunc(func() bool { return data.Mode != string(executionScopeContainer) }),
	)
	editor := component.NewEditor("apply", component.EditorSection{ID: "scope", Title: "Scope", Description: "Filter command execution output without reconnecting the global stream.", Form: form}).WithSubmitMode(component.EditorSubmitOnComplete)
	return editor, data, nil
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

func (page *LogsPage) openExecutionScopeEditor() tea.Cmd {
	if page == nil || page.exec.scopeEditor != nil {
		return nil
	}
	editor, data, err := newExecutionScopeEditor(page.ctx, page.exec)
	if err != nil {
		page.exec.err = err
		return nil
	}
	page.exec.scopeEditor, page.exec.scopeForm = &editor, data
	page.resizeExecutionScopeEditor()
	return editor.Init()
}

func (page *LogsPage) closeExecutionScopeEditor() tea.Cmd {
	if page == nil {
		return nil
	}
	page.exec.scopeEditor, page.exec.scopeForm = nil, nil
	return nil
}

func (page *LogsPage) submitExecutionScopeEditor() tea.Cmd {
	if page == nil || page.exec.scopeEditor == nil || page.exec.scopeForm == nil {
		return nil
	}
	mode := normalizeExecutionScopeMode(executionScopeMode(strings.TrimSpace(page.exec.scopeForm.Mode)))
	manager := workspace.NewManager(workspace.DefaultStorePath())
	workspaceID, containerID := strings.TrimSpace(page.exec.scopeForm.WorkspaceID), strings.TrimSpace(page.exec.scopeForm.ContainerID)
	workspaceView := normalizeExecutionWorkspaceView(executionWorkspaceView(strings.TrimSpace(page.exec.scopeForm.View)))
	processID := strings.TrimSpace(page.exec.scopeForm.ProcessID)
	processExecutionID := ""
	processRunning := false
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
	page.exec.scopeEditor, page.exec.scopeForm = nil, nil
	page.exec.err = nil
	page.refreshExecutionViewport()
	return nil
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
	title := component.PageTitle("Command Execution Scope", width)
	page.resizeExecutionScopeEditor()
	return title + "\n" + page.exec.scopeEditor.View()
}

func (page *LogsPage) resizeExecutionScopeEditor() {
	if page == nil || page.exec.scopeEditor == nil || page.width <= 0 || page.height <= 0 {
		return
	}
	title := component.PageTitle("Command Execution Scope", page.width)
	page.exec.scopeEditor.Resize(page.width, max(1, page.height-lipgloss.Height(title)-1))
}

func (page *LogsPage) executionScopeEditorMouseTargets(originX, originY, z int) []component.MouseTarget {
	if page == nil || page.exec.scopeEditor == nil {
		return nil
	}
	title := component.PageTitle("Command Execution Scope", page.width)
	return page.exec.scopeEditor.MouseTargets(originX, originY+lipgloss.Height(title)+1, z)
}
