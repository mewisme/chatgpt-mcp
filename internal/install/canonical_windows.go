//go:build windows

package install

import "os"

func statusCanonicalPlatform(layout Layout) (CanonicalStatus, error) {
	status := CanonicalStatus{State: CanonicalMissing, Path: layout.CanonicalBinary, Target: layout.CurrentBinary}
	info, err := os.Stat(layout.CanonicalBinary)
	if err != nil {
		if os.IsNotExist(err) {
			return status, nil
		}
		return CanonicalStatus{}, err
	}
	if !info.Mode().IsRegular() {
		status.State = CanonicalConflict
		return status, nil
	}
	status.State = CanonicalInstalled
	return status, nil
}

func installCanonicalPlatform(layout Layout) error { return nil }
func removeCanonicalPlatform(layout Layout) error  { return nil }
