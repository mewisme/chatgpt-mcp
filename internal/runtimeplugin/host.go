package runtimeplugin

import (
	"context"
	"sync"
)

type Host struct {
	mu       sync.Mutex
	sessions map[string]*Session
}

func NewHost() *Host { return &Host{sessions: map[string]*Session{}} }

func (h *Host) Ensure(ctx context.Context, spec Spec) (*Session, error) {
	if h == nil {
		return Start(ctx, spec)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if session := h.liveLocked(spec.ID); session != nil {
		return session, nil
	}
	session, err := Start(ctx, spec)
	if err != nil {
		return nil, err
	}
	if h.sessions == nil {
		h.sessions = map[string]*Session{}
	}
	h.sessions[spec.ID] = session
	return session, nil
}

func (h *Host) Get(id string) (*Session, bool) {
	if h == nil {
		return nil, false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	session := h.liveLocked(id)
	return session, session != nil
}

func (h *Host) Shutdown(ctx context.Context, id string) error {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	session := h.sessions[id]
	delete(h.sessions, id)
	h.mu.Unlock()
	if session == nil {
		return nil
	}
	return session.Shutdown(ctx)
}

func (h *Host) Close(ctx context.Context) {
	if h == nil {
		return
	}
	h.mu.Lock()
	sessions := h.sessions
	h.sessions = map[string]*Session{}
	h.mu.Unlock()
	for _, session := range sessions {
		_ = session.Shutdown(ctx)
	}
}

func (h *Host) liveLocked(id string) *Session {
	session := h.sessions[id]
	if session == nil || session.Err() != nil {
		return nil
	}
	select {
	case <-session.exited:
		return nil
	default:
		return session
	}
}
