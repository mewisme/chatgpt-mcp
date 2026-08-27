package mcp

import "sync"

type Session struct {
	ID            string
	Notifications chan Notification
	Done          chan struct{}
	mu            sync.Mutex
	closeOnce     sync.Once
}

func NewSession(id string) *Session {
	return &Session{ID: id, Notifications: make(chan Notification, 32), Done: make(chan struct{})}
}

func (s *Session) Notify(notification Notification) {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.Done:
		return
	default:
	}
	select {
	case s.Notifications <- notification:
	default:
	}
}

func (s *Session) Close() { s.closeOnce.Do(func() { close(s.Done) }) }

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
