package plugindev

import (
	"fmt"

	"go.mewis.me/chatgpt-mcp/internal/oslock"
)

func AcquireBootstrapLock(repoRoot string) (*oslock.Lock, error) {
	path, err := BootstrapLockPath(repoRoot)
	if err != nil {
		return nil, err
	}
	lock, err := oslock.Acquire(path, oslock.Exclusive)
	if err != nil {
		return nil, fmt.Errorf("dev plugin bootstrap lock: %w", err)
	}
	return lock, nil
}
