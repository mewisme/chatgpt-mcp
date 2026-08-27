package activity

import "sync"

type Stream struct {
	mu   sync.RWMutex
	subs map[chan []byte]struct{}
}

func NewStream() *Stream { return &Stream{subs: map[chan []byte]struct{}{}} }

func (s *Stream) Subscribe() chan []byte {
	ch := make(chan []byte, 16)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	return ch
}

func (s *Stream) Publish(data []byte) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for ch := range s.subs {
		select {
		case ch <- data:
		default:
		}
	}
}
