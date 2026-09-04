//go:build !windows

package install

import (
	"os"
	"path/filepath"
)

func statusAliasPlatform(layout Layout) (AliasStatus, error) {
	status := AliasStatus{State: AliasMissing, Path: layout.AliasPath, Target: layout.CurrentBinary}
	info, err := os.Lstat(layout.AliasPath)
	if err != nil {
		if os.IsNotExist(err) {
			return status, nil
		}
		return AliasStatus{}, err
	}
	if info.Mode()&os.ModeSymlink == 0 {
		status.State = AliasConflict
		return status, nil
	}
	target, err := os.Readlink(layout.AliasPath)
	if err != nil {
		return AliasStatus{}, err
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(layout.AliasPath), target)
	}
	target = filepath.Clean(target)
	status.Target = target
	if samePath(target, layout.CurrentBinary) {
		status.State = AliasInstalled
	} else {
		status.State = AliasConflict
	}
	return status, nil
}

func installAliasPlatform(layout Layout) error {
	if err := os.MkdirAll(layout.BinDir, 0755); err != nil {
		return err
	}
	target, err := filepath.Rel(layout.BinDir, layout.CurrentBinary)
	if err != nil {
		return err
	}
	return os.Symlink(target, layout.AliasPath)
}

func removeAliasPlatform(layout Layout) error {
	return os.Remove(layout.AliasPath)
}
