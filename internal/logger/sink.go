package logger

type Sink interface {
	WriteEvent(Event) error
}

func (l *Logger) AddSink(sink Sink) {
	if l == nil || sink == nil {
		return
	}
	l.sinksMu.Lock()
	l.sinks = append(l.sinks, sink)
	l.sinksMu.Unlock()
}

func (l *Logger) writeSinks(event Event) {
	l.sinksMu.RLock()
	sinks := append([]Sink(nil), l.sinks...)
	l.sinksMu.RUnlock()
	for _, sink := range sinks {
		_ = sink.WriteEvent(event)
	}
}
