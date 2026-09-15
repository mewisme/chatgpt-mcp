//go:build windows

package cli

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func launchPackageUpgradeHandoff(h packageUpgradeHandoff) error {
	shell, err := packagePowerShell()
	if err != nil {
		return err
	}
	command := exec.Command(shell, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", h.ScriptPath)
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW}
	if err := command.Start(); err != nil {
		return err
	}
	return command.Process.Release()
}
