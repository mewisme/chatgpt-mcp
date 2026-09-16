package workspace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.mewis.me/chatgpt-mcp/internal/configformat"
	"go.mewis.me/chatgpt-mcp/internal/state"
	tracepkg "go.mewis.me/chatgpt-mcp/internal/trace"
	"go.mewis.me/chatgpt-mcp/internal/workspacestate"
)

func (m *Manager) Relocate(id, path string) (Workspace, error) {
	span := tracepkg.StartObserver(m.trace, "WORKSPACE", "workspace.relocate", "Relocating workspace", tracepkg.String("workspace_id", strings.TrimSpace(id)), tracepkg.String("input_path", path))
	if err := m.ensureLoaded(); err != nil {
		span.FailMessage("Workspace relocation failed", err)
		return Workspace{}, err
	}
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

	m.mu.Lock()
	defer m.mu.Unlock()
	oldID := m.canonicalIDLocked(strings.TrimSpace(id))
	item, ok := m.items[oldID]
	if !ok {
		err := fmt.Errorf("%w: %s", ErrNotFound, id)
		span.FailMessage("Workspace relocation failed", err)
		return Workspace{}, err
	}
	oldRoot := item.Path
	if filepath.Clean(oldRoot) == filepath.Clean(root) {
		span.EndMessage("Workspace already uses requested root", tracepkg.String("workspace_id", oldID), tracepkg.String("root", root), tracepkg.Bool("changed", false))
		return item, nil
	}
	for otherID, existing := range m.items {
		if otherID != oldID && filepath.Clean(existing.Path) == root {
			err := fmt.Errorf("workspace already registered for destination: %s", root)
			span.FailMessage("Workspace relocation failed", err, tracepkg.String("destination_workspace_id", otherID))
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
	config, err := local.LoadConfig()
	if err != nil {
		span.FailMessage("Workspace relocation failed", err, tracepkg.String("workspace_id", oldID), tracepkg.String("root", root))
		return Workspace{}, err
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
	m.items[oldID] = item
	if err := m.saveLocked(); err != nil {
		m.items[oldID] = previous
		_ = local.SaveConfig(previousConfig)
		span.FailMessage("Workspace relocation failed", err)
		return Workspace{}, err
	}
	span.EndMessage("Workspace relocated", tracepkg.String("workspace_id", oldID), tracepkg.String("previous_root", oldRoot), tracepkg.String("root", root), tracepkg.Bool("identity_preserved", true))
	return item, nil
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
