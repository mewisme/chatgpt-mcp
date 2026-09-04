package install

import (
	"errors"
	"os"
)

func RollbackResult(result Result) error {
	if err := Rollback(result.Activation); err != nil {
		return err
	}
	if result.PreviousMetadata != nil {
		return WriteMetadata(result.Layout.Metadata, *result.PreviousMetadata)
	}
	if err := os.Remove(result.Layout.Metadata); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func FinalizeResult(result Result) error {
	keep := []string{result.Version}
	if previous := result.Activation.PreviousVersion; previous != "" && previous != result.Version {
		keep = append(keep, previous)
	}
	return Cleanup(result.Layout, keep...)
}
