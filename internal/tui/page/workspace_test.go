package page

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/tui/component"
)

func TestWorkspacePageLifecycle(t *testing.T) {
	defer configformat.SetRootPath("")
	if err := configformat.SetRootPath(filepath.Join(t.TempDir(), "config")); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(t.TempDir(), "project")
	extra := filepath.Join(t.TempDir(), "extra")
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(extra, 0700); err != nil {
		t.Fatal(err)
	}
	page, err := NewWorkspaces(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.openCommand(WorkspaceRegister, ""); err != nil {
		t.Fatal(err)
	}
	page.value = project
	page.submitForm()
	items, err := page.manager.List()
	if err != nil || len(items) != 1 {
		t.Fatalf("workspaces=%#v err=%v", items, err)
	}
	id := items[0].ID
	if _, err := page.openCommand(WorkspaceAccessAdd, id); err != nil {
		t.Fatal(err)
	}
	page.value = extra
	page.submitForm()
	item, err := page.manager.Get(id)
	if err != nil || len(item.AllowDirs) != 1 {
		t.Fatalf("workspace=%#v err=%v", item, err)
	}
	if _, err := page.openCommand(WorkspaceAccessRemove, id); err != nil {
		t.Fatal(err)
	}
	page.value = extra
	page.submitForm()
	item, _ = page.manager.Get(id)
	if len(item.AllowDirs) != 0 {
		t.Fatalf("allow dirs=%v", item.AllowDirs)
	}
	if _, err := page.openCommand(WorkspaceUnregister, id); err != nil {
		t.Fatal(err)
	}
	page.confirm = component.NewConfirmButtons("Delete", "Cancel", true)
	page.updateConfirm(tea.KeyPressMsg{Code: tea.KeyEnter})
	items, err = page.manager.List()
	if err != nil || len(items) != 0 {
		t.Fatalf("workspaces after unregister=%#v err=%v", items, err)
	}
}

func TestWorkspaceFormOpensInitializedFromKeyAndCommandMessage(t *testing.T) {
	defer configformat.SetRootPath("")
	if err := configformat.SetRootPath(filepath.Join(t.TempDir(), "config")); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		message tea.Msg
	}{
		{name: "keyboard", message: tea.KeyPressMsg{Code: 'a', Text: "a"}},
		{name: "command-message", message: WorkspaceCommandMsg{Command: WorkspaceRegister}},
	} {
		t.Run(test.name, func(t *testing.T) {
			page, err := NewWorkspaces(t.Context(), "")
			if err != nil {
				t.Fatal(err)
			}
			updated, cmd := page.Update(test.message)
			page = updated.(*WorkspacePage)
			if page.overlay != workspaceOverlayForm || cmd == nil {
				t.Fatalf("overlay=%d init=%v", page.overlay, cmd != nil)
			}
			page = runWorkspacePageCmd(t, page, cmd)
			plain := ansi.Strip(page.View(100, 24))
			if !strings.Contains(plain, "Workspace path") {
				t.Fatalf("initialized form field missing from first render: %q", plain)
			}
		})
	}
}

func TestWorkspaceContainerLifecycleAndMembership(t *testing.T) {
	defer configformat.SetRootPath("")
	if err := configformat.SetRootPath(filepath.Join(t.TempDir(), "config")); err != nil {
		t.Fatal(err)
	}
	managerPage, err := NewWorkspaces(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, 2)
	for _, name := range []string{"one", "two"} {
		path := filepath.Join(t.TempDir(), name)
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		item, err := managerPage.manager.Register(path)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, item.ID)
	}
	page, err := NewContainers(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.openCommand(WorkspaceContainerCreate, ""); err != nil {
		t.Fatal(err)
	}
	page.value = "Primary"
	page.submitForm()
	containers, err := page.manager.ListContainers()
	if err != nil || len(containers) != 1 {
		t.Fatalf("containers=%#v err=%v", containers, err)
	}
	id := containers[0].ID
	if _, err := page.openCommand(WorkspaceContainerRename, id); err != nil {
		t.Fatal(err)
	}
	page.value = "Renamed"
	page.submitForm()
	if _, err := page.openCommand(WorkspaceContainerMembers, id); err != nil {
		t.Fatal(err)
	}
	page.members = append([]string(nil), ids...)
	page.submitForm()
	container, err := page.manager.GetContainer(id)
	if err != nil || len(container.WorkspaceIDs) != 2 || container.Name != "Renamed" {
		t.Fatalf("container=%#v err=%v", container, err)
	}
	if _, err := page.openCommand(WorkspaceContainerDelete, id); err != nil {
		t.Fatal(err)
	}
	page.confirm = component.NewConfirmButtons("Delete", "Cancel", true)
	page.updateConfirm(tea.KeyPressMsg{Code: tea.KeyEnter})
	containers, err = page.manager.ListContainers()
	if err != nil || len(containers) != 0 {
		t.Fatalf("containers after delete=%#v err=%v", containers, err)
	}
	workspaces, err := page.manager.List()
	if err != nil || len(workspaces) != 2 {
		t.Fatalf("workspace records changed by container delete: %#v err=%v", workspaces, err)
	}
}

func TestWorkspaceDetailDeletionKeepsDetailUntilParentNavigation(t *testing.T) {
	defer configformat.SetRootPath("")
	if err := configformat.SetRootPath(filepath.Join(t.TempDir(), "config")); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}
	list, err := NewWorkspaces(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	item, err := list.manager.Register(project)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("workspace unregister", func(t *testing.T) {
		page, err := NewWorkspacesRoute(t.Context(), item.ID, "")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := page.openCommand(WorkspaceUnregister, item.ID); err != nil {
			t.Fatal(err)
		}
		page.confirm = component.NewConfirmButtons("Delete", "Cancel", true)
		cmd := page.updateConfirm(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd == nil || page.resourceID != item.ID {
			t.Fatalf("navigation=%v resource=%q", cmd != nil, page.resourceID)
		}
		if got := ansi.Strip(page.View(100, 24)); !strings.Contains(got, "Workspace · "+item.ID) {
			t.Fatalf("intermediate detail render=%q", got)
		}
		message, ok := cmd().(NavigateMsg)
		if !ok || strings.Join(message.Path, "/") != "workspaces" || !message.Replace {
			t.Fatalf("navigation=%#v", message)
		}
	})

	item, err = list.manager.Register(project)
	if err != nil {
		t.Fatal(err)
	}
	container, err := list.manager.CreateContainer("Primary")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := list.manager.AddWorkspaceToContainer(container.ID, item.ID); err != nil {
		t.Fatal(err)
	}
	page, err := NewContainersRoute(t.Context(), container.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.openCommand(WorkspaceContainerDelete, container.ID); err != nil {
		t.Fatal(err)
	}
	page.confirm = component.NewConfirmButtons("Delete", "Cancel", true)
	cmd := page.updateConfirm(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || page.resourceID != container.ID {
		t.Fatalf("navigation=%v resource=%q", cmd != nil, page.resourceID)
	}
	if got := ansi.Strip(page.View(100, 24)); !strings.Contains(got, "Container · Primary") {
		t.Fatalf("intermediate container detail render=%q", got)
	}
	message, ok := cmd().(NavigateMsg)
	if !ok || strings.Join(message.Path, "/") != "containers" || !message.Replace {
		t.Fatalf("navigation=%#v", message)
	}
	workspaces, err := page.manager.List()
	if err != nil || len(workspaces) != 1 || workspaces[0].ID != item.ID {
		t.Fatalf("workspace records changed by container delete: %#v err=%v", workspaces, err)
	}
}

func TestWorkspaceBrowserHelpStaysAboveAppFooterWithFeedback(t *testing.T) {
	defer configformat.SetRootPath("")
	if err := configformat.SetRootPath(filepath.Join(t.TempDir(), "config")); err != nil {
		t.Fatal(err)
	}
	page, err := NewWorkspaces(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	page.notice = "Workspace updated"
	plain := ansi.Strip(page.View(100, 24))
	lines := strings.Split(plain, "\n")
	last := len(lines) - 1
	for last >= 0 && strings.TrimSpace(lines[last]) == "" {
		last--
	}
	if last != 23 || !strings.Contains(lines[last], "? more") {
		t.Fatalf("workspace help line=%d want=23 view=%q", last, plain)
	}
	if !strings.Contains(lines[0], "Workspaces") || !strings.Contains(lines[0], "Containers") || !strings.Contains(lines[0], "Workspace updated") {
		t.Fatalf("workspace title/notice invalid: %q", lines[0])
	}
	if !strings.Contains(plain, "enter open") || !strings.Contains(plain, "←/→ tabs") || strings.Contains(plain, "c containers") {
		t.Fatalf("workspace list help invalid: %q", plain)
	}
	notice, help := strings.Index(plain, "Workspace updated"), strings.LastIndex(plain, "? more")
	if notice < 0 || help < 0 || notice >= help {
		t.Fatalf("feedback/help order invalid: notice=%d help=%d view=%q", notice, help, plain)
	}
}

func TestWorkspaceAndContainersRemainTabbedParentPages(t *testing.T) {
	defer configformat.SetRootPath("")
	if err := configformat.SetRootPath(filepath.Join(t.TempDir(), "config")); err != nil {
		t.Fatal(err)
	}
	page, err := NewWorkspaces(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	_, cmd := page.Update(tea.KeyPressMsg{Code: '2', Text: "2"})
	if cmd == nil {
		t.Fatal("containers tab navigation returned no command")
	}
	message, ok := cmd().(NavigateMsg)
	if !ok || strings.Join(message.Path, "/") != "containers" || !message.Replace {
		t.Fatalf("containers navigation=%#v", message)
	}
	containers, err := NewContainers(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	plain := ansi.Strip(containers.View(100, 24))
	if !strings.Contains(plain, "Workspaces") || !strings.Contains(plain, "Containers") || !strings.Contains(plain, "←/→ tabs") {
		t.Fatalf("containers tab page=%q", plain)
	}
	_, cmd = containers.Update(tea.KeyPressMsg{Code: '1', Text: "1"})
	if cmd == nil {
		t.Fatal("workspaces tab navigation returned no command")
	}
	message, ok = cmd().(NavigateMsg)
	if !ok || strings.Join(message.Path, "/") != "workspaces" || !message.Replace {
		t.Fatalf("workspaces navigation=%#v", message)
	}
}

func TestWorkspaceBrowserOpenNavigatesToResourceChild(t *testing.T) {
	defer configformat.SetRootPath("")
	if err := configformat.SetRootPath(filepath.Join(t.TempDir(), "config")); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}
	page, err := NewWorkspaces(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	item, err := page.manager.Register(project)
	if err != nil {
		t.Fatal(err)
	}
	if err := page.reload(); err != nil {
		t.Fatal(err)
	}
	_, cmd := page.Update(component.BrowserOpenMsg{Row: component.Row{ID: item.ID}})
	if cmd == nil {
		t.Fatal("resource open returned no navigation command")
	}
	message, ok := cmd().(NavigateMsg)
	if !ok || strings.Join(message.Path, "/") != "workspaces/"+item.ID {
		t.Fatalf("resource navigation=%#v", message)
	}
}

func TestWorkspaceDetailUsesFullChildPageAndNestedSections(t *testing.T) {
	defer configformat.SetRootPath("")
	if err := configformat.SetRootPath(filepath.Join(t.TempDir(), "config")); err != nil {
		t.Fatal(err)
	}
	project, extra := filepath.Join(t.TempDir(), "project"), filepath.Join(t.TempDir(), "extra")
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(extra, 0700); err != nil {
		t.Fatal(err)
	}
	list, err := NewWorkspaces(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	item, err := list.manager.Register(project)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := list.manager.AddAllowDir(item.ID, extra); err != nil {
		t.Fatal(err)
	}
	detail, err := NewWorkspacesRoute(t.Context(), item.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if detail.OverlayActive() {
		t.Fatal("resource detail incorrectly reports overlay active")
	}
	plain := ansi.Strip(detail.View(100, 24))
	if !strings.Contains(plain, "Workspace · "+item.ID) || !strings.Contains(plain, filepath.Base(item.Path)) || !strings.Contains(plain, "a access") || !strings.Contains(plain, "v containers") {
		t.Fatalf("workspace detail=%q", plain)
	}
	if strings.Contains(plain, "Overview   Access") || strings.Contains(plain, "╭") {
		t.Fatalf("workspace detail retained tab/modal chrome: %q", plain)
	}
	_, cmd := detail.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if cmd == nil {
		t.Fatal("access child navigation returned no command")
	}
	message, ok := cmd().(NavigateMsg)
	if !ok || strings.Join(message.Path, "/") != "workspaces/"+item.ID+"/access" {
		t.Fatalf("access navigation=%#v", message)
	}
	access, err := NewWorkspacesRoute(t.Context(), item.ID, "access")
	if err != nil {
		t.Fatal(err)
	}
	if got := ansi.Strip(access.View(100, 24)); !strings.Contains(got, filepath.Base(extra)) || strings.Contains(got, "a access") {
		t.Fatalf("workspace access child=%q", got)
	}
}

func TestContainerMembersPickerUsesCompactFilterableLayout(t *testing.T) {
	defer configformat.SetRootPath("")
	if err := configformat.SetRootPath(filepath.Join(t.TempDir(), "config")); err != nil {
		t.Fatal(err)
	}
	page, err := NewWorkspaces(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	ids := make([]string, 0, 24)
	for index := 0; index < 24; index++ {
		path := filepath.Join(parent, fmt.Sprintf("project-%02d", index))
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		item, err := page.manager.Register(path)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, item.ID)
	}
	container, err := page.manager.CreateContainer("Primary")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := page.manager.AddWorkspacesToContainer(container.ID, ids[:2]); err != nil {
		t.Fatal(err)
	}
	page, err = NewContainers(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	_ = page.View(120, 30)
	cmd, err := page.openCommand(WorkspaceContainerMembers, container.ID)
	if err != nil {
		t.Fatal(err)
	}
	page = runWorkspacePageCmd(t, page, cmd)
	plain := ansi.Strip(page.form.View())
	if !strings.Contains(plain, "2 selected / 24 available") {
		t.Fatalf("member count missing: %q", plain)
	}
	first, err := page.manager.Get(ids[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plain, filepath.Base(first.Path)+" · "+first.ID) {
		t.Fatalf("compact member label missing: %q", plain)
	}
	if strings.Contains(plain, parent+string(filepath.Separator)) {
		t.Fatalf("absolute workspace paths leaked into member options: %q", plain)
	}
	if got := page.formOverlayWidth(120); got != 94 {
		t.Fatalf("member modal width=%d want=94", got)
	}
	if got := workspaceMemberPickerHeight(100, 30); got != 16 {
		t.Fatalf("large member picker height=%d want=16", got)
	}
}

func TestWorkspaceAndContainerCopySelectedID(t *testing.T) {
	defer configformat.SetRootPath("")
	if err := configformat.SetRootPath(filepath.Join(t.TempDir(), "config")); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(project, 0700); err != nil {
		t.Fatal(err)
	}
	workspacePage, err := NewWorkspaces(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	workspaceItem, err := workspacePage.manager.Register(project)
	if err != nil {
		t.Fatal(err)
	}
	if err := workspacePage.reload(); err != nil {
		t.Fatal(err)
	}
	container, err := workspacePage.manager.CreateContainer("Primary")
	if err != nil {
		t.Fatal(err)
	}
	previous := copyWorkspaceID
	t.Cleanup(func() { copyWorkspaceID = previous })
	var copied string
	copyWorkspaceID = func(value string) error { copied = value; return nil }

	for _, test := range []struct {
		name string
		open func() (*WorkspacePage, error)
		id   string
	}{{"workspace", func() (*WorkspacePage, error) { return NewWorkspacesRoute(t.Context(), workspaceItem.ID, "") }, workspaceItem.ID}, {"container", func() (*WorkspacePage, error) { return NewContainersRoute(t.Context(), container.ID, "") }, container.ID}} {
		t.Run(test.name, func(t *testing.T) {
			copied = ""
			page, err := test.open()
			if err != nil {
				t.Fatal(err)
			}
			updated, cmd := page.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
			page = updated.(*WorkspacePage)
			_ = runWorkspacePageCmd(t, page, cmd)
			if copied != test.id {
				t.Fatalf("copied=%q want=%q", copied, test.id)
			}
		})
	}
}

func runWorkspacePageCmd(t *testing.T, page *WorkspacePage, cmd tea.Cmd) *WorkspacePage {
	t.Helper()
	if cmd == nil {
		return page
	}
	message := cmd()
	if batch, ok := message.(tea.BatchMsg); ok {
		for _, next := range batch {
			page = runWorkspacePageCmd(t, page, next)
		}
		return page
	}
	updated, next := page.Update(message)
	return runWorkspacePageCmd(t, updated.(*WorkspacePage), next)
}
