package checkpoint

import (
	"errors"
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
	roots, err := openRestoreRoots(allowedRoots)
	if err != nil {
		return err
	}
	defer roots.Close()
	for _, stored := range snapshots {
		if err := validateSnapshotPathAllowed(roots, stored.Snapshot); err != nil {
			return err
		}
		if err := s.validateSnapshotStorage(workspaceID, stored.CheckpointID, stored.Snapshot); err != nil {
			return err
		}
	}
	return nil
}

func validateSnapshotPathAllowed(roots restoreRoots, snapshot FileSnapshot) error {
	root, relative, err := roots.target(snapshot.Path)
	if err != nil {
		return fmt.Errorf("checkpoint restore denied for %s: %w", snapshot.Path, err)
	}
	if snapshot.IsSymlink {
		if err := validateRootedSymlinkSnapshot(root, relative, snapshot.Path, snapshot.LinkTarget); err != nil {
			return fmt.Errorf("checkpoint restore denied for %s: %w", snapshot.Path, err)
		}
	} else if err := validateRootedPath(root, relative, snapshot.Path); err != nil {
		return fmt.Errorf("checkpoint restore denied for %s: %w", snapshot.Path, err)
	}
	for _, child := range snapshot.Children {
		if err := validateSnapshotPathAllowed(roots, child); err != nil {
			return err
		}
	}
	return nil
}

func validateRootedSymlinkSnapshot(root *os.Root, relative, path, target string) error {
	if root == nil {
		return fmt.Errorf("checkpoint symlink target escapes allowed root: %s -> %s", filepath.Clean(path), target)
	}
	if strings.TrimSpace(target) == "" {
		return fmt.Errorf("checkpoint symlink target is empty: %s", filepath.Clean(path))
	}
	cleanPath := filepath.Clean(path)
	targetRelative := filepath.Clean(target)
	if filepath.IsAbs(targetRelative) {
		absoluteTarget := filepath.Clean(targetRelative)
		rootPath := filepath.Clean(root.Name())
		if !pathWithin(rootPath, absoluteTarget) {
			return fmt.Errorf("checkpoint symlink target escapes allowed root: %s -> %s", cleanPath, target)
		}
		var err error
		targetRelative, err = filepath.Rel(rootPath, absoluteTarget)
		if err != nil {
			return fmt.Errorf("checkpoint symlink target escapes allowed root: %s -> %s", cleanPath, target)
		}
	} else {
		targetRelative = filepath.Clean(filepath.Join(filepath.Dir(relative), targetRelative))
	}
	if targetRelative == ".." || strings.HasPrefix(targetRelative, ".."+string(filepath.Separator)) || filepath.IsAbs(targetRelative) {
		return fmt.Errorf("checkpoint symlink target escapes allowed root: %s -> %s", cleanPath, target)
	}
	if _, err := root.Stat(targetRelative); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("checkpoint symlink target escapes allowed root: %s -> %s", cleanPath, target)
	}
	ancestor := filepath.Dir(targetRelative)
	for {
		candidate, err := root.OpenRoot(ancestor)
		if err == nil {
			return candidate.Close()
		}
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("checkpoint symlink target escapes allowed root: %s -> %s", cleanPath, target)
		}
		if ancestor == "." {
			return fmt.Errorf("checkpoint symlink target escapes allowed root: %s -> %s", cleanPath, target)
		}
		next := filepath.Dir(ancestor)
		if next == ancestor {
			return fmt.Errorf("checkpoint symlink target escapes allowed root: %s -> %s", cleanPath, target)
		}
		ancestor = next
	}
}

func validateRootedPath(root *os.Root, relative, path string) error {
	if root == nil {
		return fmt.Errorf("checkpoint path is outside allowed root: %s", filepath.Clean(path))
	}
	if _, err := root.Stat(relative); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("checkpoint path escapes allowed root: %s", filepath.Clean(path))
	}
	ancestor := filepath.Dir(relative)
	for {
		candidate, err := root.OpenRoot(ancestor)
		if err == nil {
			return candidate.Close()
		}
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("checkpoint path escapes allowed root: %s", filepath.Clean(path))
		}
		if ancestor == "." {
			return fmt.Errorf("checkpoint path escapes allowed root: %s", filepath.Clean(path))
		}
		next := filepath.Dir(ancestor)
		if next == ancestor {
			return fmt.Errorf("checkpoint path escapes allowed root: %s", filepath.Clean(path))
		}
		ancestor = next
	}
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

func pathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative))
}
