package page

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

type WorkspaceCommand string

const (
	WorkspaceRegister         WorkspaceCommand = "workspace.register"
	WorkspaceUnregister       WorkspaceCommand = "workspace.unregister"
	WorkspaceAccessAdd        WorkspaceCommand = "workspace.access.add"
	WorkspaceAccessRemove     WorkspaceCommand = "workspace.access.remove"
	WorkspaceContainerCreate  WorkspaceCommand = "workspace.container.create"
	WorkspaceContainerRename  WorkspaceCommand = "workspace.container.rename"
	WorkspaceContainerDelete  WorkspaceCommand = "workspace.container.delete"
	WorkspaceContainerMembers WorkspaceCommand = "workspace.container.members"
)

type WorkspaceCommandMsg struct {
	Command    WorkspaceCommand
	ResourceID string
}

type workspaceTab uint8

const (
	workspaceTabWorkspaces workspaceTab = iota
	workspaceTabContainers
)

var workspaceTabLabels = []string{"Workspaces", "Containers"}

type workspaceOverlayKind uint8

const (
	workspaceOverlayNone workspaceOverlayKind = iota
	workspaceOverlayForm
	workspaceOverlayConfirm
)

type WorkspacePage struct {
	ctx        context.Context
	manager    *workspace.Manager
	tab        workspaceTab
	resourceID string
	browser    component.Browser
	overlay    workspaceOverlayKind
	form       component.Form
	confirm    component.ConfirmButtons
	command    WorkspaceCommand
	targetID   string
	value      string
	members    []string
	notice     string
	err        error
	width      int
	height     int
}

var copyWorkspaceID = component.CopyText

func NewWorkspaces(ctx context.Context, resourceID string) (*WorkspacePage, error) {
	return newWorkspacePage(ctx, workspaceTabWorkspaces, resourceID)
}

func NewContainers(ctx context.Context, resourceID string) (*WorkspacePage, error) {
	return newWorkspacePage(ctx, workspaceTabContainers, resourceID)
}

func newWorkspacePage(ctx context.Context, tab workspaceTab, resourceID string) (*WorkspacePage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	page := &WorkspacePage{ctx: ctx, manager: workspace.NewManager(workspace.DefaultStorePath()), tab: tab, resourceID: strings.TrimSpace(resourceID)}
	if err := page.reload(); err != nil {
		return nil, err
	}
	if page.resourceID != "" && !page.browser.OpenDetail(page.resourceID) {
		return nil, fmt.Errorf("resource not found: %s", page.resourceID)
	}
	return page, nil
}

func (page *WorkspacePage) Init() tea.Cmd { return nil }

func (page *WorkspacePage) OverlayActive() bool {
	return page != nil && (page.overlay != workspaceOverlayNone || page.browser.DetailOpen())
}

func (page *WorkspacePage) InputActive() bool {
	return page != nil && (page.overlay == workspaceOverlayForm || page.browser.InputActive())
}

func (page *WorkspacePage) Notice() string {
	if page == nil {
		return ""
	}
	return page.notice
}

func (page *WorkspacePage) SetNotice(value string) {
	if page != nil {
		page.notice = strings.TrimSpace(value)
	}
}

func (page *WorkspacePage) Update(message tea.Msg) (Model, tea.Cmd) {
	if page == nil {
		return page, nil
	}
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		page.width, page.height = msg.Width, msg.Height
		tabs := component.PageTabsNotice(workspaceTabLabels, int(page.tab), page.notice, msg.Width)
		updated, cmd := page.browser.Update(tea.WindowSizeMsg{Width: msg.Width, Height: max(1, msg.Height-lipgloss.Height(tabs))})
		page.browser = updated.(component.Browser)
		if page.overlay == workspaceOverlayForm {
			form, formCmd := page.form.Update(msg)
			page.form = form
			return page, tea.Batch(cmd, formCmd)
		}
		return page, cmd
	case component.FormSubmittedMsg:
		return page, page.submitForm()
	case component.FormCancelledMsg:
		page.closeOverlay()
		return page, nil
	case component.FormMouseMsg:
		if page.overlay == workspaceOverlayForm {
			updated, cmd := page.form.Update(msg)
			page.form = updated
			return page, cmd
		}
		return page, nil
	case component.ConfirmChoiceMsg:
		if page.overlay == workspaceOverlayConfirm {
			page.confirm.Select(msg.Affirmative)
			return page, page.updateConfirm(tea.KeyPressMsg{Code: tea.KeyEnter})
		}
		return page, nil
	case WorkspaceCommandMsg:
		cmd, err := page.openCommand(msg.Command, msg.ResourceID)
		if err != nil {
			page.err = err
		}
		return page, cmd
	case tea.KeyPressMsg:
		if page.overlay == workspaceOverlayForm {
			updated, cmd := page.form.Update(msg)
			page.form = updated
			return page, cmd
		}
		if page.overlay == workspaceOverlayConfirm {
			return page, page.updateConfirm(msg)
		}
		if page.browser.InputActive() {
			updated, cmd := page.browser.Update(msg)
			page.browser = updated.(component.Browser)
			return page, cmd
		}
		if !page.browser.DetailOpen() {
			if cmd, handled := page.handleTabKey(msg); handled {
				return page, cmd
			}
			if cmd, handled := page.handleListKey(msg); handled {
				return page, cmd
			}
		}
	}
	if page.overlay == workspaceOverlayForm {
		updated, cmd := page.form.Update(message)
		page.form = updated
		return page, cmd
	}
	updated, cmd := page.browser.Update(message)
	page.browser = updated.(component.Browser)
	return page, cmd
}

func (page *WorkspacePage) View(width, height int) string {
	if page == nil {
		return component.StateView(component.PageError, "Workspace page unavailable", "")
	}
	page.width, page.height = width, height
	tabs := component.PageTabsNotice(workspaceTabLabels, int(page.tab), page.notice, width)
	feedback := ""
	if page.err != nil {
		feedback = component.Banner(page.err.Error(), component.ToneDanger)
	}
	browserHeight := max(1, height-lipgloss.Height(tabs)-pageFeedbackHeight(feedback))
	if width > 0 && browserHeight > 0 {
		updated, _ := page.browser.Update(tea.WindowSizeMsg{Width: width, Height: browserHeight})
		page.browser = updated.(component.Browser)
	}
	content := tabs + "\n" + prependPageFeedback(feedback, page.browser.Content())
	switch page.overlay {
	case workspaceOverlayForm:
		content = component.CenterOverlay(content, component.Modal(page.form.View(), page.formOverlayWidth(width)), width, height)
	case workspaceOverlayConfirm:
		body := component.Title(page.confirmTitle()) + "\n\n" + component.Muted(page.confirmDescription()) + "\n\n" + page.confirm.View() + "\n" + component.Muted("Enter confirm · Esc cancel")
		content = component.CenterOverlay(content, component.Modal(body, overlayWidth(width, 64)), width, height)
	}
	return content
}

func (page *WorkspacePage) MouseTargets(originX, originY, z int) []component.MouseTarget {
	if page == nil {
		return nil
	}
	switch page.overlay {
	case workspaceOverlayForm:
		return formOverlayMouseTargets(page.form, page.formOverlayWidth(page.width), page.width, page.height, originX, originY, z+20)
	case workspaceOverlayConfirm:
		return confirmOverlayMouseTargets(page.confirm, page.confirmTitle(), page.confirmDescription(), overlayWidth(page.width, 64), page.width, page.height, originX, originY, z+20)
	default:
		feedback := ""
		if page.err != nil {
			feedback = component.Banner(page.err.Error(), component.ToneDanger)
		}
		tabs := component.PageTabsNotice(workspaceTabLabels, int(page.tab), page.notice, page.width)
		tabTargets := page.workspaceTabMouseTargets(originX, originY, z+2)
		browserY := originY + lipgloss.Height(tabs) + pageFeedbackHeight(feedback)
		return append(tabTargets, page.browser.MouseTargets(originX, browserY, z)...)
	}
}

func (page *WorkspacePage) handleTabKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "1":
		return page.switchWorkspaceTab(workspaceTabWorkspaces), true
	case "2":
		return page.switchWorkspaceTab(workspaceTabContainers), true
	}
	if delta, ok := component.TabDelta(msg); ok {
		next := component.MoveTab(int(page.tab), len(workspaceTabLabels), delta)
		return page.switchWorkspaceTab(workspaceTab(next)), true
	}
	return nil, false
}

func (page *WorkspacePage) switchWorkspaceTab(tab workspaceTab) tea.Cmd {
	if tab > workspaceTabContainers || page.tab == tab {
		return nil
	}
	page.tab, page.resourceID, page.err = tab, "", nil
	if err := page.reload(); err != nil {
		page.err = err
	}
	return nil
}

func (page *WorkspacePage) workspaceTabMouseTargets(originX, originY, z int) []component.MouseTarget {
	_, spans := component.PageTabsLayout(workspaceTabLabels, int(page.tab), page.notice, page.width)
	targets := make([]component.MouseTarget, 0, len(spans))
	for _, span := range spans {
		tab := span.Index
		targets = append(targets, component.MouseTarget{
			ID: "workspace.tab", Rect: component.Rect{X: originX + span.X, Y: originY, Width: span.Width, Height: 1}, Z: z,
			Handle: func(event component.MouseEvent) tea.Msg {
				if event.Button != tea.MouseLeft {
					return nil
				}
				if tab == 0 {
					return tea.KeyPressMsg{Code: '1'}
				}
				return tea.KeyPressMsg{Code: '2'}
			},
		})
	}
	return targets
}

func (page *WorkspacePage) handleListKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	selected, _ := page.browser.Selected()
	switch msg.String() {
	case "enter":
		if selected.ID == "" {
			return nil, true
		}
		path := []string{"workspace", selected.ID}
		if page.tab == workspaceTabContainers {
			path = []string{"containers", selected.ID}
		}
		return func() tea.Msg { return NavigateMsg{Path: path} }, true
	case "a":
		command := WorkspaceRegister
		if page.tab == workspaceTabContainers {
			command = WorkspaceContainerCreate
		}
		cmd, err := page.openCommand(command, "")
		if err != nil {
			page.err = err
		}
		return cmd, true
	}
	return nil, false
}

func (page *WorkspacePage) openCommand(command WorkspaceCommand, resourceID string) (tea.Cmd, error) {
	page.err, page.notice = nil, ""
	page.command, page.targetID, page.value, page.members = command, strings.TrimSpace(resourceID), "", nil
	switch command {
	case WorkspaceRegister:
		if cwd, err := os.Getwd(); err == nil {
			page.value = cwd
		}
		page.form = component.NewForm(component.Group(component.Input("Workspace path", &page.value).Validate(requiredValue("workspace path"))))
		page.overlay = workspaceOverlayForm
		return page.form.Init(), nil
	case WorkspaceAccessAdd:
		if _, err := page.manager.Get(page.targetID); err != nil {
			return nil, err
		}
		page.form = component.NewForm(component.Group(component.Input("Additional directory", &page.value).Validate(requiredValue("directory"))))
		page.overlay = workspaceOverlayForm
		return page.form.Init(), nil
	case WorkspaceAccessRemove:
		item, err := page.manager.Get(page.targetID)
		if err != nil {
			return nil, err
		}
		if len(item.AllowDirs) == 0 {
			return nil, fmt.Errorf("workspace has no additional directories")
		}
		page.value = item.AllowDirs[0]
		options := make([]huh.Option[string], 0, len(item.AllowDirs))
		for _, value := range item.AllowDirs {
			options = append(options, huh.NewOption(value, value))
		}
		page.form = component.NewForm(component.Group(component.Select("Directory to remove", &page.value, options...)))
		page.overlay = workspaceOverlayForm
		return page.form.Init(), nil
	case WorkspaceContainerCreate:
		page.form = component.NewForm(component.Group(component.Input("Container name", &page.value).Validate(requiredValue("container name"))))
		page.overlay = workspaceOverlayForm
		return page.form.Init(), nil
	case WorkspaceContainerRename:
		item, err := page.manager.GetContainer(page.targetID)
		if err != nil {
			return nil, err
		}
		page.value = item.Name
		page.form = component.NewForm(component.Group(component.Input("Container name", &page.value).Validate(requiredValue("container name"))))
		page.overlay = workspaceOverlayForm
		return page.form.Init(), nil
	case WorkspaceContainerMembers:
		item, err := page.manager.GetContainer(page.targetID)
		if err != nil {
			return nil, err
		}
		items, err := page.manager.List()
		if err != nil {
			return nil, err
		}
		page.members = append([]string(nil), item.WorkspaceIDs...)
		options := make([]huh.Option[string], 0, len(items))
		for _, workspaceItem := range items {
			options = append(options, huh.NewOption(workspaceMemberLabel(workspaceItem), workspaceItem.ID))
		}
		field := component.MultiSelect("Container workspaces", &page.members, options...).Filterable(true).Height(workspaceMemberPickerHeight(len(items), page.height))
		field.DescriptionFunc(func() string { return fmt.Sprintf("%d selected / %d available", len(page.members), len(items)) }, &page.members)
		page.form = component.NewForm(component.Group(field))
		page.overlay = workspaceOverlayForm
		return page.form.Init(), nil
	case WorkspaceUnregister, WorkspaceContainerDelete:
		if command == WorkspaceUnregister {
			if _, err := page.manager.Get(page.targetID); err != nil {
				return nil, err
			}
		} else if _, err := page.manager.GetContainer(page.targetID); err != nil {
			return nil, err
		}
		page.confirm = component.NewConfirmButtons("Delete", "Cancel", false)
		page.overlay = workspaceOverlayConfirm
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported workspace action: %s", command)
	}
}

func (page *WorkspacePage) submitForm() tea.Cmd {
	var err error
	switch page.command {
	case WorkspaceRegister:
		_, err = page.manager.Register(page.value)
	case WorkspaceAccessAdd:
		_, err = page.manager.AddAllowDir(page.targetID, page.value)
	case WorkspaceAccessRemove:
		_, err = page.manager.RemoveAllowDir(page.targetID, page.value)
	case WorkspaceContainerCreate:
		_, err = page.manager.CreateContainer(page.value)
	case WorkspaceContainerRename:
		_, err = page.manager.RenameContainer(page.targetID, page.value)
	case WorkspaceContainerMembers:
		err = page.updateMembers()
	default:
		err = fmt.Errorf("unsupported form action: %s", page.command)
	}
	if err != nil {
		page.err = err
		return nil
	}
	page.notice = workspaceSuccess(page.command)
	page.closeOverlay()
	if err := page.reload(); err != nil {
		page.err = err
	}
	return nil
}

func (page *WorkspacePage) updateConfirm(msg tea.KeyPressMsg) tea.Cmd {
	if msg.String() == "esc" {
		page.closeOverlay()
		return nil
	}
	if msg.String() == "enter" {
		if !page.confirm.AffirmativeSelected() {
			page.closeOverlay()
			return nil
		}
		var err error
		if page.command == WorkspaceUnregister {
			err = page.manager.Unregister(page.targetID)
		} else {
			err = page.manager.DeleteContainer(page.targetID)
		}
		if err != nil {
			page.err = err
			return nil
		}
		page.notice = workspaceSuccess(page.command)
		deletedID := page.targetID
		page.closeOverlay()
		page.resourceID = ""
		if err := page.reload(); err != nil {
			page.err = err
		}
		if deletedID != "" {
			path := []string{"workspaces"}
			if page.tab == workspaceTabContainers {
				path = []string{"containers"}
			}
			return func() tea.Msg { return NavigateMsg{Path: path} }
		}
		return nil
	}
	return page.confirm.Update(msg)
}

func (page *WorkspacePage) updateMembers() error {
	container, err := page.manager.GetContainer(page.targetID)
	if err != nil {
		return err
	}
	before, after := stringSet(container.WorkspaceIDs), stringSet(page.members)
	add, remove := []string{}, []string{}
	for id := range after {
		if !before[id] {
			add = append(add, id)
		}
	}
	for id := range before {
		if !after[id] {
			remove = append(remove, id)
		}
	}
	sort.Strings(add)
	sort.Strings(remove)
	if len(add) > 0 {
		if _, err := page.manager.AddWorkspacesToContainer(page.targetID, add); err != nil {
			return err
		}
	}
	if len(remove) > 0 {
		if _, err := page.manager.RemoveWorkspacesFromContainer(page.targetID, remove); err != nil {
			return err
		}
	}
	return nil
}

func (page *WorkspacePage) reload() error {
	helpExpanded := page.browser.HelpExpanded()
	detailOpen := page.browser.DetailOpen()
	selected, _ := page.browser.Selected()
	var rows []component.Row
	var err error
	if page.tab == workspaceTabContainers {
		rows, err = page.containerRows()
	} else {
		rows, err = page.workspaceRows()
	}
	if err != nil {
		return err
	}
	title := workspaceTabLabels[int(page.tab)]
	refresh := func(context.Context) ([]component.Row, error) {
		if page.tab == workspaceTabContainers {
			return page.containerRows()
		}
		return page.workspaceRows()
	}
	page.browser = component.NewBrowser(page.ctx, title, rows, refresh).WithTitleVisible(false).WithDetailAction(component.RowAction{Key: "c", Desc: "copy ID", Run: func(row component.Row) (string, tea.Cmd, error) {
		if err := copyWorkspaceID(row.ID); err != nil {
			return "", nil, err
		}
		return "Copied " + row.ID, nil, nil
	}})
	detailAction := func(key, desc string, command WorkspaceCommand) component.RowAction {
		return component.RowAction{Key: key, Desc: desc, Run: func(row component.Row) (string, tea.Cmd, error) {
			return "", func() tea.Msg { return WorkspaceCommandMsg{Command: command, ResourceID: row.ID} }, nil
		}}
	}
	page.browser.SetHelpExpanded(helpExpanded)
	if page.tab == workspaceTabContainers {
		page.browser = page.browser.WithDetailAction(detailAction("e", "rename", WorkspaceContainerRename)).WithDetailAction(detailAction("m", "members", WorkspaceContainerMembers)).WithDetailAction(detailAction("d", "delete", WorkspaceContainerDelete))
		page.browser.SetHelpBindings(component.Binding([]string{"h", "l", "left", "right"}, "←/→", "tabs"), component.Binding([]string{"a"}, "a", "create"))
	} else {
		page.browser = page.browser.WithDetailAction(detailAction("+", "add access", WorkspaceAccessAdd)).WithDetailAction(detailAction("-", "remove access", WorkspaceAccessRemove)).WithDetailAction(detailAction("d", "unregister", WorkspaceUnregister))
		page.browser.SetHelpBindings(component.Binding([]string{"h", "l", "left", "right"}, "←/→", "tabs"), component.Binding([]string{"a"}, "a", "register"))
	}
	if page.width > 0 && page.height > 0 {
		tabs := component.PageTabsNotice(workspaceTabLabels, int(page.tab), page.notice, page.width)
		updated, _ := page.browser.Update(tea.WindowSizeMsg{Width: page.width, Height: max(1, page.height-lipgloss.Height(tabs))})
		page.browser = updated.(component.Browser)
	}
	if page.resourceID != "" {
		page.browser.OpenDetail(page.resourceID)
	} else if detailOpen && selected.ID != "" {
		page.browser.OpenDetail(selected.ID)
	}
	return nil
}

func (page *WorkspacePage) workspaceRows() ([]component.Row, error) {
	items, err := page.manager.List()
	if err != nil {
		return nil, err
	}
	rows := make([]component.Row, 0, len(items))
	for _, item := range items {
		containers, err := page.manager.ContainersForWorkspace(item.ID)
		if err != nil {
			return nil, err
		}
		containerNames := make([]string, 0, len(containers))
		for _, container := range containers {
			containerNames = append(containerNames, container.Name+" ("+container.ID+")")
		}
		rows = append(rows, component.Row{
			ID: item.ID, Title: item.ID, Description: item.Path, Meta: fmt.Sprintf("%d extra roots", len(item.AllowDirs)), Search: strings.Join(append(append([]string{item.Path}, item.AllowDirs...), item.LegacyIDs...), " "),
			DetailTitle: "Workspace · " + item.ID,
			DetailTabs: []component.DetailTab{
				{Title: "Overview", Content: detailFields([2]string{"Root", item.Path}, [2]string{"Legacy IDs", joinedOrNone(item.LegacyIDs)})},
				{Title: "Access", Content: detailList(item.AllowDirs)},
				{Title: "Containers", Content: detailList(containerNames)},
			},
		})
	}
	return rows, nil
}

func (page *WorkspacePage) containerRows() ([]component.Row, error) {
	items, err := page.manager.ListContainers()
	if err != nil {
		return nil, err
	}
	rows := make([]component.Row, 0, len(items))
	for _, item := range items {
		workspaces, err := page.manager.WorkspacesForContainer(item.ID)
		if err != nil {
			return nil, err
		}
		members := make([]string, 0, len(workspaces))
		for _, workspaceItem := range workspaces {
			members = append(members, workspaceItem.ID+" · "+workspaceItem.Path)
		}
		rows = append(rows, component.Row{ID: item.ID, Title: item.Name, Description: item.ID, Meta: fmt.Sprintf("%d workspaces", len(item.WorkspaceIDs)), Search: strings.Join(item.WorkspaceIDs, " "), DetailTitle: "Container · " + item.Name, DetailTabs: []component.DetailTab{{Title: "Overview", Content: detailFields([2]string{"ID", item.ID}, [2]string{"Name", item.Name})}, {Title: "Workspaces", Content: detailList(members)}}})
	}
	return rows, nil
}

func (page *WorkspacePage) closeOverlay() {
	page.overlay = workspaceOverlayNone
	page.form = component.Form{}
	page.confirm = component.ConfirmButtons{}
	page.command, page.targetID = "", ""
	page.value, page.members = "", nil
}

func (page *WorkspacePage) formOverlayWidth(width int) int {
	if page.command == WorkspaceContainerMembers {
		return overlayWidth(width, 94)
	}
	return overlayWidth(width, 72)
}

func workspaceMemberPickerHeight(count, pageHeight int) int {
	limit := 14
	if pageHeight > 0 {
		limit = max(6, min(16, pageHeight-8))
	}
	return max(6, min(limit, count+3))
}

func workspaceMemberLabel(item workspace.Workspace) string {
	name := strings.TrimSpace(filepath.Base(filepath.Clean(item.Path)))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return item.ID
	}
	return name + " · " + item.ID
}

func (page *WorkspacePage) confirmTitle() string {
	if page.command == WorkspaceUnregister {
		return "Unregister workspace " + page.targetID + "?"
	}
	return "Delete container " + page.targetID + "?"
}

func (page *WorkspacePage) confirmDescription() string {
	if page.command == WorkspaceUnregister {
		return "The workspace handle and workspace-scoped state will be removed. Project files are unchanged."
	}
	return "The container record will be removed. Registered workspaces and project files are unchanged."
}

func workspaceSuccess(command WorkspaceCommand) string {
	switch command {
	case WorkspaceRegister:
		return "Workspace registered"
	case WorkspaceUnregister:
		return "Workspace unregistered"
	case WorkspaceAccessAdd:
		return "Access directory added"
	case WorkspaceAccessRemove:
		return "Access directory removed"
	case WorkspaceContainerCreate:
		return "Container created"
	case WorkspaceContainerRename:
		return "Container renamed"
	case WorkspaceContainerDelete:
		return "Container deleted"
	case WorkspaceContainerMembers:
		return "Container membership updated"
	default:
		return "Workspace updated"
	}
}

func requiredValue(label string) func(string) error {
	return func(value string) error {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s is required", label)
		}
		return nil
	}
}

func stringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result[value] = true
		}
	}
	return result
}

func detailFields(fields ...[2]string) string {
	lines := make([]string, 0, len(fields))
	for _, field := range fields {
		value := strings.TrimSpace(field[1])
		if value == "" {
			value = "None"
		}
		lines = append(lines, component.Label(field[0])+"  "+value)
	}
	return strings.Join(lines, "\n")
}

func detailList(values []string) string {
	if len(values) == 0 {
		return component.Muted("None")
	}
	return strings.Join(values, "\n")
}

func joinedOrNone(values []string) string {
	if len(values) == 0 {
		return "None"
	}
	return strings.Join(values, ", ")
}
