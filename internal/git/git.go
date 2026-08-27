package git

import (
	"os/exec"
)

func Run(args ...string) ([]byte, error) {
	return exec.Command("git", args...).CombinedOutput()
}

func Status() ([]byte, error) { return Run("status", "--short") }
func Diff() ([]byte, error) { return Run("diff") }
