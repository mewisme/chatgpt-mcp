package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/oslock"
	"go.mewis.me/chatgpt-mcp/internal/state"
	tracepkg "go.mewis.me/chatgpt-mcp/internal/trace"
	"go.mewis.me/chatgpt-mcp/internal/workspacestate"
)

func (m *Manager) Relocate(id, path string) (Workspace, error) {
	span := tracepkg.StartObserver(m.trace, "WORKSPACE", "workspace.relocate", "Relocating workspace", tracepkg.String("workspace_id", strings.TrimSpace(id)), tracepkg.String("input_path", path))
	m.runtimeMu.Lock()
	defer m.runtimeMu.Unlock()
	root, err := canonicalExistingDirectory(path)
	if err != nil {
		span.FailMessage("Workspace relocation failed", err)
		return Workspace{}, err
	}
	if m.protected(root) {
		err := fmt.Errorf("workspace root is inside protected control-plane state: %s", root)
		span.FailMessage("Workspace relocation failed", err)
		return Workspace{}, err
	}
	requestedID := strings.TrimSpace(id)
	m.mu.RLock()
	loaded := m.loaded
	oldID := requestedID
	item := Workspace{}
	if loaded {
		oldID = m.canonicalIDLocked(requestedID)
		item = m.items[oldID]
	}
	active := m.runtime != nil && m.runtime.active
	activeLock := m.runtimeLockLocked(oldID)
	m.mu.RUnlock()
	stored, storedIndex, storedItem, err := m.relocationStore(oldID)
	if err != nil {
		span.FailMessage("Workspace relocation failed", err)
		return Workspace{}, err
	}
	if loaded && item.ID == "" {
		err := fmt.Errorf("%w: %s", ErrNotFound, id)
		span.FailMessage("Workspace relocation failed", err)
		return Workspace{}, err
	}
	if !loaded {
		item = Workspace{ID: storedItem.ID, Path: storedItem.Path, AllowDirs: append([]string(nil), storedItem.AllowDirs...), LegacyIDs: append([]string(nil), storedItem.LegacyIDs...)}
		oldID = item.ID
	}
	oldRoot := item.Path
	sameRoot := filepath.Clean(oldRoot) == filepath.Clean(root)
	for _, existing := range stored.Workspaces {
		if existing.ID != oldID && canonicalRoot(existing.Path) == root {
			err := fmt.Errorf("workspace already registered for destination: %s", root)
			span.FailMessage("Workspace relocation failed", err, tracepkg.String("destination_workspace_id", existing.ID))
			return Workspace{}, err
		}
	}
	local := workspacestate.New(root)
	identity, err := local.LoadIdentity()
	if err != nil {
		span.FailMessage("Workspace relocation failed", err, tracepkg.String("workspace_id", oldID), tracepkg.String("root", root))
		return Workspace{}, err
	}
	if identity.ID != oldID {
		err := fmt.Errorf("workspace identity mismatch at destination: found %s, expected %s", identity.ID, oldID)
		span.FailMessage("Workspace relocation failed", err, tracepkg.String("workspace_id", oldID), tracepkg.String("root", root))
		return Workspace{}, err
	}
	if !sameRoot {
		if sourceIdentity, sourceErr := workspacestate.New(oldRoot).LoadIdentity(); sourceErr == nil && sourceIdentity.ID == oldID {
			err := fmt.Errorf("workspace identity is present at both source and destination; copied .cgm state must be reinitialized before relocation: %s", oldID)
			span.FailMessage("Workspace relocation failed", err, tracepkg.String("workspace_id", oldID), tracepkg.String("source_root", oldRoot), tracepkg.String("root", root))
			return Workspace{}, err
		}
	}
	var transientLocks map[string]*oslock.Lock
	if active {
		if activeLock == nil {
			err := fmt.Errorf("workspace runtime lock missing during relocation: %s", oldID)
			span.FailMessage("Workspace relocation failed", err, tracepkg.String("workspace_id", oldID), tracepkg.String("root", root))
			return Workspace{}, err
		}
		same, sameErr := activeLock.SameFile(local.RuntimeLockPath())
		if sameErr != nil {
			err := fmt.Errorf("verify workspace runtime lock during relocation: %w", sameErr)
			span.FailMessage("Workspace relocation failed", err, tracepkg.String("workspace_id", oldID), tracepkg.String("root", root))
			return Workspace{}, err
		}
		if !same {
			err := fmt.Errorf("active workspace relocation requires the existing .cgm state to move with the workspace: %s", oldID)
			span.FailMessage("Workspace relocation failed", err, tracepkg.String("workspace_id", oldID), tracepkg.String("root", root))
			return Workspace{}, err
		}
	} else {
		transientLocks, err = m.acquireRelocationLocks(stored, oldID, root)
		if err != nil {
			span.FailMessage("Workspace relocation failed", err, tracepkg.String("workspace_id", oldID), tracepkg.String("root", root))
			return Workspace{}, err
		}
		defer releaseRuntimeLocks(transientLocks)
	}
	config, err := local.LoadConfig()
	if err != nil {
		span.FailMessage("Workspace relocation failed", err, tracepkg.String("workspace_id", oldID), tracepkg.String("root", root))
		return Workspace{}, err
	}
	if sameRoot {
		item.AllowDirs = normalizeRoots(config.AllowDirs)
		item.LegacyIDs = normalizeIDs(config.LegacyIDs, oldID)
		span.EndMessage("Workspace already uses requested root", tracepkg.String("workspace_id", oldID), tracepkg.String("root", root), tracepkg.Bool("changed", false))
		return item, nil
	}
	for _, stateRoot := range []string{local.StateRoot(), local.CheckpointRoot()} {
		if _, statErr := os.Stat(stateRoot); errors.Is(statErr, os.ErrNotExist) {
			continue
		} else if statErr != nil {
			span.FailMessage("Workspace relocation failed", statErr, tracepkg.String("workspace_id", oldID), tracepkg.String("state_root", stateRoot))
			return Workspace{}, statErr
		}
		if _, err := rewriteRelocatedWorkspaceState(stateRoot, oldID, oldID, oldRoot, root); err != nil {
			span.FailMessage("Workspace relocation failed", err, tracepkg.String("workspace_id", oldID), tracepkg.String("state_root", stateRoot))
			return Workspace{}, err
		}
	}
	previousConfig := config
	for index, allowDir := range config.AllowDirs {
		if relocated, ok := relocateAbsolutePath(allowDir, oldRoot, root); ok {
			config.AllowDirs[index] = relocated
		}
	}
	config.AllowDirs = normalizeRoots(config.AllowDirs)
	config.LegacyIDs = normalizeIDs(config.LegacyIDs, oldID)
	if err := local.SaveConfig(config); err != nil {
		span.FailMessage("Workspace relocation failed", err, tracepkg.String("workspace_id", oldID), tracepkg.String("root", root))
		return Workspace{}, err
	}
	if err := local.EnsureGitExcluded(context.Background()); err != nil {
		_ = local.SaveConfig(previousConfig)
		span.FailMessage("Workspace relocation failed", err, tracepkg.String("workspace_id", oldID), tracepkg.String("root", root))
		return Workspace{}, err
	}
	previous := item
	item.Path = root
	item.AllowDirs = append([]string(nil), config.AllowDirs...)
	item.LegacyIDs = append([]string(nil), config.LegacyIDs...)
	if loaded {
		m.mu.Lock()
		m.items[oldID] = item
		if err := m.saveLocked(); err != nil {
			m.items[oldID] = previous
			m.mu.Unlock()
			_ = local.SaveConfig(previousConfig)
			span.FailMessage("Workspace relocation failed", err)
			return Workspace{}, err
		}
		if active {
			m.runtime.roots[oldID] = root
		}
		m.mu.Unlock()
	} else {
		stored.Workspaces[storedIndex].Path = root
		data, marshalErr := configformat.MarshalPath(m.path, stored)
		if marshalErr != nil {
			_ = local.SaveConfig(previousConfig)
			span.FailMessage("Workspace relocation failed", marshalErr)
			return Workspace{}, marshalErr
		}
		if writeErr := state.WriteFileAtomic(m.path, data, 0600); writeErr != nil {
			_ = local.SaveConfig(previousConfig)
			span.FailMessage("Workspace relocation failed", writeErr)
			return Workspace{}, writeErr
		}
	}
	span.EndMessage("Workspace relocated", tracepkg.String("workspace_id", oldID), tracepkg.String("previous_root", oldRoot), tracepkg.String("root", root), tracepkg.Bool("identity_preserved", true))
	_ = m.notifyRelocated(item)
	return item, nil
}

func (m *Manager) runtimeLockLocked(id string) *oslock.Lock {
	if m.runtime == nil {
		return nil
	}
	return m.runtime.locks[id]
}

func (m *Manager) relocationStore(id string) (storeFile, int, storedWorkspace, error) {
	data, err := os.ReadFile(m.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return storeFile{}, -1, storedWorkspace{}, fmt.Errorf("%w: %s", ErrNotFound, id)
		}
		return storeFile{}, -1, storedWorkspace{}, fmt.Errorf("read workspace registry: %w", err)
	}
	var stored storeFile
	if err := configformat.UnmarshalPath(m.path, data, &stored); err != nil {
		return storeFile{}, -1, storedWorkspace{}, fmt.Errorf("decode workspace registry: %w", err)
	}
	if stored.Version < 1 || stored.Version > storeVersion {
		return storeFile{}, -1, storedWorkspace{}, fmt.Errorf("unsupported workspace registry version: %d", stored.Version)
	}
	for index, item := range stored.Workspaces {
		if strings.TrimSpace(item.ID) == id {
			return stored, index, item, nil
		}
	}
	return storeFile{}, -1, storedWorkspace{}, fmt.Errorf("%w: %s", ErrNotFound, id)
}

func (m *Manager) acquireRelocationLocks(stored storeFile, targetID, targetRoot string) (map[string]*oslock.Lock, error) {
	items := make([]Workspace, 0, len(stored.Workspaces))
	for _, storedItem := range stored.Workspaces {
		root := storedItem.Path
		if storedItem.ID == targetID {
			root = targetRoot
		}
		canonical, err := canonicalExistingDirectory(root)
		if err != nil {
			if storedItem.ID == targetID {
				return nil, fmt.Errorf("resolve workspace %s for relocation lock: %w", storedItem.ID, err)
			}
			continue
		}
		local := workspacestate.New(canonical)
		identity, err := local.LoadIdentity()
		if errors.Is(err, os.ErrNotExist) && stored.Version < storeVersion {
			identity, _, err = local.EnsureIdentity(storedItem.ID)
			if err == nil {
				err = local.EnsureGitExcluded(context.Background())
			}
		}
		if err != nil || identity.ID != storedItem.ID {
			if storedItem.ID == targetID {
				if err != nil {
					return nil, fmt.Errorf("%w: %s: %v", ErrStateLost, storedItem.ID, err)
				}
				return nil, fmt.Errorf("%w: %s: identity changed to %s", ErrStateLost, storedItem.ID, identity.ID)
			}
			continue
		}
		items = append(items, Workspace{ID: storedItem.ID, Path: canonical})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	locks := map[string]*oslock.Lock{}
	for _, item := range items {
		lock, err := m.acquireRuntimeLock(item)
		if err != nil {
			_ = releaseRuntimeLocks(locks)
			return nil, err
		}
		locks[item.ID] = lock
	}
	return locks, nil
}

func rewriteRelocatedWorkspaceState(root, oldID, newID, oldRoot, newRoot string) (int, error) {
	paths := []string{}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		return 0, err
	}
	rewritten := 0
	for _, path := range paths {
		format, err := configformat.Detect(path)
		if err != nil {
			continue
		}
		data, err := os.ReadFile(path) // #nosec G304 -- path is emitted by filepath.WalkDir under the workspace-owned state root.
		if err != nil {
			return rewritten, err
		}
		decoded, err := configformat.DecodeGeneric(format, data)
		if err != nil {
			if isCheckpointManifestPath(path) {
				continue
			}
			return rewritten, fmt.Errorf("decode workspace state %s: %w", path, err)
		}
		updated, changed := relocateStateValue(decoded, oldID, newID, oldRoot, newRoot)
		if !changed {
			continue
		}
		encoded, err := configformat.EncodeGeneric(format, updated)
		if err != nil {
			return rewritten, fmt.Errorf("encode workspace state %s: %w", path, err)
		}
		if err := state.WriteFileAtomic(path, encoded, 0600); err != nil {
			return rewritten, fmt.Errorf("rewrite workspace state %s: %w", path, err)
		}
		rewritten++
	}
	return rewritten, nil
}

func isCheckpointManifestPath(path string) bool {
	clean := filepath.ToSlash(filepath.Clean(path))
	return strings.Contains(clean, "/checkpoints/data/") && strings.HasPrefix(filepath.Base(clean), "manifest.")
}

func relocateStateValue(value any, oldID, newID, oldRoot, newRoot string) (any, bool) {
	switch typed := value.(type) {
	case map[string]any:
		changed := false
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			if key == "workspace_id" {
				if text, ok := item.(string); ok && text == oldID {
					result[key], changed = newID, true
					continue
				}
			}
			updated, itemChanged := relocateStateValue(item, oldID, newID, oldRoot, newRoot)
			result[key] = updated
			changed = changed || itemChanged
		}
		return result, changed
	case []any:
		changed := false
		result := make([]any, len(typed))
		for index, item := range typed {
			updated, itemChanged := relocateStateValue(item, oldID, newID, oldRoot, newRoot)
			result[index] = updated
			changed = changed || itemChanged
		}
		return result, changed
	case string:
		if relocated, ok := relocateAbsolutePath(typed, oldRoot, newRoot); ok {
			return relocated, true
		}
	}
	return value, false
}

func relocateAbsolutePath(value, oldRoot, newRoot string) (string, bool) {
	if !filepath.IsAbs(value) {
		return value, false
	}
	clean := filepath.Clean(value)
	comparisonValue := clean
	if canonical, err := canonicalForContainment(clean, false); err == nil {
		comparisonValue = canonical
	}
	comparisonOldRoot := filepath.Clean(oldRoot)
	if canonical, err := canonicalForContainment(comparisonOldRoot, false); err == nil {
		comparisonOldRoot = canonical
	}
	relative, err := filepath.Rel(comparisonOldRoot, comparisonValue)
	if err != nil || relative == ".." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return value, false
	}
	if relative == "." {
		return filepath.Clean(newRoot), true
	}
	return filepath.Join(newRoot, relative), true
}
