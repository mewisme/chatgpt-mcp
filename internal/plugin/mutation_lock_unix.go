//go:build linux || darwin

package plugin

import (
	"os"

	"golang.org/x/sys/unix"
)

func lockMutationFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_EX)
}

func unlockMutationFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
