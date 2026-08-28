package activity

import (
	"encoding/json"
	"sync"
)

const defaultRecentLimit = 200

type Stream struct {
	mu        sync.RWMutex
	subs      map[chan Event]struct{}
	recent    []Event
	maxRecent int
}

func NewStream() *Stream {
	return &Stream{subs: map[chan Event]struct{}{}, maxRecent: defaultRecentLimit}
}

func (s *Stream) Subscribe() chan Event {
	ch, _ := s.SubscribeWithRecent(0)
	return ch
}

func (s *Stream) SubscribeWithRecent(limit int) (chan Event, []Event) {
	ch := make(chan Event, 32)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	recent := recentEvents(s.recent, limit)
	s.mu.Unlock()
	return ch, recent
}

func (s *Stream) Unsubscribe(ch chan Event) {
	s.mu.Lock()
	if _, ok := s.subs[ch]; ok {
		delete(s.subs, ch)
		close(ch)
	}
	s.mu.Unlock()
}

func (s *Stream) Recent(limit int) []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return recentEvents(s.recent, limit)
}

func (s *Stream) Publish(event Event) {
	event = normalizeEvent(event)
	s.mu.Lock()
	s.recent = append(s.recent, event)
	if overflow := len(s.recent) - s.maxRecent; overflow > 0 {
		s.recent = append([]Event(nil), s.recent[overflow:]...)
	}
	for ch := range s.subs {
		select {
		case ch <- event:
		default:
		}
	}
	s.mu.Unlock()
}

func recentEvents(events []Event, limit int) []Event {
	if limit <= 0 || len(events) == 0 {
		return nil
	}
	if limit > len(events) {
		limit = len(events)
	}
	result := make([]Event, limit)
	copy(result, events[len(events)-limit:])
	return result
}

func Encode(event Event) string {
	data, _ := json.Marshal(event)
	return string(data)
}
