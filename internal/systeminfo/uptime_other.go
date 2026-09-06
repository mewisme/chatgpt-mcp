//go:build !linux && !darwin && !windows

package systeminfo

import (
	"errors"
	"time"
)

func readUptime() (time.Duration, error) {
	return 0, errors.New("machine uptime is unsupported on this platform")
}
