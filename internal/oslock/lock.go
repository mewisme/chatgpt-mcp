package oslock

import (
	"errors"
	"os"
	"path/filepath"
)

type Mode int

const (
	Shared Mode = iota
	Exclusive
)

var ErrBusy = errors.New("file lock is busy")

type Lock struct {
	file *os.File
}

func Acquire(path string, mode Mode) (*Lock, error) {
	return acquire(path, mode, false)
}

func TryAcquire(path string, mode Mode) (*Lock, bool, error) {
	lock, err := acquire(path, mode, true)
	if errors.Is(err, ErrBusy) {
		return nil, false, nil
	}
	return lock, err == nil, err
}

func acquire(path string, mode Mode, nonBlocking bool) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := lockFile(file, mode, nonBlocking); err != nil {
		_ = file.Close()
		return nil, err
	}
	return &Lock{file: file}, nil
}

func (l *Lock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	err := unlockFile(l.file)
	closeErr := l.file.Close()
	l.file = nil
	if err != nil {
		return err
	}
	return closeErr
}
