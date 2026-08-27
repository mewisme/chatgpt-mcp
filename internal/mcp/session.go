package mcp

import "sync"

type Session struct {
	ID string
	mu sync.Mutex
}

type SessionManager struct{ sessions map[string]*Session }

func NewSessionManager() *SessionManager { return &SessionManager{sessions: map[string]*Session{}} }

func (m *SessionManager) Get(id string) *Session {
	if s := m.sessions[id]; s != nil {
		return s
	}
	s := &Session{ID: id}
	m.sessions[id] = s
	return s
}
