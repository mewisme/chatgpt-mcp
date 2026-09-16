package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/oslock"
	tracepkg "go.mewis.me/chatgpt-mcp/internal/trace"
	"go.mewis.me/chatgpt-mcp/internal/workspacestate"
)

func (m *Manager) Unregister(id string) (resultErr error) {
	span := tracepkg.StartObserver(m.trace, "WORKSPACE", "workspace.unregister", "Unregistering workspace", tracepkg.String("workspace_id", id))
	m.runtimeMu.Lock()
	defer m.runtimeMu.Unlock()
	transient, err := m.ensureRuntimeOwnershipLocked()
	if err != nil {
		span.FailMessage("Workspace unregistration failed", err)
		return err
	}
	if transient {
		defer func() { resultErr = errors.Join(resultErr, m.deactivateLocked()) }()
	}
	if err := m.ensureLoadedState(); err != nil {
		span.FailMessage("Workspace unregistration failed", err)
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	canonical := m.canonicalIDLocked(id)
	item, ok := m.items[canonical]
	if !ok {
		err := fmt.Errorf("%w: %s", ErrNotFound, id)
		span.FailMessage("Workspace unregistration failed", err)
		return err
	}
	delete(m.items, canonical)
	previousContainers := make(map[string]WorkspaceContainer)
	for containerID, container := range m.containers {
		if !containsString(container.WorkspaceIDs, canonical) {
			continue
		}
		previousContainers[containerID] = container
		container.WorkspaceIDs = removeStrings(container.WorkspaceIDs, []string{canonical})
		m.containers[containerID] = container
	}
	removedAliases := map[string]string{}
	for alias, target := range m.aliases {
		if target == canonical {
			removedAliases[alias] = target
			delete(m.aliases, alias)
		}
	}
	if err := m.saveLocked(); err != nil {
		m.items[canonical] = item
		for containerID, container := range previousContainers {
			m.containers[containerID] = container
		}
		for alias, target := range removedAliases {
			m.aliases[alias] = target
		}
		span.FailMessage("Workspace unregistration failed", err, tracepkg.String("canonical_workspace_id", canonical), tracepkg.Int("containers_updated", len(previousContainers)), tracepkg.Int("aliases_removed", len(removedAliases)))
		return err
	}
	if lock := m.releaseRuntimeLockLocked(canonical); lock != nil {
		if err := lock.Release(); err != nil {
			span.FailMessage("Workspace runtime lock release failed", err, tracepkg.String("canonical_workspace_id", canonical))
			return err
		}
	}
	span.EndMessage("Workspace unregistered", tracepkg.String("canonical_workspace_id", canonical), tracepkg.String("root", item.Path), tracepkg.Int("containers_updated", len(previousContainers)), tracepkg.Int("aliases_removed", len(removedAliases)), tracepkg.Bool("project_files_removed", false), tracepkg.Bool("local_state_removed", false))
	m.notifyUnregistered(canonical)
	return nil
}

func (m *Manager) DeleteState(target string) (Workspace, error) {
	span := tracepkg.StartObserver(m.trace, "WORKSPACE", "workspace.delete-state", "Deleting workspace local state", tracepkg.String("target", target))
	item, registered, lookupErr := m.lookupWorkspace(target)
	root := ""
	if registered {
		root = item.Path
		if err := m.Unregister(item.ID); err != nil {
			span.FailMessage("Workspace state deletion failed", err, tracepkg.String("workspace_id", item.ID), tracepkg.String("root", root))
			return Workspace{}, err
		}
	} else {
		var err error
		root, err = canonicalExistingDirectory(target)
		if err != nil {
			switch {
			case lookupErr != nil:
				err = lookupErr
			case strings.HasPrefix(strings.TrimSpace(target), "ws_"):
				err = fmt.Errorf("%w: %s", ErrNotFound, target)
			}
			span.FailMessage("Workspace state deletion failed", err, tracepkg.String("target", target))
			return Workspace{}, err
		}
	}
	if err := removeLocalState(root); err != nil {
		span.FailMessage("Workspace state deletion failed", err, tracepkg.String("workspace_id", item.ID), tracepkg.String("root", root), tracepkg.Bool("unregistered", registered))
		return Workspace{}, err
	}
	if item.Path == "" {
		item.Path = root
	}
	span.EndMessage("Workspace local state deleted", tracepkg.String("workspace_id", item.ID), tracepkg.String("root", root), tracepkg.Bool("unregistered", registered), tracepkg.Bool("project_files_removed", false), tracepkg.Bool("local_state_removed", true))
	return item, nil
}

func (m *Manager) lookupWorkspace(target string) (Workspace, bool, error) {
	if err := m.ensureLoaded(); err != nil {
		return Workspace{}, false, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	canonical := m.canonicalIDLocked(target)
	if item, ok := m.items[canonical]; ok {
		return item, true, nil
	}
	root, err := canonicalExistingDirectory(target)
	if err != nil {
		return Workspace{}, false, nil
	}
	for _, item := range m.items {
		if filepath.Clean(item.Path) == root {
			return item, true, nil
		}
	}
	return Workspace{}, false, nil
}

func removeLocalState(root string) error {
	local := workspacestate.New(root)
	info, err := os.Lstat(local.Root())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing to delete workspace state through a symlink: %s", local.Root())
	}
	lock, ok, err := oslock.TryAcquire(local.RuntimeLockPath(), oslock.Exclusive)
	if err != nil {
		return fmt.Errorf("lock workspace state: %w", err)
	}
	if !ok {
		return fmt.Errorf("%w: %s", ErrAlreadyActive, root)
	}
	defer lock.Release()
	return os.RemoveAll(local.Root())
}
