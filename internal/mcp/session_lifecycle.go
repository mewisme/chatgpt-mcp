package mcp

import (
	"crypto/rand"
	"encoding/hex"
)

type Lifecycle struct {
	store *SessionStore
}

func NewLifecycle(store *SessionStore) *Lifecycle { return &Lifecycle{store: store} }

func (s *Lifecycle) Create() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)
	s.store.Set(&Session{ID: id})
	return id
}

func (s *Lifecycle) Delete(id string) { s.store.Delete(id) }
