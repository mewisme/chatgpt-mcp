//go:build windows

package runtimeplugin

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func configureProcess(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NEW_PROCESS_GROUP
}

func signalProcess(cmd *exec.Cmd, force bool) error {
	if cmd == nil || cmd.Process == nil {
		return os.ErrProcessDone
	}
	if !force && windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(cmd.Process.Pid)) == nil {
		return nil
	}
	return cmd.Process.Kill()
}
