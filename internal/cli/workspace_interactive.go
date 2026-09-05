package cli

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"
	"go.mewis.me/chatgpt-mcp/internal/cli/interactive"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

type workspaceInteractiveMode uint8

const (
	workspaceModeList workspaceInteractiveMode = iota
	workspaceModeMenu
	workspaceModeDetail
	workspaceModeCreate
	workspaceModeSelectAdd
	workspaceModeSelectRemove
	workspaceModeConfirm
)

type workspaceInteractiveItem struct{ workspace workspace.Workspace }

func (i workspaceInteractiveItem) Title() string       { return i.workspace.ID }
func (i workspaceInteractiveItem) Description() string { return i.workspace.Path }
func (i workspaceInteractiveItem) FilterValue() string {
	return strings.Join([]string{i.workspace.ID, i.workspace.Path, strings.Join(i.workspace.AllowDirs, " ")}, " ")
}

type workspacePendingAction struct {
	action       string
	workspaceID  string
	containerIDs []string
	name         string
}

type workspaceInteractiveModel struct {
	manager        *workspace.Manager
	list           list.Model
	mode           workspaceInteractiveMode
	workspace      workspace.Workspace
	containers     []workspace.WorkspaceContainer
	memberIDs      map[string]bool
	selectedIDs    map[string]bool
	menuCursor     int
	selectorCursor int
	input          textinput.Model
	confirm        interactive.ConfirmButtons
	pending        workspacePendingAction
	width          int
	height         int
	notice         string
	err            error
}

var workspaceMenuItems = []string{"View details", "Select workspace container", "Remove workspace container", "Copy workspace ID", "Unregister workspace"}

func runWorkspaceInteractive(cmd *cobra.Command, manager *workspace.Manager, items []workspace.Workspace) error {
	model := newWorkspaceInteractiveModel(manager, items)
	_, err := interactive.Run(cmd.Context(), model, cmd.InOrStdin(), cmd.OutOrStdout())
	return err
}

func newWorkspaceInteractiveModel(manager *workspace.Manager, items []workspace.Workspace) workspaceInteractiveModel {
	values := workspaceInteractiveItems(items)
	model := interactive.NewDefaultList("Registered workspaces", values, 80, 20, "workspace", "workspaces")
	model.SetShowStatusBar(len(values) > 0)
	model.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{interactive.Binding([]string{"enter"}, "enter", "menu"), interactive.Binding([]string{"n"}, "n", "new container")}
	}
	model.AdditionalFullHelpKeys = model.AdditionalShortHelpKeys
	input := textinput.New()
	input.Placeholder = "Container name"
	input.Prompt = "> "
	input.CharLimit = 120
	return workspaceInteractiveModel{manager: manager, list: model, input: input, memberIDs: map[string]bool{}, selectedIDs: map[string]bool{}}
}

func workspaceInteractiveItems(items []workspace.Workspace) []list.Item {
	values := make([]list.Item, 0, len(items))
	for _, item := range items {
		values = append(values, workspaceInteractiveItem{workspace: item})
	}
	return values
}

func (m workspaceInteractiveModel) Init() tea.Cmd { return nil }

func (m workspaceInteractiveModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.BackgroundColorMsg:
		interactive.ApplyDefaultListTheme(&m.list, msg.IsDark())
		m.confirm.Update(msg)
		return m, nil
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.list.SetSize(msg.Width, msg.Height)
		m.input.SetWidth(max(24, min(60, msg.Width-14)))
		m.confirm.SetWidth(max(24, min(54, msg.Width-16)))
		return m, nil
	case tea.KeyPressMsg:
		return m.handleWorkspaceKey(msg)
	}
	if m.mode == workspaceModeCreate {
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(message)
		return m, cmd
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(message)
	return m, cmd
}

func (m workspaceInteractiveModel) handleWorkspaceKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	switch m.mode {
	case workspaceModeMenu:
		return m.handleWorkspaceMenuKey(msg)
	case workspaceModeDetail:
		if msg.String() == "esc" || msg.String() == "q" || msg.String() == "enter" {
			m.mode = workspaceModeMenu
		}
		return m, nil
	case workspaceModeCreate:
		if msg.String() == "esc" {
			m.mode = workspaceModeList
			m.input.Blur()
			return m, nil
		}
		if msg.String() == "enter" {
			name := strings.TrimSpace(m.input.Value())
			if name == "" {
				m.err = fmt.Errorf("container name is required")
				return m, nil
			}
			m.startWorkspaceConfirmation(workspacePendingAction{action: "create", name: name})
			return m, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	case workspaceModeSelectAdd, workspaceModeSelectRemove:
		return m.handleWorkspaceSelectorKey(msg)
	case workspaceModeConfirm:
		return m.handleWorkspaceConfirmKey(msg)
	default:
		if m.list.FilterState() == list.Filtering {
			var cmd tea.Cmd
			m.list, cmd = m.list.Update(msg)
			return m, cmd
		}
		if msg.String() == "n" {
			m.err, m.notice = nil, ""
			m.mode = workspaceModeCreate
			m.input.Reset()
			return m, m.input.Focus()
		}
		if msg.String() == "enter" {
			if item, ok := m.list.SelectedItem().(workspaceInteractiveItem); ok {
				m.workspace = item.workspace
				m.mode = workspaceModeMenu
				m.menuCursor = 0
				m.err, m.notice = nil, ""
			}
			return m, nil
		}
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}
}

func (m workspaceInteractiveModel) handleWorkspaceMenuKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.mode = workspaceModeList
	case "up", "k":
		if m.menuCursor > 0 {
			m.menuCursor--
		}
	case "down", "j":
		if m.menuCursor < len(workspaceMenuItems)-1 {
			m.menuCursor++
		}
	case "enter":
		switch m.menuCursor {
		case 0:
			m.mode = workspaceModeDetail
		case 1:
			if err := m.loadWorkspaceContainers(true); err != nil {
				m.err = err
			} else {
				m.mode = workspaceModeSelectAdd
			}
		case 2:
			if err := m.loadWorkspaceContainers(false); err != nil {
				m.err = err
			} else {
				m.mode = workspaceModeSelectRemove
			}
		case 3:
			m.notice = "Copied " + m.workspace.ID
			return m, tea.SetClipboard(m.workspace.ID)
		case 4:
			m.startWorkspaceConfirmation(workspacePendingAction{action: "unregister", workspaceID: m.workspace.ID})
		}
	}
	return m, nil
}

func (m *workspaceInteractiveModel) loadWorkspaceContainers(add bool) error {
	values, err := m.manager.ListContainers()
	if err != nil {
		return err
	}
	members, err := m.manager.ContainersForWorkspace(m.workspace.ID)
	if err != nil {
		return err
	}
	m.memberIDs = map[string]bool{}
	for _, value := range members {
		m.memberIDs[value.ID] = true
	}
	if add {
		m.containers = values
	} else {
		m.containers = append([]workspace.WorkspaceContainer(nil), members...)
	}
	m.selectedIDs = map[string]bool{}
	m.selectorCursor = 0
	m.err, m.notice = nil, ""
	return nil
}

func (m workspaceInteractiveModel) handleWorkspaceSelectorKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.mode = workspaceModeMenu
	case "up", "k":
		if m.selectorCursor > 0 {
			m.selectorCursor--
		}
	case "down", "j":
		if m.selectorCursor < len(m.containers)-1 {
			m.selectorCursor++
		}
	case "space":
		if len(m.containers) == 0 {
			return m, nil
		}
		item := m.containers[m.selectorCursor]
		if m.mode == workspaceModeSelectAdd && m.memberIDs[item.ID] {
			return m, nil
		}
		m.selectedIDs[item.ID] = !m.selectedIDs[item.ID]
	case "enter":
		ids := m.workspaceSelectedContainerIDs()
		if len(ids) == 0 {
			m.err = fmt.Errorf("select at least one workspace container")
			return m, nil
		}
		action := "add"
		if m.mode == workspaceModeSelectRemove {
			action = "remove"
		}
		m.startWorkspaceConfirmation(workspacePendingAction{action: action, workspaceID: m.workspace.ID, containerIDs: ids})
	}
	return m, nil
}

func (m workspaceInteractiveModel) workspaceSelectedContainerIDs() []string {
	ids := []string{}
	for _, value := range m.containers {
		if m.selectedIDs[value.ID] {
			ids = append(ids, value.ID)
		}
	}
	return ids
}

func (m *workspaceInteractiveModel) startWorkspaceConfirmation(pending workspacePendingAction) {
	m.pending = pending
	m.mode = workspaceModeConfirm
	m.confirm = interactive.NewConfirmButtons(workspaceConfirmLabel(pending.action), "Cancel", false)
	m.confirm.SetWidth(max(24, min(54, m.width-16)))
}

func (m workspaceInteractiveModel) handleWorkspaceConfirmKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "n", "N":
		m.cancelWorkspaceConfirmation()
		return m, nil
	case "h", "l", "left", "right", "tab", "shift+tab":
		return m, m.confirm.Update(msg)
	case "y", "Y":
		m.confirm = interactive.NewConfirmButtons(workspaceConfirmLabel(m.pending.action), "Cancel", true)
	case "enter":
		if !m.confirm.AffirmativeSelected() {
			m.cancelWorkspaceConfirmation()
			return m, nil
		}
		return m.executeWorkspacePending()
	}
	return m, nil
}

func (m *workspaceInteractiveModel) cancelWorkspaceConfirmation() {
	switch m.pending.action {
	case "create":
		m.mode = workspaceModeCreate
	case "add":
		m.mode = workspaceModeSelectAdd
	case "remove":
		m.mode = workspaceModeSelectRemove
	default:
		m.mode = workspaceModeMenu
	}
	m.pending = workspacePendingAction{}
}

func (m workspaceInteractiveModel) executeWorkspacePending() (tea.Model, tea.Cmd) {
	pending := m.pending
	m.pending = workspacePendingAction{}
	var err error
	switch pending.action {
	case "create":
		var value workspace.WorkspaceContainer
		value, err = m.manager.CreateContainer(pending.name)
		if err == nil {
			m.notice = "Created " + value.Name + " · " + value.ID
			m.input.Reset()
			m.input.Blur()
			m.mode = workspaceModeList
		}
	case "add":
		_, err = m.manager.AddWorkspaceToContainers(pending.workspaceID, pending.containerIDs)
		if err == nil {
			m.notice = fmt.Sprintf("Added %s to %d container(s)", pending.workspaceID, len(pending.containerIDs))
			m.mode = workspaceModeMenu
		}
	case "remove":
		_, err = m.manager.RemoveWorkspaceFromContainers(pending.workspaceID, pending.containerIDs)
		if err == nil {
			m.notice = fmt.Sprintf("Removed %s from %d container(s)", pending.workspaceID, len(pending.containerIDs))
			m.mode = workspaceModeMenu
		}
	case "unregister":
		err = m.manager.Unregister(pending.workspaceID)
		if err == nil {
			m.notice = "Unregistered " + pending.workspaceID
			m.mode = workspaceModeList
			m.workspace = workspace.Workspace{}
		}
	}
	if err != nil {
		m.err = err
		m.mode = workspaceModeMenu
		return m, m.list.NewStatusMessage(err.Error())
	}
	m.err = nil
	items, listErr := m.manager.List()
	if listErr != nil {
		m.err = listErr
		return m, nil
	}
	cmd := m.list.SetItems(workspaceInteractiveItems(items))
	m.list.SetShowStatusBar(len(items) > 0)
	status := m.list.NewStatusMessage(m.notice)
	return m, tea.Batch(cmd, status)
}

func workspaceConfirmLabel(action string) string {
	switch action {
	case "create":
		return "Create"
	case "add":
		return "Add"
	case "remove":
		return "Remove"
	case "unregister":
		return "Unregister"
	default:
		return "Confirm"
	}
}

func (m workspaceInteractiveModel) View() tea.View {
	background := m.list.View()
	var dialog string
	switch m.mode {
	case workspaceModeMenu:
		dialog = m.workspaceMenuView()
	case workspaceModeDetail:
		dialog = m.workspaceDetailView()
	case workspaceModeCreate:
		dialog = m.workspaceCreateView()
	case workspaceModeSelectAdd, workspaceModeSelectRemove:
		dialog = m.workspaceSelectorView()
	case workspaceModeConfirm:
		dialog = m.workspaceConfirmView()
	}
	if dialog != "" {
		background = interactive.CenterOverlay(background, dialog, m.width, m.height)
	}
	view := tea.NewView(background)
	view.AltScreen = true
	return view
}

func (m workspaceInteractiveModel) workspaceMenuView() string {
	var builder strings.Builder
	builder.WriteString(interactive.Title("Workspace · " + m.workspace.ID))
	builder.WriteString("\n")
	builder.WriteString(interactive.Muted(m.workspace.Path))
	if m.notice != "" {
		builder.WriteString("\n\n")
		builder.WriteString(interactive.Banner(m.notice, interactive.ToneSuccess))
	}
	if m.err != nil {
		builder.WriteString("\n\n")
		builder.WriteString(interactive.Banner(m.err.Error(), interactive.ToneDanger))
	}
	builder.WriteString("\n\n")
	for index, item := range workspaceMenuItems {
		prefix := "  "
		if index == m.menuCursor {
			prefix = "> "
		}
		builder.WriteString(prefix + item + "\n")
	}
	builder.WriteString("\n")
	builder.WriteString(interactive.DefaultHelp(m.modalContentWidth(), interactive.Binding([]string{"j", "k", "up", "down"}, "j/k", "move"), interactive.Binding([]string{"enter"}, "enter", "select"), interactive.Binding([]string{"esc", "q"}, "esc/q", "close")))
	return interactive.Modal(strings.TrimSuffix(builder.String(), "\n"), m.modalWidth())
}

func (m workspaceInteractiveModel) workspaceDetailView() string {
	containers, _ := m.manager.ContainersForWorkspace(m.workspace.ID)
	containerNames := make([]string, 0, len(containers))
	for _, value := range containers {
		containerNames = append(containerNames, value.Name+" ("+value.ID+")")
	}
	if len(containerNames) == 0 {
		containerNames = []string{"None"}
	}
	body := interactive.Title("Workspace details") + "\n\n" + interactive.KeyValue("ID", m.workspace.ID) + "\n" + interactive.KeyValue("Path", m.workspace.Path) + "\n" + interactive.KeyValue("Containers", strings.Join(containerNames, ", ")) + "\n\n" + interactive.DefaultHelp(m.modalContentWidth(), interactive.Binding([]string{"esc", "q", "enter"}, "esc", "back"))
	return interactive.Modal(body, m.modalWidth())
}

func (m workspaceInteractiveModel) workspaceCreateView() string {
	body := interactive.Title("Create workspace container") + "\n\n" + m.input.View()
	if m.err != nil {
		body += "\n\n" + interactive.Banner(m.err.Error(), interactive.ToneDanger)
	}
	body += "\n\n" + interactive.DefaultHelp(m.modalContentWidth(), interactive.Binding([]string{"enter"}, "enter", "continue"), interactive.Binding([]string{"esc"}, "esc", "cancel"))
	return interactive.Modal(body, m.modalWidth())
}

func (m workspaceInteractiveModel) workspaceSelectorView() string {
	title := "Add workspace to containers"
	if m.mode == workspaceModeSelectRemove {
		title = "Remove workspace from containers"
	}
	var builder strings.Builder
	builder.WriteString(interactive.Title(title))
	builder.WriteString("\n")
	builder.WriteString(interactive.Muted(m.workspace.ID))
	if m.err != nil {
		builder.WriteString("\n\n")
		builder.WriteString(interactive.Banner(m.err.Error(), interactive.ToneDanger))
	}
	builder.WriteString("\n\n")
	if len(m.containers) == 0 {
		if m.mode == workspaceModeSelectRemove {
			builder.WriteString(interactive.Muted("This workspace is not assigned to any container."))
		} else {
			builder.WriteString(interactive.Muted("No workspace containers exist yet."))
		}
	} else {
		for index, value := range m.containers {
			checked := m.selectedIDs[value.ID]
			disabled := m.mode == workspaceModeSelectAdd && m.memberIDs[value.ID]
			mark := "[ ]"
			if checked || disabled {
				mark = "[x]"
			}
			prefix := "  "
			if index == m.selectorCursor {
				prefix = "> "
			}
			line := fmt.Sprintf("%s%s %s", prefix, mark, value.Name)
			if disabled {
				line = interactive.Muted(line + " · already added")
			}
			builder.WriteString(line + "\n")
		}
	}
	builder.WriteString("\n")
	builder.WriteString(interactive.DefaultHelp(m.modalContentWidth(), interactive.Binding([]string{"j", "k", "up", "down"}, "j/k", "move"), interactive.Binding([]string{" "}, "space", "toggle"), interactive.Binding([]string{"enter"}, "enter", "continue"), interactive.Binding([]string{"esc"}, "esc", "back")))
	return interactive.Modal(strings.TrimSuffix(builder.String(), "\n"), m.modalWidth())
}

func (m workspaceInteractiveModel) workspaceConfirmView() string {
	var description string
	switch m.pending.action {
	case "create":
		description = fmt.Sprintf("Create workspace container %q?", m.pending.name)
	case "add":
		description = fmt.Sprintf("Add %s to %d container(s)?", m.pending.workspaceID, len(m.pending.containerIDs))
	case "remove":
		description = fmt.Sprintf("Remove %s from %d container(s)?", m.pending.workspaceID, len(m.pending.containerIDs))
	case "unregister":
		description = fmt.Sprintf("Unregister %s? Project files will not be deleted.", m.pending.workspaceID)
	}
	body := interactive.Title(workspaceConfirmLabel(m.pending.action)+"?") + "\n\n" + description + "\n\n" + m.confirm.View() + "\n\n" + interactive.DefaultHelp(m.modalContentWidth(), interactive.Binding([]string{"left", "right", "tab"}, "←/→", "choose"), interactive.Binding([]string{"enter"}, "enter", "select"), interactive.Binding([]string{"esc", "n"}, "esc", "cancel"))
	return interactive.Modal(body, max(38, min(62, m.modalWidth())))
}

func (m workspaceInteractiveModel) modalWidth() int {
	width := m.width
	if width <= 0 {
		width = 80
	}
	return max(44, min(82, width-8))
}

func (m workspaceInteractiveModel) modalContentWidth() int { return max(32, m.modalWidth()-6) }
