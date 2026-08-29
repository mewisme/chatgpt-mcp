package logger

import (
	"bytes"
	"io"
	"strconv"
	"strings"
	"sync"
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
	level, message, fields, ok := parseStructuredLine(line)
	if !ok {
		w.log.Info(w.component, line)
		return
	}
	switch level {
	case Debug:
		w.log.Debug(w.component, message, fields...)
	case Warn:
		w.log.Warn(w.component, message, fields...)
	case Error:
		w.log.Error(w.component, message, fields...)
	default:
		w.log.Info(w.component, message, fields...)
	}
}

func parseStructuredLine(line string) (Level, string, []any, bool) {
	level := Info
	message := ""
	fields := make([]any, 0, 8)
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
		case "level", "lvl":
			structured = true
			level = parseStructuredLevel(value)
		case "msg", "message":
			structured = true
			message = value
		case "source":
			structured = true
		default:
			fields = append(fields, key, redactStructuredField(key, value))
		}
	}
	if !structured || strings.TrimSpace(message) == "" {
		return Info, "", nil, false
	}
	return level, message, fields, true
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
