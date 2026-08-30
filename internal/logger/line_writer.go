package logger

import (
	"bytes"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"
)

const maxBufferedLogLine = 64 << 10

type componentLineWriter struct {
	mu        sync.Mutex
	log       *Logger
	component string
	pending   []byte
}

func (l *Logger) LineWriter(component string) io.Writer {
	if l == nil {
		return io.Discard
	}
	component = strings.TrimSpace(component)
	if component == "" {
		component = "LOG"
	}
	return &componentLineWriter{log: l, component: component}
}

func (w *componentLineWriter) Write(p []byte) (int, error) {
	n := len(p)
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending = append(w.pending, p...)
	for {
		index := bytes.IndexByte(w.pending, '\n')
		if index < 0 {
			break
		}
		w.emit(strings.TrimSuffix(string(w.pending[:index]), "\r"))
		w.pending = w.pending[index+1:]
	}
	if len(w.pending) > maxBufferedLogLine {
		w.emit(string(w.pending))
		w.pending = nil
	}
	if len(w.pending) == 0 {
		w.pending = nil
	}
	return n, nil
}

func (w *componentLineWriter) emit(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	event, ok := parseStructuredLine(line)
	if !ok {
		w.log.Emit(Event{Level: Info, Name: "diagnostic.raw", Message: redactRawLine(line), Component: w.component, Visibility: VisibilityDebug})
		return
	}
	if event.Component == "" {
		event.Component = w.component
	} else if !strings.EqualFold(event.Component, w.component) {
		event.Fields = append(event.Fields, WithDebug("stream_component", w.component))
	}
	classifyDiagnosticEvent(&event)
	if event.Name == "" {
		event.Name = "diagnostic." + slug(event.Message)
	}
	w.log.Emit(event)
}

func parseStructuredLine(line string) (Event, bool) {
	event := Event{Level: Info, Kind: KindInfo, Visibility: VisibilityDebug}
	structured := false
	for _, token := range splitStructuredTokens(line) {
		key, value, ok := strings.Cut(token, "=")
		if !ok || strings.TrimSpace(key) == "" {
			continue
		}
		key = strings.TrimSpace(key)
		value = decodeStructuredValue(value)
		switch strings.ToLower(key) {
		case "time", "timestamp", "ts":
			structured = true
			if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
				event.Time = parsed
			} else {
				event.Fields = append(event.Fields, diagnosticField(key, value))
			}
		case "level", "lvl":
			structured = true
			event.Level = parseStructuredLevel(value)
			event.Kind = kindForLevel(event.Level)
		case "msg", "message":
			structured = true
			event.Message = value
		case "event", "event_name":
			structured = true
			event.Name = value
		case "component":
			structured = true
			event.Component = value
		case "error", "err":
			structured = true
			event.Err = errors.New(redactStructuredField(key, value))
		case "source":
			structured = true
			event.Fields = append(event.Fields, diagnosticField(key, redactStructuredField(key, value)))
		default:
			event.Fields = append(event.Fields, diagnosticField(key, redactStructuredField(key, value)))
		}
	}
	if !structured || strings.TrimSpace(event.Message) == "" {
		return Event{}, false
	}
	return event, true
}

func classifyDiagnosticEvent(event *Event) {
	message := strings.ToLower(strings.TrimSpace(event.Message))
	switch {
	case strings.Contains(message, "route resolved"):
		event.Name = "tunnel.route.resolved"
		event.Visibility = VisibilityVerbose
	default:
		event.Visibility = VisibilityDebug
	}
}

func diagnosticField(key, value string) Field {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "tunnel_id", "channel", "transport", "route_kind", "route_name", "route_mode", "target_host":
		return WithVerbose(key, value)
	default:
		return WithDebug(key, value)
	}
}

func redactRawLine(line string) string {
	tokens := splitStructuredTokens(line)
	for index, token := range tokens {
		key, value, ok := strings.Cut(token, "=")
		if !ok {
			continue
		}
		decoded := decodeStructuredValue(value)
		redacted := redactStructuredField(key, decoded)
		if redacted != decoded {
			tokens[index] = key + "=" + redacted
		}
	}
	return strings.Join(tokens, " ")
}

func splitStructuredTokens(line string) []string {
	var tokens []string
	start := -1
	quoted := false
	escaped := false
	for index, char := range line {
		if start < 0 {
			if char == ' ' || char == '\t' {
				continue
			}
			start = index
		}
		if escaped {
			escaped = false
			continue
		}
		if quoted && char == '\\' {
			escaped = true
			continue
		}
		if char == '"' {
			quoted = !quoted
			continue
		}
		if !quoted && (char == ' ' || char == '\t') {
			tokens = append(tokens, line[start:index])
			start = -1
		}
	}
	if start >= 0 {
		tokens = append(tokens, line[start:])
	}
	return tokens
}

func decodeStructuredValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		if decoded, err := strconv.Unquote(value); err == nil {
			return decoded
		}
	}
	return value
}

func parseStructuredLevel(value string) Level {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "DEBUG", "DBG":
		return Debug
	case "WARN", "WARNING", "WRN":
		return Warn
	case "ERROR", "ERR":
		return Error
	default:
		return Info
	}
}

func redactStructuredField(key, value string) string {
	normalized := strings.NewReplacer("-", "_", ".", "_").Replace(strings.ToLower(key))
	for _, sensitive := range []string{"authorization", "api_key", "apikey", "password", "secret", "token"} {
		if strings.Contains(normalized, sensitive) {
			return "[redacted]"
		}
	}
	return value
}
