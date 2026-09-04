//go:build windows

package install

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func currentTarget(layout Layout) (string, error) {
	if _, err := os.Lstat(layout.Current); err != nil {
		return "", err
	}
	target, err := filepath.EvalSymlinks(layout.Current)
	if err != nil {
		return "", err
	}
	target, err = filepath.Abs(target)
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
	temp, err := os.MkdirTemp(layout.Root, ".current-link-")
	if err != nil {
		return err
	}
	if err := os.Remove(temp); err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	if output, err := exec.Command("cmd", "/c", "mklink", "/J", temp, target).CombinedOutput(); err != nil {
		return fmt.Errorf("create current junction: %w: %s", err, output)
	}
	backup := ""
	if _, err := os.Lstat(layout.Current); err == nil {
		backup, err = os.MkdirTemp(layout.Root, ".previous-current-")
		if err != nil {
			return err
		}
		if err := os.Remove(backup); err != nil {
			return err
		}
		if err := os.Rename(layout.Current, backup); err != nil {
			return fmt.Errorf("move current junction: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(temp, layout.Current); err != nil {
		if backup != "" {
			_ = os.Rename(backup, layout.Current)
		}
		return fmt.Errorf("switch current install: %w", err)
	}
	if backup != "" {
		if err := os.RemoveAll(backup); err != nil {
			return fmt.Errorf("remove previous current junction: %w", err)
		}
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
	return os.RemoveAll(layout.Current)
}
