package tools

import (
	"errors"
	"strings"
	"sync"
	"time"
)

const defaultSessionWorkspaceAccessTTL = 30 * 24 * time.Hour

type SessionWorkspaceAccessDecision string

const (
	SessionWorkspaceAccessNew      SessionWorkspaceAccessDecision = "new"
	SessionWorkspaceAccessExisting SessionWorkspaceAccessDecision = "existing"
)

type WorkspaceAccess struct {
	WorkspaceID string
	GrantedAt   time.Time
	LastSeen    time.Time
}

type SessionWorkspaceAccess struct {
	Workspaces map[string]WorkspaceAccess
	CreatedAt  time.Time
	LastSeen   time.Time
}

type SessionWorkspaceAccessManager struct {
	mu       sync.Mutex
	sessions map[string]SessionWorkspaceAccess
	now      func() time.Time
	ttl      time.Duration
}

func NewSessionWorkspaceAccessManager() *SessionWorkspaceAccessManager {
	return &SessionWorkspaceAccessManager{sessions: map[string]SessionWorkspaceAccess{}, now: time.Now, ttl: defaultSessionWorkspaceAccessTTL}
}

func (m *SessionWorkspaceAccessManager) CheckOrGrant(sessionID, workspaceID string) (WorkspaceAccess, SessionWorkspaceAccessDecision, int, error) {
	sessionID = strings.TrimSpace(sessionID)
	workspaceID = strings.TrimSpace(workspaceID)
	if sessionID == "" {
		return WorkspaceAccess{}, "", 0, errors.New("MCP session id is required")
	}
	if workspaceID == "" {
		return WorkspaceAccess{}, "", 0, errors.New("workspace id is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	m.purgeExpiredLocked(now)
	session, ok := m.sessions[sessionID]
	if !ok {
		session = SessionWorkspaceAccess{Workspaces: map[string]WorkspaceAccess{}, CreatedAt: now}
	}
	if grant, exists := session.Workspaces[workspaceID]; exists {
		grant.LastSeen = now
		session.Workspaces[workspaceID] = grant
		session.LastSeen = now
		m.sessions[sessionID] = session
		return grant, SessionWorkspaceAccessExisting, len(session.Workspaces), nil
	}
	grant := WorkspaceAccess{WorkspaceID: workspaceID, GrantedAt: now, LastSeen: now}
	session.Workspaces[workspaceID] = grant
	session.LastSeen = now
	m.sessions[sessionID] = session
	return grant, SessionWorkspaceAccessNew, len(session.Workspaces), nil
}

func (m *SessionWorkspaceAccessManager) Lookup(sessionID string) (SessionWorkspaceAccess, bool) {
	if m == nil {
		return SessionWorkspaceAccess{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.purgeExpiredLocked(m.now())
	session, ok := m.sessions[strings.TrimSpace(sessionID)]
	if !ok {
		return SessionWorkspaceAccess{}, false
	}
	session.Workspaces = cloneWorkspaceAccess(session.Workspaces)
	return session, true
}

func (m *SessionWorkspaceAccessManager) Delete(sessionID string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	delete(m.sessions, strings.TrimSpace(sessionID))
	m.mu.Unlock()
}

func (m *SessionWorkspaceAccessManager) purgeExpiredLocked(now time.Time) int {
	if m.ttl <= 0 {
		return 0
	}
	removed := 0
	for sessionID, session := range m.sessions {
		if now.Sub(session.LastSeen) < m.ttl {
			continue
		}
		delete(m.sessions, sessionID)
		removed++
	}
	return removed
}

func (m *SessionWorkspaceAccessManager) PurgeExpired() int {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.purgeExpiredLocked(m.now())
}

func (m *SessionWorkspaceAccessManager) Count() int {
	if m == nil {
		return 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.purgeExpiredLocked(m.now())
	return len(m.sessions)
}

func cloneWorkspaceAccess(values map[string]WorkspaceAccess) map[string]WorkspaceAccess {
	result := make(map[string]WorkspaceAccess, len(values))
	for id, value := range values {
		result[id] = value
	}
	return result
}
