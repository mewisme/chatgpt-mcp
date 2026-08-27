package mcp

import "time"

type Recovery struct {
	Enabled bool
	Grace   time.Duration
}

func DefaultRecovery() Recovery {
	return Recovery{Enabled: true, Grace: 45 * time.Second}
}

func (r Recovery) CanRecover(id string) bool {
	return r.Enabled && id != ""
}
