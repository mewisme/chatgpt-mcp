package plugin

import (
	"os"
	"path/filepath"
)

type mutationFileLock struct {
	file *os.File
}

func acquireMutationFileLock(path string) (*mutationFileLock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := lockMutationFile(file); err != nil {
		_ = file.Close()
		return nil, err
	}
	return &mutationFileLock{file: file}, nil
}

func (lock *mutationFileLock) release() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	unlockErr := unlockMutationFile(lock.file)
	closeErr := lock.file.Close()
	if unlockErr != nil {
		return unlockErr
	}
	return closeErr
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
