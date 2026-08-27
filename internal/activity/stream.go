package activity

import (
	"encoding/json"
	"sync"
)

type Stream struct {
	mu   sync.RWMutex
	subs map[chan Event]struct{}
}

func NewStream() *Stream { return &Stream{subs: map[chan Event]struct{}{}} }

func (s *Stream) Subscribe() chan Event {
	ch := make(chan Event, 32)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	return ch
}

func (s *Stream) Unsubscribe(ch chan Event) {
	s.mu.Lock()
	if _, ok := s.subs[ch]; ok {
		delete(s.subs, ch)
		close(ch)
	}
	s.mu.Unlock()
}

func (s *Stream) Publish(event Event) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for ch := range s.subs {
		select {
		case ch <- event:
		default:
		}
	}
}

func Encode(event Event) string {
	data, _ := json.Marshal(event)
	return string(data)
}
