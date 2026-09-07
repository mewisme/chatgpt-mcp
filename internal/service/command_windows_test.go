//go:build windows

package service

import (
	"os/exec"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

func TestConfigureCommandSuppressesHelperConsoleWindow(t *testing.T) {
	command := exec.Command("schtasks.exe", "/Query")
	configureCommand(command)
	if command.SysProcAttr == nil || !command.SysProcAttr.HideWindow {
		t.Fatalf("SysProcAttr=%#v", command.SysProcAttr)
	}
	if command.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Fatalf("creation flags=%#x, want CREATE_NO_WINDOW", command.SysProcAttr.CreationFlags)
	}
	var _ *syscall.SysProcAttr = command.SysProcAttr
}
