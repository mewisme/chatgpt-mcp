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
	for _, stored := range snapshots {
		if err := validateSnapshotPathAllowed(allowedRoots, stored.Snapshot); err != nil {
			return err
		}
		if err := s.validateSnapshotStorage(workspaceID, stored.CheckpointID, stored.Snapshot); err != nil {
			return err
		}
	}
	return nil
}

func validateSnapshotPathAllowed(roots []string, snapshot FileSnapshot) error {
	if snapshot.IsSymlink {
		if err := validateSymlinkSnapshot(roots, snapshot.Path, snapshot.LinkTarget); err != nil {
			return fmt.Errorf("checkpoint restore denied for %s: %w", snapshot.Path, err)
		}
	} else if _, err := safeCanonicalAny(roots, snapshot.Path); err != nil {
		return fmt.Errorf("checkpoint restore denied for %s: %w", snapshot.Path, err)
	}
	for _, child := range snapshot.Children {
		if err := validateSnapshotPathAllowed(roots, child); err != nil {
			return err
		}
	}
	return nil
}

func validateSymlinkSnapshot(roots []string, path, target string) error {
	if strings.TrimSpace(target) == "" {
		return fmt.Errorf("checkpoint symlink target is empty: %s", path)
	}
	cleanPath := filepath.Clean(path)
	targetPath := filepath.Clean(target)
	if !filepath.IsAbs(targetPath) {
		targetPath = filepath.Join(filepath.Dir(cleanPath), targetPath)
	}
	for _, root := range roots {
		root = filepath.Clean(root)
		if !within(root, cleanPath) || !within(root, targetPath) {
			continue
		}
		if _, err := safeCanonical(root, filepath.Dir(cleanPath)); err != nil {
			continue
		}
		if _, err := safeCanonical(root, targetPath); err == nil {
			return nil
		}
	}
	return fmt.Errorf("checkpoint symlink target escapes allowed root: %s -> %s", cleanPath, target)
}

func (s *Store) validateSnapshotStorage(workspaceID, checkpointID string, snapshot FileSnapshot) error {
	if snapshot.Blob != "" {
		blob, err := openVerifiedBlob(s.checkpointDir(workspaceID, checkpointID), snapshot.Blob, snapshot)
		if err != nil {
			return fmt.Errorf("checkpoint blob invalid for %s: %w", snapshot.Path, err)
		}
		if err := blob.Close(); err != nil {
			return err
		}
	}
	for _, child := range snapshot.Children {
		if err := s.validateSnapshotStorage(workspaceID, checkpointID, child); err != nil {
			return err
		}
	}
	return nil
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
