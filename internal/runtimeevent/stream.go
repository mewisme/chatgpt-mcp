package runtimeevent

import (
	"sync"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/logger"
)

const defaultStreamBuffer = 64

type Stream struct {
	metadata Metadata
	mu       sync.RWMutex
	subs     map[chan Event]struct{}
	sequence uint64
}

func NewStream(metadata Metadata) *Stream {
	return &Stream{metadata: metadata, subs: map[chan Event]struct{}{}}
}

func (s *Stream) WriteEvent(event logger.Event) error {
	if s == nil {
		return nil
	}
	s.Publish(fromLoggerEvent(event, s.metadata))
	return nil
}

func (s *Stream) Publish(event Event) Event {
	if s == nil {
		return event
	}
	s.mu.Lock()
	s.sequence++
	event.Sequence = s.sequence
	for ch := range s.subs {
		select {
		case ch <- event:
		default:
		}
	}
	s.mu.Unlock()
	return event
}

func (s *Stream) Subscribe() chan Event {
	ch := make(chan Event, defaultStreamBuffer)
	if s == nil {
		close(ch)
		return ch
	}
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	return ch
}

func (s *Stream) Unsubscribe(ch chan Event) {
	if s == nil || ch == nil {
		return
	}
	s.mu.Lock()
	if _, ok := s.subs[ch]; ok {
		delete(s.subs, ch)
		close(ch)
	}
	s.mu.Unlock()
}

func (s *Stream) LatestSequence() uint64 {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sequence
}

type Recorder struct {
	Journal  *Journal
	Stream   *Stream
	metadata Metadata
	mu       sync.Mutex
	sequence uint64
}

func NewRecorder(journal *Journal, metadata Metadata) *Recorder {
	return &Recorder{Journal: journal, Stream: NewStream(metadata), metadata: metadata}
}

func (r *Recorder) WriteEvent(event logger.Event) error {
	if r == nil {
		return nil
	}
	return r.Record(fromLoggerEvent(event, r.metadata))
}

func (r *Recorder) Record(value Event) error {
	if r == nil {
		return nil
	}
	if value.Time.IsZero() {
		value.Time = time.Now().UTC()
	} else {
		value.Time = value.Time.UTC()
	}
	if value.RunID == "" {
		value.RunID = r.metadata.RunID
	}
	if value.PID == 0 {
		value.PID = r.metadata.PID
	}
	if !value.Managed {
		value.Managed = r.metadata.Managed
	}
	if value.ServiceID == "" {
		value.ServiceID = r.metadata.ServiceID
	}
	if value.ServiceScope == "" {
		value.ServiceScope = r.metadata.ServiceScope
	}
	value.Message = sanitizeString(value.Message)
	value.Error = sanitizeString(value.Error)
	value.WorkspaceID = sanitizeString(value.WorkspaceID)
	value.Tool = sanitizeString(value.Tool)
	value.Method = sanitizeString(value.Method)
	value.Source = sanitizeString(value.Source)
	value.Status = sanitizeString(value.Status)
	for index := range value.Fields {
		value.Fields[index].Value = sanitizeValue(value.Fields[index].Key, value.Fields[index].Value)
	}
	r.mu.Lock()
	r.sequence++
	value.Sequence = r.sequence
	err := r.Journal.Append(value)
	r.Stream.mu.Lock()
	if r.Stream.sequence < value.Sequence {
		r.Stream.sequence = value.Sequence
	}
	for ch := range r.Stream.subs {
		select {
		case ch <- value:
		default:
		}
	}
	r.Stream.mu.Unlock()
	r.mu.Unlock()
	return err
}
