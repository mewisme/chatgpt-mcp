package plugin

import (
	"go.mewis.me/chatgpt-mcp/internal/oslock"
)

type mutationFileLock struct {
	lock *oslock.Lock
}

func acquireMutationFileLock(path string) (*mutationFileLock, error) {
	lock, err := oslock.Acquire(path, oslock.Exclusive)
	if err != nil {
		return nil, err
	}
	return &mutationFileLock{lock: lock}, nil
}

func (lock *mutationFileLock) release() error {
	if lock == nil || lock.lock == nil {
		return nil
	}
	return lock.lock.Release()
}

func (store *Store) lockMutation() (func(), error) {
	store.mu.Lock()
	lock, err := acquireMutationFileLock(store.layout.MutationLockPath())
	if err != nil {
		store.mu.Unlock()
		return nil, err
	}
	return func() {
		_ = lock.release()
		store.mu.Unlock()
	}, nil
}
