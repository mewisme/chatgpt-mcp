package page

import (
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
	if err := page.openCommand(WorkspaceRegister, ""); err != nil {
		t.Fatal(err)
	}
	page.value = project
	page.submitForm()
	items, err := page.manager.List()
	if err != nil || len(items) != 1 {
		t.Fatalf("workspaces=%#v err=%v", items, err)
	}
	id := items[0].ID
	if err := page.openCommand(WorkspaceAccessAdd, id); err != nil {
		t.Fatal(err)
	}
	page.value = extra
	page.submitForm()
	item, err := page.manager.Get(id)
	if err != nil || len(item.AllowDirs) != 1 {
		t.Fatalf("workspace=%#v err=%v", item, err)
	}
	if err := page.openCommand(WorkspaceAccessRemove, id); err != nil {
		t.Fatal(err)
	}
	page.value = extra
	page.submitForm()
	item, _ = page.manager.Get(id)
	if len(item.AllowDirs) != 0 {
		t.Fatalf("allow dirs=%v", item.AllowDirs)
	}
	if err := page.openCommand(WorkspaceUnregister, id); err != nil {
		t.Fatal(err)
	}
	page.confirm = component.NewConfirmButtons("Delete", "Cancel", true)
	page.updateConfirm(tea.KeyPressMsg{Code: tea.KeyEnter})
	items, err = page.manager.List()
	if err != nil || len(items) != 0 {
		t.Fatalf("workspaces after unregister=%#v err=%v", items, err)
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
	if err := page.openCommand(WorkspaceContainerCreate, ""); err != nil {
		t.Fatal(err)
	}
	page.value = "Primary"
	page.submitForm()
	containers, err := page.manager.ListContainers()
	if err != nil || len(containers) != 1 {
		t.Fatalf("containers=%#v err=%v", containers, err)
	}
	id := containers[0].ID
	if err := page.openCommand(WorkspaceContainerRename, id); err != nil {
		t.Fatal(err)
	}
	page.value = "Renamed"
	page.submitForm()
	if err := page.openCommand(WorkspaceContainerMembers, id); err != nil {
		t.Fatal(err)
	}
	page.members = append([]string(nil), ids...)
	page.submitForm()
	container, err := page.manager.GetContainer(id)
	if err != nil || len(container.WorkspaceIDs) != 2 || container.Name != "Renamed" {
		t.Fatalf("container=%#v err=%v", container, err)
	}
	if err := page.openCommand(WorkspaceContainerDelete, id); err != nil {
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
	notice, help := strings.Index(plain, "Workspace updated"), strings.LastIndex(plain, "? more")
	if notice < 0 || help < 0 || notice >= help {
		t.Fatalf("feedback/help order invalid: notice=%d help=%d view=%q", notice, help, plain)
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
	containerPage, err := NewContainers(t.Context(), "")
	if err != nil {
		t.Fatal(err)
	}
	previous := copyWorkspaceID
	t.Cleanup(func() { copyWorkspaceID = previous })
	var copied string
	copyWorkspaceID = func(value string) error { copied = value; return nil }

	for _, test := range []struct {
		name string
		page *WorkspacePage
		id   string
	}{{"workspace", workspacePage, workspaceItem.ID}, {"container", containerPage, container.ID}} {
		t.Run(test.name, func(t *testing.T) {
			copied = ""
			if !test.page.browser.SelectID(test.id) {
				t.Fatalf("could not select %s", test.id)
			}
			updated, _ := test.page.Update(tea.KeyPressMsg{Code: 'c', Text: "c"})
			test.page = updated.(*WorkspacePage)
			if copied != test.id {
				t.Fatalf("copied=%q want=%q", copied, test.id)
			}
		})
	}
}
