//go:build linux || darwin

package cli

import (
	"os/exec"
	"syscall"
)

func launchPackageUpgradeHandoff(h packageUpgradeHandoff) error {
	// #nosec G204 -- ScriptPath is created internally with os.CreateTemp and is never caller-provided.
	command := exec.Command("sh", h.ScriptPath)
	command.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}
