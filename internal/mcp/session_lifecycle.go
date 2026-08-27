package mcp

import (
	"crypto/rand"
	"encoding/hex"
)

type SessionLifecycle struct{ store *SessionStore }

func NewSessionLifecycle(store *SessionStore) *SessionLifecycle {
	return &SessionLifecycle{store: store}
}

func (s *SessionLifecycle) Create() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)
	s.store.Set(&Session{ID: id})
	return id
}

func (s *SessionLifecycle) Delete(id string) {}
