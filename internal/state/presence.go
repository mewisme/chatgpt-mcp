package state

import (
	"path/filepath"

	"go.mewis.me/chatgpt-mcp/internal/oslock"
)

func TUIPresencePath() string {
	return filepath.Join(Root(), "tui-presence.lock")
}

func HoldTUIPresence() (*oslock.Lock, error) {
	return oslock.Acquire(TUIPresencePath(), oslock.Shared)
}

func TUIReviewerActive() (bool, error) {
	lock, ok, err := oslock.TryAcquire(TUIPresencePath(), oslock.Exclusive)
	if err != nil {
		return false, err
	}
	if !ok {
		return true, nil
	}
	return false, lock.Release()
}
