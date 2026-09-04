//go:build !windows

package install

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func currentTarget(layout Layout) (string, error) {
	info, err := os.Lstat(layout.Current)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return "", fmt.Errorf("%w: current is not a symlink", ErrCurrentNotManaged)
	}
	target, err := filepath.EvalSymlinks(layout.Current)
	if err != nil {
		return "", err
	}
	return filepath.Clean(target), nil
}

func switchCurrent(layout Layout, target string) error {
	if _, err := versionFromTarget(layout, target); err != nil {
		return err
	}
	if _, _, err := CurrentVersion(layout); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(layout.Root, 0755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(layout.Root, ".current-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	if err := os.Remove(tempPath); err != nil {
		return err
	}
	defer os.Remove(tempPath)
	relative, err := filepath.Rel(layout.Root, target)
	if err != nil {
		return err
	}
	if err := os.Symlink(relative, tempPath); err != nil {
		return err
	}
	if err := os.Rename(tempPath, layout.Current); err != nil {
		return fmt.Errorf("switch current install: %w", err)
	}
	return nil
}

func removeCurrent(layout Layout) error {
	if _, _, err := CurrentVersion(layout); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	return os.Remove(layout.Current)
}
