//go:build windows

package tools

import (
	"fmt"
	"syscall"
	"time"
)

var kernel32 = syscall.NewLazyDLL("kernel32.dll")
var getTickCount64 = kernel32.NewProc("GetTickCount64")

func readMachineUptime() (time.Duration, error) {
	if err := getTickCount64.Find(); err != nil {
		return 0, fmt.Errorf("GetTickCount64: %w", err)
	}
	milliseconds, _, _ := getTickCount64.Call()
	return time.Duration(uint64(milliseconds)) * time.Millisecond, nil
}
