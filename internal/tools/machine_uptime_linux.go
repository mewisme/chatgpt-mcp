//go:build linux

package tools

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func readMachineUptime() (time.Duration, error) {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0, err
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0, fmt.Errorf("empty /proc/uptime")
	}
	seconds, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("parse /proc/uptime: %w", err)
	}
	if seconds < 0 {
		return 0, fmt.Errorf("negative /proc/uptime")
	}
	return time.Duration(seconds * float64(time.Second)), nil
}
