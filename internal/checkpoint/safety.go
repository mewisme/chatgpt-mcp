package checkpoint

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (s *Store) ValidateRestorePaths(workspaceID, workspaceRoot, id string) error {
	return s.ValidateRestorePathsAllowed(workspaceID, workspaceRoot, []string{workspaceRoot}, id)
}

func (s *Store) ValidateRestorePathsAllowed(workspaceID, workspaceRoot string, allowedRoots []string, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	allowedRoots = effectiveRoots(workspaceRoot, allowedRoots)
	_, snapshots, err := s.collectRestorePlanLocked(workspaceID, workspaceRoot, allowedRoots, id)
	if err != nil {
		return err
	}
	for _, snapshot := range snapshots {
		if err := validateSnapshotPathAllowed(allowedRoots, snapshot); err != nil {
			return err
		}
	}
	return nil
}

func validateSnapshotPathAllowed(roots []string, snapshot FileSnapshot) error {
	if _, err := safeCanonicalAny(roots, snapshot.Path); err != nil {
		return fmt.Errorf("checkpoint restore denied for %s: %w", snapshot.Path, err)
	}
	for _, child := range snapshot.Children {
		if err := validateSnapshotPathAllowed(roots, child); err != nil {
			return err
		}
	}
	return nil
}

func validateSnapshotPath(root string, snapshot FileSnapshot) error {
	return validateSnapshotPathAllowed([]string{root}, snapshot)
}

func safeCanonicalAny(roots []string, candidate string) (string, error) {
	var lastErr error
	for _, root := range roots {
		resolved, err := safeCanonical(root, candidate)
		if err == nil {
			return resolved, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no allowed roots configured")
	}
	return "", lastErr
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
