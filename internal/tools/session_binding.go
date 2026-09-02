package tools

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var ErrSessionWorkspaceMismatch = errors.New("MCP session workspace mismatch")

type SessionBindingDecision string

const (
	SessionBindingNew      SessionBindingDecision = "new"
	SessionBindingExisting SessionBindingDecision = "existing"
	SessionBindingDenied   SessionBindingDecision = "denied"
)

type SessionBinding struct {
	WorkspaceID string
	BoundAt     time.Time
	LastSeen    time.Time
}

type SessionWorkspaceBinder struct {
	mu       sync.Mutex
	bindings map[string]SessionBinding
	now      func() time.Time
}

func NewSessionWorkspaceBinder() *SessionWorkspaceBinder {
	return &SessionWorkspaceBinder{bindings: map[string]SessionBinding{}, now: time.Now}
}

func (b *SessionWorkspaceBinder) CheckOrBind(sessionID, workspaceID string) (SessionBinding, SessionBindingDecision, error) {
	sessionID = strings.TrimSpace(sessionID)
	workspaceID = strings.TrimSpace(workspaceID)
	if sessionID == "" {
		return SessionBinding{}, SessionBindingDenied, errors.New("MCP session id is required")
	}
	if workspaceID == "" {
		return SessionBinding{}, SessionBindingDenied, errors.New("workspace id is required")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	if binding, ok := b.bindings[sessionID]; ok {
		if binding.WorkspaceID != workspaceID {
			return binding, SessionBindingDenied, fmt.Errorf("%w: session is bound to workspace %s and cannot access %s", ErrSessionWorkspaceMismatch, binding.WorkspaceID, workspaceID)
		}
		binding.LastSeen = now
		b.bindings[sessionID] = binding
		return binding, SessionBindingExisting, nil
	}
	binding := SessionBinding{WorkspaceID: workspaceID, BoundAt: now, LastSeen: now}
	b.bindings[sessionID] = binding
	return binding, SessionBindingNew, nil
}

func (b *SessionWorkspaceBinder) Lookup(sessionID string) (SessionBinding, bool) {
	if b == nil {
		return SessionBinding{}, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	binding, ok := b.bindings[strings.TrimSpace(sessionID)]
	return binding, ok
}

func (b *SessionWorkspaceBinder) Delete(sessionID string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	delete(b.bindings, strings.TrimSpace(sessionID))
	b.mu.Unlock()
}
