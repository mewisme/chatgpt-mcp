//go:build !windows

package service

import "os/exec"

func configureCommand(command *exec.Cmd) {}
