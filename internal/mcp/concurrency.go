package mcp

import "sync"

type SessionQueue struct {
	locks sync.Map
}

func NewSessionQueue() *SessionQueue { return &SessionQueue{} }

func (q *SessionQueue) Lock(id string) func() {
	value, _ := q.locks.LoadOrStore(id, &sync.Mutex{})
	mutex := value.(*sync.Mutex)
	mutex.Lock()
	return mutex.Unlock
}
