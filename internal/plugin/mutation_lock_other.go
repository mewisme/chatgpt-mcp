//go:build !windows && !linux && !darwin

package plugin

import "os"

func lockMutationFile(*os.File) error   { return nil }
func unlockMutationFile(*os.File) error { return nil }
