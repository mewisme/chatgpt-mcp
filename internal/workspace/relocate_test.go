package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/workspacestate"
)

func TestRelocatePreservesIdentityContainersAndLocalState(t *testing.T) {
	manager := NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	oldRoot := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(filepath.Join(oldRoot, "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	item, err := manager.Register(oldRoot)
	if err != nil {
		t.Fatal(err)
	}
	container, err := manager.CreateContainer("group")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.AddWorkspaceToContainer(container.ID, item.ID); err != nil {
		t.Fatal(err)
	}
	local, err := manager.LocalState(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(local.StateRoot(), 0700); err != nil {
		t.Fatal(err)
	}
	statePath := local.StatePath("shell" + configformat.ExtensionForRoot(t.TempDir()))
	data, err := configformat.MarshalPath(statePath, map[string]any{"workspace_id": item.ID, "cwd": filepath.Join(oldRoot, "nested")})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, data, 0600); err != nil {
		t.Fatal(err)
	}
	newRoot := filepath.Join(t.TempDir(), "moved")
	if err := os.Rename(oldRoot, newRoot); err != nil {
		t.Fatal(err)
	}
	relocated, err := manager.Relocate(item.ID, newRoot)
	if err != nil {
		t.Fatal(err)
	}
	if relocated.ID != item.ID || relocated.Path != canonicalRoot(newRoot) || len(relocated.LegacyIDs) != 0 {
		t.Fatalf("relocated=%#v", relocated)
	}
	if resolved, err := manager.Get(item.ID); err != nil || resolved.ID != item.ID || resolved.Path != canonicalRoot(newRoot) {
		t.Fatalf("resolved=%#v err=%v", resolved, err)
	}
	updatedContainer, err := manager.GetContainer(container.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(updatedContainer.WorkspaceIDs, []string{item.ID}) {
		t.Fatalf("container members=%#v", updatedContainer.WorkspaceIDs)
	}
	movedStatePath := filepath.Join(newRoot, ".cgm", "state", filepath.Base(statePath))
	encoded, err := os.ReadFile(movedStatePath)
	if err != nil {
		t.Fatal(err)
	}
	format, err := configformat.Detect(movedStatePath)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := configformat.DecodeGeneric(format, encoded)
	if err != nil {
		t.Fatal(err)
	}
	stateMap := decoded.(map[string]any)
	if stateMap["workspace_id"] != item.ID || stateMap["cwd"] != filepath.Join(canonicalRoot(newRoot), "nested") {
		t.Fatalf("state=%#v", stateMap)
	}
}

func TestActiveRelocatePreservesRuntimeLockOwnership(t *testing.T) {
	t.Setenv("CHATGPT_MCP_CONFIG_DIR", t.TempDir())
	manager := NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	oldRoot := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(oldRoot, 0755); err != nil {
		t.Fatal(err)
	}
	item, err := manager.Register(oldRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Activate(); err != nil {
		t.Fatal(err)
	}
	defer manager.Deactivate()
	newRoot := filepath.Join(t.TempDir(), "moved")
	if err := os.Rename(oldRoot, newRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Relocate(item.ID, newRoot); err != nil {
		t.Fatal(err)
	}
	contender := NewManager(filepath.Join(t.TempDir(), "contender.json"))
	if err := contender.Activate(); err != nil {
		t.Fatal(err)
	}
	defer contender.Deactivate()
	if _, err := contender.Register(newRoot); !errors.Is(err, ErrAlreadyActive) {
		t.Fatalf("register relocated active workspace error=%v", err)
	}
}

func TestActiveRelocateRejectsCopiedWorkspaceState(t *testing.T) {
	t.Setenv("CHATGPT_MCP_CONFIG_DIR", t.TempDir())
	manager := NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	oldRoot := t.TempDir()
	item, err := manager.Register(oldRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Activate(); err != nil {
		t.Fatal(err)
	}
	defer manager.Deactivate()
	target := t.TempDir()
	if err := copyDirectoryForRelocateTest(workspacestate.New(oldRoot).Root(), workspacestate.New(target).Root()); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Relocate(item.ID, target); err == nil {
		t.Fatal("expected active relocation to copied workspace state to fail")
	}
}

func TestFreshManagerRelocatesWorkspaceAfterExternalMove(t *testing.T) {
	t.Setenv("CHATGPT_MCP_CONFIG_DIR", t.TempDir())
	store := filepath.Join(t.TempDir(), "workspaces.json")
	oldRoot := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(oldRoot, 0755); err != nil {
		t.Fatal(err)
	}
	item, err := NewManager(store).Register(oldRoot)
	if err != nil {
		t.Fatal(err)
	}
	newRoot := filepath.Join(t.TempDir(), "moved")
	if err := os.Rename(oldRoot, newRoot); err != nil {
		t.Fatal(err)
	}
	fresh := NewManager(store)
	relocated, err := fresh.Relocate(item.ID, newRoot)
	if err != nil {
		t.Fatal(err)
	}
	if relocated.ID != item.ID || relocated.Path != canonicalRoot(newRoot) {
		t.Fatalf("relocated=%#v", relocated)
	}
	resolved, err := fresh.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Path != canonicalRoot(newRoot) {
		t.Fatalf("resolved=%#v", resolved)
	}
}

func TestStandaloneRelocateRejectsWorkspaceOwnedByActiveRuntime(t *testing.T) {
	t.Setenv("CHATGPT_MCP_CONFIG_DIR", t.TempDir())
	store := filepath.Join(t.TempDir(), "workspaces.json")
	owner := NewManager(store)
	item, err := owner.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Activate(); err != nil {
		t.Fatal(err)
	}
	defer owner.Deactivate()
	if _, err := NewManager(store).Relocate(item.ID, item.Path); !errors.Is(err, ErrAlreadyActive) {
		t.Fatalf("standalone relocate error=%v", err)
	}
}

func TestInactiveRelocateRejectsCopiedWorkspaceState(t *testing.T) {
	t.Setenv("CHATGPT_MCP_CONFIG_DIR", t.TempDir())
	manager := NewManager(filepath.Join(t.TempDir(), "workspaces.json"))
	oldRoot := t.TempDir()
	item, err := manager.Register(oldRoot)
	if err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	if err := copyDirectoryForRelocateTest(workspacestate.New(oldRoot).Root(), workspacestate.New(target).Root()); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Relocate(item.ID, target); err == nil {
		t.Fatal("expected copied workspace identity to reject inactive relocate")
	}
}

func copyDirectoryForRelocateTest(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0700)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0600)
	})
}

func TestRelocateRejectsRegisteredDestination(t *testing.T) {
	manager := newTestManager(t)
	first, err := manager.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Relocate(first.ID, second.Path); err == nil {
		t.Fatal("expected registered destination to be rejected")
	}
}

func TestRelocateRejectsMismatchedLocalIdentity(t *testing.T) {
	manager := newTestManager(t)
	item, err := manager.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	if _, _, err := workspacestate.New(target).EnsureIdentity("ws_other1234567890"); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Relocate(item.ID, target); err == nil {
		t.Fatal("expected mismatched local identity to be rejected")
	}
}

func TestRelocateRewritesAllowDirsUnderOldRoot(t *testing.T) {
	manager := newTestManager(t)
	oldRoot := filepath.Join(t.TempDir(), "workspace")
	nested := filepath.Join(oldRoot, "generated")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	item, err := manager.Register(oldRoot)
	if err != nil {
		t.Fatal(err)
	}
	item, err = manager.AddAllowDir(item.ID, nested)
	if err != nil {
		t.Fatal(err)
	}
	newRoot := filepath.Join(t.TempDir(), "moved")
	if err := os.Rename(oldRoot, newRoot); err != nil {
		t.Fatal(err)
	}
	relocated, err := manager.Relocate(item.ID, newRoot)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(canonicalRoot(newRoot), "generated")
	if !reflect.DeepEqual(relocated.AllowDirs, []string{want}) {
		t.Fatalf("allow dirs=%#v want=%#v", relocated.AllowDirs, []string{want})
	}
	config, err := workspacestate.New(newRoot).LoadConfig()
	if err != nil || !reflect.DeepEqual(config.AllowDirs, []string{want}) {
		t.Fatalf("local config=%#v err=%v", config, err)
	}
}

func TestRelocateRejectsMissingLocalState(t *testing.T) {
	manager := newTestManager(t)
	item, err := manager.Register(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Relocate(item.ID, t.TempDir()); err == nil {
		t.Fatal("expected destination without .cgm identity to be rejected")
	}
}

func TestRelocateSkipsMalformedCheckpointManifest(t *testing.T) {
	manager := newTestManager(t)
	oldRoot := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(oldRoot, 0755); err != nil {
		t.Fatal(err)
	}
	item, err := manager.Register(oldRoot)
	if err != nil {
		t.Fatal(err)
	}
	local, err := manager.LocalState(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	manifestDir := filepath.Join(local.CheckpointRoot(), "data", "cp_broken")
	if err := os.MkdirAll(manifestDir, 0700); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(manifestDir, "manifest"+configformat.ExtensionForRoot(t.TempDir()))
	broken := []byte("version: 1\nfiles:\n  - content: first\n      broken: value\n")
	if err := os.WriteFile(manifestPath, broken, 0600); err != nil {
		t.Fatal(err)
	}
	newRoot := filepath.Join(t.TempDir(), "moved")
	if err := os.Rename(oldRoot, newRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Relocate(item.ID, newRoot); err != nil {
		t.Fatal(err)
	}
	movedManifest := filepath.Join(newRoot, ".cgm", "checkpoints", "data", "cp_broken", filepath.Base(manifestPath))
	got, err := os.ReadFile(movedManifest)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, broken) {
		t.Fatalf("malformed checkpoint manifest changed during relocate:\n%s", got)
	}
}

func TestRelocateRejectsMalformedCoreState(t *testing.T) {
	manager := newTestManager(t)
	oldRoot := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(oldRoot, 0755); err != nil {
		t.Fatal(err)
	}
	item, err := manager.Register(oldRoot)
	if err != nil {
		t.Fatal(err)
	}
	local, err := manager.LocalState(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(local.StateRoot(), 0700); err != nil {
		t.Fatal(err)
	}
	statePath := local.StatePath("shell" + configformat.ExtensionForRoot(t.TempDir()))
	if err := os.WriteFile(statePath, []byte("cwd: first\n  broken: value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	newRoot := filepath.Join(t.TempDir(), "moved")
	if err := os.Rename(oldRoot, newRoot); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Relocate(item.ID, newRoot); err == nil {
		t.Fatal("expected malformed core workspace state to reject relocate")
	}
}
