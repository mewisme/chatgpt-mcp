//go:build darwin

package systeminfo

import (
	"encoding/binary"
	"fmt"
	"syscall"
	"time"
)

func readUptime() (time.Duration, error) {
	raw, err := syscall.Sysctl("kern.boottime")
	if err != nil {
		return 0, err
	}
	data := []byte(raw)
	if len(data) < 16 {
		return 0, fmt.Errorf("unexpected kern.boottime size: %d", len(data))
	}
	seconds := int64(binary.LittleEndian.Uint64(data[:8]))
	microseconds := int64(binary.LittleEndian.Uint64(data[8:16]))
	bootedAt := time.Unix(seconds, microseconds*int64(time.Microsecond))
	now := time.Now()
	if bootedAt.After(now) {
		return 0, fmt.Errorf("kern.boottime is in the future")
	}
	return now.Sub(bootedAt), nil
}
