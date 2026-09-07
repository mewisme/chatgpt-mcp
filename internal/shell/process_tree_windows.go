//go:build windows

package shell

import (
	"os"
	"os/exec"
	"time"
)

func configureCommandLifecycle(cmd *exec.Cmd) {
	if cmd != nil {
		cmd.WaitDelay = time.Second
	}
}

func signalCommandTree(cmd *exec.Cmd, force bool) error {
	if cmd == nil || cmd.Process == nil {
		return os.ErrProcessDone
	}
	if force {
		return cmd.Process.Kill()
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		return cmd.Process.Kill()
	}
	return nil
}
