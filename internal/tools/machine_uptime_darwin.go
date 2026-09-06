//go:build darwin

package tools

import (
	"fmt"
	"time"

	"golang.org/x/sys/unix"
)

func readMachineUptime() (time.Duration, error) {
	bootTime, err := unix.SysctlTimeval("kern.boottime")
	if err != nil {
		return 0, err
	}
	bootedAt := time.Unix(bootTime.Unix())
	now := time.Now()
	if bootedAt.After(now) {
		return 0, fmt.Errorf("kern.boottime is in the future")
	}
	return now.Sub(bootedAt), nil
}
