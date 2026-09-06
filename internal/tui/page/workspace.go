package page

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
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

type workspacePageKind uint8

const (
	workspacePageWorkspaces workspacePageKind = iota
	workspacePageContainers
)

type workspaceOverlayKind uint8

const (
	workspaceOverlayNone workspaceOverlayKind = iota
	workspaceOverlayForm
	workspaceOverlayConfirm
)

type WorkspacePage struct {
	ctx        context.Context
	manager    *workspace.Manager
	kind       workspacePageKind
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
	return newWorkspacePage(ctx, workspacePageWorkspaces, resourceID)
}

func NewContainers(ctx context.Context, resourceID string) (*WorkspacePage, error) {
	return newWorkspacePage(ctx, workspacePageContainers, resourceID)
}

func newWorkspacePage(ctx context.Context, kind workspacePageKind, resourceID string) (*WorkspacePage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	page := &WorkspacePage{ctx: ctx, manager: workspace.NewManager(workspace.DefaultStorePath()), kind: kind, resourceID: strings.TrimSpace(resourceID)}
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

func (page *WorkspacePage) Update(message tea.Msg) (Model, tea.Cmd) {
	if page == nil {
		return page, nil
	}
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		page.width, page.height = msg.Width, msg.Height
		updated, cmd := page.browser.Update(msg)
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
		if err := page.openCommand(msg.Command, msg.ResourceID); err != nil {
			page.err = err
		}
		return page, nil
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
	browserHeight := height
	if page.err != nil || page.notice != "" {
		browserHeight = max(1, height-2)
	}
	if width > 0 && browserHeight > 0 && (page.width != width || page.height != browserHeight) {
		updated, _ := page.browser.Update(tea.WindowSizeMsg{Width: width, Height: browserHeight})
		page.browser = updated.(component.Browser)
		page.width, page.height = width, browserHeight
	}
	content := page.browser.Content()
	if page.err != nil {
		content += "\n" + component.Banner(page.err.Error(), component.ToneDanger)
	} else if page.notice != "" {
		content += "\n" + component.Banner(page.notice, component.ToneSuccess)
	}
	switch page.overlay {
	case workspaceOverlayForm:
		content = component.CenterOverlay(content, component.Modal(page.form.View(), overlayWidth(width, 72)), width, height)
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
		return formOverlayMouseTargets(page.form, overlayWidth(page.width, 72), page.width, page.height, originX, originY, z+20)
	case workspaceOverlayConfirm:
		return confirmOverlayMouseTargets(page.confirm, page.confirmTitle(), page.confirmDescription(), overlayWidth(page.width, 64), page.width, page.height, originX, originY, z+20)
	default:
		return page.browser.MouseTargets(originX, originY, z)
	}
}

func (page *WorkspacePage) handleListKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	selected, _ := page.browser.Selected()
	switch msg.String() {
	case "enter":
		if selected.ID == "" {
			return nil, true
		}
		path := []string{"workspace", selected.ID}
		if page.kind == workspacePageContainers {
			path = []string{"containers", selected.ID}
		}
		return func() tea.Msg { return NavigateMsg{Path: path} }, true
	case "a":
		command := WorkspaceRegister
		if page.kind == workspacePageContainers {
			command = WorkspaceContainerCreate
		}
		if err := page.openCommand(command, ""); err != nil {
			page.err = err
		}
		return nil, true
	case "d":
		if selected.ID == "" {
			return nil, true
		}
		command := WorkspaceUnregister
		if page.kind == workspacePageContainers {
			command = WorkspaceContainerDelete
		}
		if err := page.openCommand(command, selected.ID); err != nil {
			page.err = err
		}
		return nil, true
	case "e":
		if page.kind == workspacePageContainers && selected.ID != "" {
			if err := page.openCommand(WorkspaceContainerRename, selected.ID); err != nil {
				page.err = err
			}
			return nil, true
		}
	case "+":
		if page.kind == workspacePageWorkspaces && selected.ID != "" {
			if err := page.openCommand(WorkspaceAccessAdd, selected.ID); err != nil {
				page.err = err
			}
			return nil, true
		}
	case "-":
		if page.kind == workspacePageWorkspaces && selected.ID != "" {
			if err := page.openCommand(WorkspaceAccessRemove, selected.ID); err != nil {
				page.err = err
			}
			return nil, true
		}
	case "m":
		if page.kind == workspacePageContainers && selected.ID != "" {
			if err := page.openCommand(WorkspaceContainerMembers, selected.ID); err != nil {
				page.err = err
			}
			return nil, true
		}
	}
	return nil, false
}

func (page *WorkspacePage) openCommand(command WorkspaceCommand, resourceID string) error {
	page.err, page.notice = nil, ""
	page.command, page.targetID, page.value, page.members = command, strings.TrimSpace(resourceID), "", nil
	switch command {
	case WorkspaceRegister:
		if cwd, err := os.Getwd(); err == nil {
			page.value = cwd
		}
		page.form = component.NewForm(component.Group(component.Input("Workspace path", &page.value).Validate(requiredValue("workspace path"))))
		page.overlay = workspaceOverlayForm
	case WorkspaceAccessAdd:
		if _, err := page.manager.Get(page.targetID); err != nil {
			return err
		}
		page.form = component.NewForm(component.Group(component.Input("Additional directory", &page.value).Validate(requiredValue("directory"))))
		page.overlay = workspaceOverlayForm
	case WorkspaceAccessRemove:
		item, err := page.manager.Get(page.targetID)
		if err != nil {
			return err
		}
		if len(item.AllowDirs) == 0 {
			return fmt.Errorf("workspace has no additional directories")
		}
		page.value = item.AllowDirs[0]
		options := make([]huh.Option[string], 0, len(item.AllowDirs))
		for _, value := range item.AllowDirs {
			options = append(options, huh.NewOption(value, value))
		}
		page.form = component.NewForm(component.Group(component.Select("Directory to remove", &page.value, options...)))
		page.overlay = workspaceOverlayForm
	case WorkspaceContainerCreate:
		page.form = component.NewForm(component.Group(component.Input("Container name", &page.value).Validate(requiredValue("container name"))))
		page.overlay = workspaceOverlayForm
	case WorkspaceContainerRename:
		item, err := page.manager.GetContainer(page.targetID)
		if err != nil {
			return err
		}
		page.value = item.Name
		page.form = component.NewForm(component.Group(component.Input("Container name", &page.value).Validate(requiredValue("container name"))))
		page.overlay = workspaceOverlayForm
	case WorkspaceContainerMembers:
		item, err := page.manager.GetContainer(page.targetID)
		if err != nil {
			return err
		}
		items, err := page.manager.List()
		if err != nil {
			return err
		}
		page.members = append([]string(nil), item.WorkspaceIDs...)
		options := make([]huh.Option[string], 0, len(items))
		for _, workspaceItem := range items {
			options = append(options, huh.NewOption(workspaceItem.ID+" · "+workspaceItem.Path, workspaceItem.ID))
		}
		page.form = component.NewForm(component.Group(component.MultiSelect("Container workspaces", &page.members, options...)))
		page.overlay = workspaceOverlayForm
	case WorkspaceUnregister, WorkspaceContainerDelete:
		if command == WorkspaceUnregister {
			if _, err := page.manager.Get(page.targetID); err != nil {
				return err
			}
		} else if _, err := page.manager.GetContainer(page.targetID); err != nil {
			return err
		}
		page.confirm = component.NewConfirmButtons("Delete", "Cancel", false)
		page.overlay = workspaceOverlayConfirm
	default:
		return fmt.Errorf("unsupported workspace action: %s", command)
	}
	return nil
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
			if page.kind == workspacePageContainers {
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
	var rows []component.Row
	var err error
	if page.kind == workspacePageContainers {
		rows, err = page.containerRows()
	} else {
		rows, err = page.workspaceRows()
	}
	if err != nil {
		return err
	}
	title := "Workspaces"
	if page.kind == workspacePageContainers {
		title = "Workspace containers"
	}
	refresh := func(context.Context) ([]component.Row, error) {
		if page.kind == workspacePageContainers {
			return page.containerRows()
		}
		return page.workspaceRows()
	}
	page.browser = component.NewBrowser(page.ctx, title, rows, refresh).WithAction(component.RowAction{Key: "c", Desc: "copy ID", Run: func(row component.Row) (string, tea.Cmd, error) {
		if err := copyWorkspaceID(row.ID); err != nil {
			return "", nil, err
		}
		return "Copied " + row.ID, nil, nil
	}})
	if page.kind == workspacePageContainers {
		page.browser.SetHelpBindings(component.Binding([]string{"a"}, "a", "create"), component.Binding([]string{"e"}, "e", "rename"), component.Binding([]string{"m"}, "m", "members"), component.Binding([]string{"d"}, "d", "delete"))
	} else {
		page.browser.SetHelpBindings(component.Binding([]string{"a"}, "a", "register"), component.Binding([]string{"+"}, "+", "add access"), component.Binding([]string{"-"}, "-", "remove access"), component.Binding([]string{"d"}, "d", "unregister"))
	}
	if page.width > 0 && page.height > 0 {
		updated, _ := page.browser.Update(tea.WindowSizeMsg{Width: page.width, Height: page.height})
		page.browser = updated.(component.Browser)
	}
	if page.resourceID != "" {
		page.browser.OpenDetail(page.resourceID)
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
	page.command, page.targetID = "", ""
	page.value, page.members = "", nil
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
