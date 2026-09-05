package cli

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"go.mewis.me/chatgpt-mcp/internal/cli/interactive"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

func TestWorkspaceInteractiveContainerFlow(t *testing.T) {
	manager := workspace.NewManager(t.TempDir() + "/workspaces.json")
	item, err := manager.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	existing, err := manager.CreateContainer("Existing")
	if err != nil {
		t.Fatal(err)
	}
	available, err := manager.CreateContainer("Available")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.AddWorkspaceToContainer(existing.ID, item.ID); err != nil {
		t.Fatal(err)
	}
	model := newWorkspaceInteractiveModel(manager, []workspace.Workspace{item})
	updated, _ := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(workspaceInteractiveModel)
	if model.mode != workspaceModeMenu {
		t.Fatalf("mode = %v", model.mode)
	}
	model.menuCursor = 1
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(workspaceInteractiveModel)
	if model.mode != workspaceModeSelectAdd || len(model.containers) != 2 || !model.memberIDs[existing.ID] {
		t.Fatalf("selector state = mode=%v containers=%#v members=%#v", model.mode, model.containers, model.memberIDs)
	}
	for index, value := range model.containers {
		if value.ID == available.ID {
			model.selectorCursor = index
		}
	}
	updated, _ = model.Update(tea.KeyPressMsg{Text: " ", Code: ' '})
	model = updated.(workspaceInteractiveModel)
	if !model.selectedIDs[available.ID] {
		t.Fatal("available container was not selected")
	}
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(workspaceInteractiveModel)
	if model.mode != workspaceModeConfirm || model.pending.action != "add" {
		t.Fatalf("confirm state = %#v", model.pending)
	}
	model.confirm = interactiveAffirmativeConfirm("Add")
	updated, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	model = updated.(workspaceInteractiveModel)
	members, err := manager.ContainersForWorkspace(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 2 {
		t.Fatalf("members = %#v", members)
	}
}

func interactiveAffirmativeConfirm(label string) interactive.ConfirmButtons {
	return interactive.NewConfirmButtons(label, "Cancel", true)
}
