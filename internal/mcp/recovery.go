package mcp

import "sync"

type RecoveryManager struct {
	mu sync.Mutex
}

func NewRecoveryManager() *RecoveryManager { return &RecoveryManager{} }

func (r *RecoveryManager) Adopt(id string, sessions *SessionStore) *Session {
	if session, ok := sessions.Get(id); ok {
		return session
	}
	session := &Session{ID: id}
	sessions.Set(session)
	return session
}
