package mcp

import "sync"

type SessionQueue struct {
	mu sync.Mutex
}

func (q *SessionQueue) LockPost() func() {
	q.mu.Lock()
	return q.mu.Unlock
}

func (q *SessionQueue) LockDelete() func() {
	q.mu.Lock()
	return q.mu.Unlock
}
