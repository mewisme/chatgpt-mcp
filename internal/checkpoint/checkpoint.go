package checkpoint

import (
	"os"
	"path/filepath"
)

type Store struct{ Root string }

func (s Store) Path(workspaceID string) string {
	return filepath.Join(s.Root, "workspaces", workspaceID, "checkpoints")
}

func (s Store) Ensure(workspaceID string) error { return os.MkdirAll(s.Path(workspaceID), 0700) }
