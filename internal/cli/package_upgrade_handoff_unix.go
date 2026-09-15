//go:build linux || darwin

package cli

import (
	"os/exec"
	"syscall"
)

func launchPackageUpgradeHandoff(h packageUpgradeHandoff) error {
	command := exec.Command("sh", h.ScriptPath)
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}
