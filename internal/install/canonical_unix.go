//go:build !windows

package install

import (
	"os"
	"path/filepath"
)

func statusCanonicalPlatform(layout Layout) (CanonicalStatus, error) {
	status := CanonicalStatus{State: CanonicalMissing, Path: layout.CanonicalBinary, Target: layout.CurrentBinary}
	info, err := os.Lstat(layout.CanonicalBinary)
	if err != nil {
		if os.IsNotExist(err) {
			return status, nil
		}
		return CanonicalStatus{}, err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		status.State = CanonicalConflict
		return status, nil
	}
	target, err := os.Readlink(layout.CanonicalBinary)
	if err != nil {
		return CanonicalStatus{}, err
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(layout.CanonicalBinary), target)
	}
	target = filepath.Clean(target)
	status.Target = target
	if samePath(target, layout.CurrentBinary) {
		status.State = CanonicalInstalled
	} else {
		status.State = CanonicalConflict
	}
	return status, nil
}

func installCanonicalPlatform(layout Layout) error {
	if err := os.MkdirAll(layout.BinDir, 0755); err != nil {
		return err
	}
	target, err := filepath.Rel(layout.BinDir, layout.CurrentBinary)
	if err != nil {
		return err
	}
	return os.Symlink(target, layout.CanonicalBinary)
}

func removeCanonicalPlatform(layout Layout) error {
	return os.Remove(layout.CanonicalBinary)
}
