package checkpoint

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type restoreRoot struct {
	path string
	root *os.Root
}

type restoreRoots []restoreRoot

func openRestoreRoots(paths []string) (restoreRoots, error) {
	values := uniquePaths(paths)
	sort.Slice(values, func(i, j int) bool { return len(values[i]) > len(values[j]) })
	roots := make(restoreRoots, 0, len(values))
	for _, path := range values {
		root, err := os.OpenRoot(filepath.Clean(path))
		if err != nil {
			_ = roots.Close()
			return nil, fmt.Errorf("open checkpoint restore root %s: %w", path, err)
		}
		roots = append(roots, restoreRoot{path: filepath.Clean(path), root: root})
	}
	return roots, nil
}

func (roots restoreRoots) Close() error {
	var result error
	for _, root := range roots {
		result = errors.Join(result, root.root.Close())
	}
	return result
}

func (roots restoreRoots) target(path string) (*os.Root, string, error) {
	path = filepath.Clean(path)
	for _, root := range roots {
		if !pathWithin(root.path, path) {
			continue
		}
		relative, err := filepath.Rel(root.path, path)
		if err != nil {
			return nil, "", err
		}
		return root.root, relative, nil
	}
	return nil, "", fmt.Errorf("checkpoint restore path is outside allowed roots: %s", path)
}

func (roots restoreRoots) RemoveAll(path string) error {
	root, relative, err := roots.target(path)
	if err != nil {
		return err
	}
	if relative == "." {
		return fmt.Errorf("checkpoint restore cannot remove allowed root: %s", path)
	}
	return root.RemoveAll(relative)
}

func (roots restoreRoots) Restore(snapshot FileSnapshot) error {
	if snapshot.Skipped {
		return fmt.Errorf("cannot restore skipped snapshot for %s: %s", snapshot.Path, snapshot.SkipReason)
	}
	root, relative, err := roots.target(snapshot.Path)
	if err != nil {
		return err
	}
	if snapshot.IsDirectory {
		return roots.restoreDirectory(root, relative, snapshot)
	}
	return restoreFile(root, relative, snapshot)
}

func (roots restoreRoots) restoreDirectory(root *os.Root, relative string, snapshot FileSnapshot) error {
	if info, err := root.Lstat(relative); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			if relative == "." {
				return fmt.Errorf("checkpoint restore root is not a directory: %s", snapshot.Path)
			}
			if err := root.RemoveAll(relative); err != nil {
				return err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := root.MkdirAll(relative, 0755); err != nil {
		return err
	}
	for _, child := range snapshot.Children {
		if err := roots.Restore(child); err != nil {
			return err
		}
	}
	if snapshot.Mode == 0 {
		return nil
	}
	dir, err := root.Open(relative)
	if err != nil {
		return err
	}
	chmodErr := dir.Chmod(os.FileMode(snapshot.Mode) & os.ModePerm)
	closeErr := dir.Close()
	return errors.Join(chmodErr, closeErr)
}

func restoreFile(root *os.Root, relative string, snapshot FileSnapshot) error {
	parent := filepath.Dir(relative)
	if parent != "." {
		if err := root.MkdirAll(parent, 0755); err != nil {
			return err
		}
	}
	if info, err := root.Lstat(relative); err == nil {
		if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			if err := root.RemoveAll(relative); err != nil {
				return err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err := snapshotData(snapshot)
	if err != nil {
		return err
	}
	file, err := root.OpenFile(relative, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	if snapshot.Mode != 0 {
		if err := file.Chmod(os.FileMode(snapshot.Mode) & os.ModePerm); err != nil {
			_ = file.Close()
			return err
		}
	}
	_, writeErr := file.Write(data)
	closeErr := file.Close()
	return errors.Join(writeErr, closeErr)
}

func snapshotData(snapshot FileSnapshot) ([]byte, error) {
	if snapshot.Encoding == "base64" {
		return base64.StdEncoding.DecodeString(snapshot.Content)
	}
	return []byte(snapshot.Content), nil
}
