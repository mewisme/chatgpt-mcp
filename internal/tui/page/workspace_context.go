package page

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/instructioncontext"
	"go.mewis.me/chatgpt-mcp/internal/projectcontext"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
)

type WorkspaceContextSession struct {
	Options    projectcontext.Options
	Result     *projectcontext.Result
	PreviewTab string
}

type workspaceContextFormData struct {
	Path                string
	MemoryQuery         string
	MaxMemoryEntries    string
	MaxMemoryBytes      string
	MaxInstructionBytes string
	MaxSectionBytes     string
	MaxLinesPerSection  string
	IncludeGit          bool
	IncludeMemory       bool
	IncludeSkills       bool
}

type workspaceContextBuildMsg struct {
	ID     uint64
	Result projectcontext.Result
	Err    error
}

type workspaceContextBuildFunc func(context.Context, string, projectcontext.Options) (projectcontext.Result, error)

func NewWorkspaceContextSession() *WorkspaceContextSession {
	return &WorkspaceContextSession{Options: defaultWorkspaceContextOptions(), PreviewTab: "rendered"}
}

func defaultWorkspaceContextOptions() projectcontext.Options {
	return projectcontext.DefaultOptions()
}

func newWorkspaceContextForm(options projectcontext.Options) (component.Form, *workspaceContextFormData) {
	data := &workspaceContextFormData{
		Path: strings.TrimSpace(options.Path), MemoryQuery: strings.TrimSpace(options.MemoryQuery), MaxMemoryEntries: strconv.Itoa(options.MaxMemoryEntries), MaxMemoryBytes: strconv.Itoa(options.MaxMemoryBytes),
		MaxInstructionBytes: strconv.Itoa(options.MaxInstructionBytes), MaxSectionBytes: strconv.Itoa(options.MaxSectionBytes), MaxLinesPerSection: strconv.Itoa(options.MaxLinesPerSection),
		IncludeGit: options.IncludeGit, IncludeMemory: options.IncludeMemory, IncludeSkills: options.IncludeSkills,
	}
	form := component.NewForm(
		component.Group(
			component.Input("Path", &data.Path).Description("Optional directory inside the workspace root"),
			component.Input("Memory query", &data.MemoryQuery).Description("Optional relevance query for cross-session memory"),
		).Title("Scope"),
		component.Group(
			workspaceContextIntInput("Max memory entries", &data.MaxMemoryEntries, projectcontext.MinMemoryEntries, projectcontext.MaxMemoryEntries),
			workspaceContextIntInput("Max memory bytes", &data.MaxMemoryBytes, projectcontext.MinMemoryBytes, projectcontext.MaxMemoryBytes),
			workspaceContextIntInput("Max instruction bytes", &data.MaxInstructionBytes, projectcontext.MinInstructionBytes, projectcontext.MaxInstructionBytes),
			workspaceContextIntInput("Max section bytes", &data.MaxSectionBytes, projectcontext.MinSectionBytes, projectcontext.MaxSectionBytes),
			workspaceContextIntInput("Max lines per section", &data.MaxLinesPerSection, projectcontext.MinLinesPerSection, projectcontext.MaxLinesPerSection),
		).Title("Budgets"),
		component.Group(
			component.Switch("Git", &data.IncludeGit), component.Switch("Memory", &data.IncludeMemory), component.Switch("Skills", &data.IncludeSkills),
		).Title("Include"),
	)
	return form, data
}

func workspaceContextIntInput(title string, value *string, minValue, maxValue int) huh.Field {
	return component.Input(title, value).Validate(func(raw string) error {
		parsed, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || parsed < minValue || parsed > maxValue {
			return fmt.Errorf("%s must be between %d and %d", strings.ToLower(title), minValue, maxValue)
		}
		return nil
	})
}

func (data *workspaceContextFormData) Options() (projectcontext.Options, error) {
	if data == nil {
		return projectcontext.Options{}, fmt.Errorf("project context form is unavailable")
	}
	parse := func(label, value string, minValue, maxValue int) (int, error) {
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil || parsed < minValue || parsed > maxValue {
			return 0, fmt.Errorf("%s must be between %d and %d", label, minValue, maxValue)
		}
		return parsed, nil
	}
	maxMemoryEntries, err := parse("max memory entries", data.MaxMemoryEntries, projectcontext.MinMemoryEntries, projectcontext.MaxMemoryEntries)
	if err != nil {
		return projectcontext.Options{}, err
	}
	maxMemoryBytes, err := parse("max memory bytes", data.MaxMemoryBytes, projectcontext.MinMemoryBytes, projectcontext.MaxMemoryBytes)
	if err != nil {
		return projectcontext.Options{}, err
	}
	maxInstructionBytes, err := parse("max instruction bytes", data.MaxInstructionBytes, projectcontext.MinInstructionBytes, projectcontext.MaxInstructionBytes)
	if err != nil {
		return projectcontext.Options{}, err
	}
	maxSectionBytes, err := parse("max section bytes", data.MaxSectionBytes, projectcontext.MinSectionBytes, projectcontext.MaxSectionBytes)
	if err != nil {
		return projectcontext.Options{}, err
	}
	maxLinesPerSection, err := parse("max lines per section", data.MaxLinesPerSection, projectcontext.MinLinesPerSection, projectcontext.MaxLinesPerSection)
	if err != nil {
		return projectcontext.Options{}, err
	}
	return projectcontext.Options{
		Path: strings.TrimSpace(data.Path), MemoryQuery: strings.TrimSpace(data.MemoryQuery), MaxMemoryEntries: maxMemoryEntries, MaxMemoryBytes: maxMemoryBytes,
		MaxInstructionBytes: maxInstructionBytes, MaxSectionBytes: maxSectionBytes, MaxLinesPerSection: maxLinesPerSection,
		IncludeGit: data.IncludeGit, IncludeMemory: data.IncludeMemory, IncludeSkills: data.IncludeSkills,
	}, nil
}

func (page *WorkspacePage) initWorkspaceContext() {
	if page.contextSession == nil {
		page.contextSession = NewWorkspaceContextSession()
	}
	if page.contextSession.Options.MaxInstructionBytes <= 0 {
		page.contextSession.Options = defaultWorkspaceContextOptions()
	}
	page.contextForm, page.contextData = newWorkspaceContextForm(page.contextSession.Options)
	if page.contextBuild == nil {
		manager := page.manager
		page.contextBuild = func(ctx context.Context, workspaceID string, options projectcontext.Options) (projectcontext.Result, error) {
			profile := application.ProjectContextToolProfile(ctx)
			service := projectcontext.New(manager, func() instructioncontext.ToolProfile { return profile })
			service.Environment = application.ProjectContextEnvironment
			return service.Build(ctx, workspaceID, options)
		}
	}
}

func (page *WorkspacePage) submitWorkspaceContext() tea.Cmd {
	options, err := page.contextData.Options()
	if err != nil {
		page.err = err
		return nil
	}
	page.contextBuildID++
	id := page.contextBuildID
	ctx, cancel := context.WithCancel(page.ctx)
	page.contextCancel = cancel
	page.contextBuilding = true
	page.err, page.notice = nil, ""
	progress := component.NewProgress("Building project context")
	page.contextProgress = &progress
	build, workspaceID := page.contextBuild, page.resourceID
	return tea.Batch(progress.Init(), func() tea.Msg {
		result, err := build(ctx, workspaceID, options)
		return workspaceContextBuildMsg{ID: id, Result: result, Err: err}
	})
}

func (page *WorkspacePage) finishWorkspaceContextBuild(msg workspaceContextBuildMsg) tea.Cmd {
	if msg.ID != page.contextBuildID {
		return nil
	}
	if page.contextCancel != nil {
		page.contextCancel()
	}
	page.contextCancel = nil
	page.contextBuilding = false
	if msg.Err != nil {
		page.err = msg.Err
		page.contextProgress = nil
		return nil
	}
	options, err := page.contextData.Options()
	if err != nil {
		page.err = err
		page.contextProgress = nil
		return nil
	}
	page.contextSession.Options = options
	result := msg.Result
	page.contextSession.Result = &result
	page.contextProgress = nil
	page.notice = "Project Context built"
	return func() tea.Msg { return NavigateMsg{Path: []string{"workspaces", page.resourceID, "context-preview"}} }
}

func (page *WorkspacePage) cancelWorkspaceContextBuild() {
	if page.contextCancel != nil {
		page.contextCancel()
	}
	page.contextCancel = nil
	page.contextBuildID++
	page.contextBuilding = false
	page.contextProgress = nil
	page.notice = "Project Context build cancelled"
}

func (page *WorkspacePage) workspaceContextView(width, height int) string {
	title := component.PageTitleNotice("Project Context · "+page.resourceID, page.notice, width)
	feedback := ""
	if page.err != nil {
		feedback = component.BannerWidth(page.err.Error(), component.ToneDanger, width)
	}
	bodyHeight := max(1, height-lipgloss.Height(title)-1-pageFeedbackHeight(feedback))
	if page.contextBuilding && page.contextProgress != nil {
		body := component.CenterLayout(page.contextProgress.View()+"\n\n"+component.Muted("Esc cancel"), width, bodyHeight)
		return title + "\n" + prependPageFeedback(feedback, body)
	}
	form, _ := page.contextForm.Update(tea.WindowSizeMsg{Width: width, Height: bodyHeight})
	page.contextForm = form
	return title + "\n" + prependPageFeedback(feedback, page.contextForm.View())
}
