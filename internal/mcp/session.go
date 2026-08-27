package mcp

import "sync"

type Session struct {
	ID            string
	Notifications chan Notification
	mu            sync.Mutex
}

func NewSession(id string) *Session {
	return &Session{ID: id, Notifications: make(chan Notification, 32)}
}

func (s *Session) Notify(notification Notification) {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case s.Notifications <- notification:
	default:
	}
}

type SessionManager struct{ sessions map[string]*Session }

func NewSessionManager() *SessionManager { return &SessionManager{sessions: map[string]*Session{}} }

func (m *SessionManager) Get(id string) *Session {
	if s := m.sessions[id]; s != nil {
		return s
	}
	s := NewSession(id)
	m.sessions[id] = s
	return s
}
