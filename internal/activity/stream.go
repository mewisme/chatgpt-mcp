package activity

import (
	"encoding/json"
	"sync"
)

const (
	defaultRecentLimit      = 200
	defaultSubscriberBuffer = 32
)

type Overflow struct {
	DroppedSequence uint64 `json:"dropped_sequence"`
}

type Subscription struct {
	Events   chan Event
	Overflow chan Overflow
	overflow bool
	closed   bool
}

type Stream struct {
	mu           sync.RWMutex
	subs         map[chan Event]*Subscription
	recent       []Event
	maxRecent    int
	nextSequence uint64
}

func NewStream() *Stream {
	return &Stream{subs: map[chan Event]*Subscription{}, maxRecent: defaultRecentLimit}
}

func (s *Stream) Subscribe() chan Event {
	ch, _ := s.SubscribeWithRecent(0)
	return ch
}

func (s *Stream) SubscribeWithRecent(limit int) (chan Event, []Event) {
	sub, recent := s.SubscribeDetailed(limit)
	return sub.Events, recent
}

func (s *Stream) SubscribeDetailed(limit int) (*Subscription, []Event) {
	sub := &Subscription{Events: make(chan Event, defaultSubscriberBuffer), Overflow: make(chan Overflow, 1)}
	s.mu.Lock()
	s.subs[sub.Events] = sub
	recent := recentEvents(s.recent, limit)
	s.mu.Unlock()
	return sub, recent
}

func (s *Stream) Unsubscribe(ch chan Event) {
	s.mu.Lock()
	if sub, ok := s.subs[ch]; ok {
		s.unsubscribeLocked(sub)
	}
	s.mu.Unlock()
}

func (s *Stream) UnsubscribeDetailed(sub *Subscription) {
	if sub == nil {
		return
	}
	s.mu.Lock()
	s.unsubscribeLocked(sub)
	s.mu.Unlock()
}

func (s *Stream) unsubscribeLocked(sub *Subscription) {
	if sub.closed {
		return
	}
	delete(s.subs, sub.Events)
	close(sub.Events)
	close(sub.Overflow)
	sub.closed = true
}

func (s *Stream) Recent(limit int) []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return recentEvents(s.recent, limit)
}

func (s *Stream) LatestSequence() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.nextSequence
}

func (s *Stream) Publish(event Event) {
	event = normalizeEvent(event)
	s.mu.Lock()
	s.nextSequence++
	event.Sequence = s.nextSequence
	s.recent = append(s.recent, event)
	if overflow := len(s.recent) - s.maxRecent; overflow > 0 {
		s.recent = append([]Event(nil), s.recent[overflow:]...)
	}
	for _, sub := range s.subs {
		if sub.overflow || sub.closed {
			continue
		}
		select {
		case sub.Events <- event:
		default:
			sub.overflow = true
			sub.Overflow <- Overflow{DroppedSequence: event.Sequence}
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
