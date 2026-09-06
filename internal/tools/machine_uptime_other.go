//go:build !linux && !windows && !darwin

package tools

import (
	"fmt"
	"runtime"
	"time"
)

func readMachineUptime() (time.Duration, error) {
	return 0, fmt.Errorf("unsupported platform: %s", runtime.GOOS)
}
