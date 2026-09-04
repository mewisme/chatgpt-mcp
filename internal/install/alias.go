package install

import (
	"errors"
	"fmt"
	"os"
)

var ErrAliasConflict = errors.New("alias path is occupied by another file")

type AliasState string

const (
	AliasMissing   AliasState = "missing"
	AliasInstalled AliasState = "installed"
	AliasConflict  AliasState = "conflict"
)

type AliasStatus struct {
	State  AliasState
	Path   string
	Target string
}

func StatusAlias(layout Layout) (AliasStatus, error) {
	return statusAliasPlatform(layout)
}

func InstallAlias(layout Layout) (AliasStatus, error) {
	if err := ensureCurrentBinary(layout); err != nil {
		return AliasStatus{}, err
	}
	status, err := StatusAlias(layout)
	if err != nil {
		return AliasStatus{}, err
	}
	switch status.State {
	case AliasInstalled:
		return status, nil
	case AliasConflict:
		return status, fmt.Errorf("%w: %s", ErrAliasConflict, status.Path)
	}
	if err := installAliasPlatform(layout); err != nil {
		return AliasStatus{}, err
	}
	return StatusAlias(layout)
}

func RemoveAlias(layout Layout) (AliasStatus, error) {
	status, err := StatusAlias(layout)
	if err != nil {
		return AliasStatus{}, err
	}
	if status.State == AliasMissing {
		return status, nil
	}
	if status.State == AliasConflict {
		return status, fmt.Errorf("%w: %s", ErrAliasConflict, status.Path)
	}
	if err := removeAliasPlatform(layout); err != nil {
		return AliasStatus{}, err
	}
	return StatusAlias(layout)
}

func ensureCurrentBinary(layout Layout) error {
	if _, _, err := CurrentVersion(layout); err != nil {
		return err
	}
	info, err := os.Stat(layout.CurrentBinary)
	if err != nil {
		return fmt.Errorf("current binary unavailable: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("current binary is not a regular file: %s", layout.CurrentBinary)
	}
	return nil
}
