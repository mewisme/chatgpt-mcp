package approval

import (
	"sync"
	"time"
)

const (
	EventRequested = "approval.requested"
	EventApproved  = "approval.approved"
	EventDenied    = "approval.denied"
	EventExpired   = "approval.expired"
	EventCancelled = "approval.cancelled"
	EventConsumed  = "approval.consumed"
	EventMismatch  = "approval.mismatch"
)

type Event struct {
	Sequence    uint64    `json:"sequence,omitempty"`
	Name        string    `json:"name"`
	RequestID   string    `json:"request_id"`
	WorkspaceID string    `json:"workspace_id"`
	SessionHash string    `json:"session_hash,omitempty"`
	Source      string    `json:"source,omitempty"`
	TargetTool  string    `json:"target_tool"`
	Title       string    `json:"title"`
	Status      Status    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	RetryUntil  time.Time `json:"retry_until,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
}

type EventObserver func(Event)

type EventOverflow struct {
	DroppedSequence uint64 `json:"dropped_sequence"`
}

type EventSubscription struct {
	Events   chan Event
	Overflow chan EventOverflow
	overflow bool
	closed   bool
}

type EventStream struct {
	mu           sync.RWMutex
	subs         map[chan Event]*EventSubscription
	recent       []Event
	maxRecent    int
	nextSequence uint64
}

func newEventStream() *EventStream {
	return &EventStream{subs: map[chan Event]*EventSubscription{}, maxRecent: 64}
}

func (s *EventStream) Subscribe() *EventSubscription {
	if s == nil {
		return nil
	}
	sub := &EventSubscription{Events: make(chan Event, 16), Overflow: make(chan EventOverflow, 1)}
	s.mu.Lock()
	s.subs[sub.Events] = sub
	s.mu.Unlock()
	return sub
}

func (s *EventStream) Unsubscribe(sub *EventSubscription) {
	if s == nil || sub == nil {
		return
	}
	s.mu.Lock()
	if current := s.subs[sub.Events]; current != nil && !current.closed {
		delete(s.subs, sub.Events)
		close(current.Events)
		close(current.Overflow)
		current.closed = true
	}
	s.mu.Unlock()
}

func (s *EventStream) Recent(limit int) []Event {
	if s == nil || limit <= 0 {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit > len(s.recent) {
		limit = len(s.recent)
	}
	result := make([]Event, limit)
	copy(result, s.recent[len(s.recent)-limit:])
	return result
}

func (s *EventStream) LatestSequence() uint64 {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.nextSequence
}

func (s *EventStream) Publish(event Event) {
	if s == nil {
		return
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	} else {
		event.Timestamp = event.Timestamp.UTC()
	}
	s.mu.Lock()
	s.nextSequence++
	event.Sequence = s.nextSequence
	s.recent = append(s.recent, event)
	if len(s.recent) > s.maxRecent {
		s.recent = append([]Event(nil), s.recent[len(s.recent)-s.maxRecent:]...)
	}
	for _, sub := range s.subs {
		if sub.closed || sub.overflow {
			continue
		}
		select {
		case sub.Events <- event:
		default:
			sub.overflow = true
			sub.Overflow <- EventOverflow{DroppedSequence: event.Sequence}
		}
	}
	s.mu.Unlock()
}
