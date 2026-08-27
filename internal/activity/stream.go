package activity

import "sync"

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
