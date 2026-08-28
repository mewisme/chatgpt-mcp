package checkpoint

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (s *Store) ValidateRestorePaths(workspaceID, workspaceRoot, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, snapshots, err := s.collectRestorePlanLocked(workspaceID, workspaceRoot, id)
	if err != nil {
		return err
	}
	for _, snapshot := range snapshots {
		if err := validateSnapshotPath(workspaceRoot, snapshot); err != nil {
			return err
		}
	}
	return nil
}

func validateSnapshotPath(root string, snapshot FileSnapshot) error {
	if _, err := safeCanonical(root, snapshot.Path); err != nil {
		return fmt.Errorf("checkpoint restore denied for %s: %w", snapshot.Path, err)
	}
	for _, child := range snapshot.Children {
		if err := validateSnapshotPath(root, child); err != nil {
			return err
		}
	}
	return nil
}

func safeCanonical(root, candidate string) (string, error) {
	rootCanonical, err := filepath.EvalSymlinks(filepath.Clean(root))
	if err != nil {
		return "", err
	}
	clean := filepath.Clean(candidate)
	if resolved, err := filepath.EvalSymlinks(clean); err == nil {
		if !pathWithin(rootCanonical, resolved) {
			return "", fmt.Errorf("path escapes workspace through symlink: %s", resolved)
		}
		return resolved, nil
	}

	current := clean
	var suffix []string
	for {
		if _, err := os.Lstat(current); err == nil {
			break
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("cannot resolve path ancestor: %s", candidate)
		}
		suffix = append(suffix, filepath.Base(current))
		current = parent
	}
	resolved, err := filepath.EvalSymlinks(current)
	if err != nil {
		return "", err
	}
	for i := len(suffix) - 1; i >= 0; i-- {
		resolved = filepath.Join(resolved, suffix[i])
	}
	if !pathWithin(rootCanonical, resolved) {
		return "", fmt.Errorf("path escapes workspace through symlink: %s", resolved)
	}
	return resolved, nil
}

func pathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative))
}
