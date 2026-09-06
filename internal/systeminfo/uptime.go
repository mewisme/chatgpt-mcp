package systeminfo

import "time"

func Uptime() (time.Duration, error) { return readUptime() }
