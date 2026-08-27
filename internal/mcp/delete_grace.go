package mcp

import "time"

const SessionDeleteGrace = 45 * time.Second

type DeleteScheduler struct{}

func (DeleteScheduler) Schedule(fn func()) { time.AfterFunc(SessionDeleteGrace, fn) }
