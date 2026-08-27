package mcp

import (
	"crypto/rand"
	"encoding/hex"

	"go.mewis.me/chatgpt-mcp/internal/activity"
)

type Lifecycle struct {
	store    *SessionStore
	activity *activity.Stream
}

func NewLifecycle(store *SessionStore, streams ...*activity.Stream) *Lifecycle {
	var stream *activity.Stream
	if len(streams) > 0 {
		stream = streams[0]
	}
	return &Lifecycle{store: store, activity: stream}
}

func (s *Lifecycle) Create() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	id := hex.EncodeToString(b)
	s.store.Set(NewSession(id))
	s.emit("session.created", id)
	return id
}

func (s *Lifecycle) Delete(id string) {
	s.store.Delete(id)
	s.emit("session.deleted", id)
}

func (s *Lifecycle) emit(kind, message string) {
	if s.activity != nil {
		s.activity.Publish(activity.Event{Kind: kind, Message: message})
	}
}
