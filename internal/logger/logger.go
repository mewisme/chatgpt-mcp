package logger

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

type Level int

const (
	Debug Level = iota
	Info
	Warn
	Error
)

type Field struct {
	Key   string
	Value any
}

func String(key, value string) Field  { return Field{Key: key, Value: value} }
func Int(key string, value int) Field { return Field{Key: key, Value: value} }
func Err(value error) Field           { return Field{Key: "error", Value: value} }

type Logger struct {
	Level Level
	Color bool
	Out   io.Writer
}

func New(level Level) *Logger { return &Logger{Level: level, Color: true, Out: os.Stdout} }

func (l *Logger) log(level Level, label, message string, fields ...Field) {
	if level < l.Level {
		return
	}
	levelText, levelColor := levelName(level)
	labelColor := componentColor(label)
	reset := "\033[0m"
	prefix := fmt.Sprintf("%s %s[%s]%s %s[%s]%s", time.Now().Format("2006-01-02 15:04:05"), levelColor, levelText, reset, labelColor, strings.ToUpper(label), reset)
	if !l.Color {
		prefix = fmt.Sprintf("%s %s [%s]", time.Now().Format("2006-01-02 15:04:05"), levelText, strings.ToUpper(label))
	}
	fmt.Fprint(l.Out, prefix, " ", message)
	for _, field := range fields {
		fmt.Fprintf(l.Out, " %s=%v", field.Key, field.Value)
	}
	fmt.Fprintln(l.Out)
}

func (l *Logger) Debug(label, message string, fields ...Field) {
	l.log(Debug, label, message, fields...)
}
func (l *Logger) Info(label, message string, fields ...Field) { l.log(Info, label, message, fields...) }
func (l *Logger) Warn(label, message string, fields ...Field) { l.log(Warn, label, message, fields...) }
func (l *Logger) Error(label, message string, fields ...Field) {
	l.log(Error, label, message, fields...)
}

func levelName(level Level) (string, string) {
	switch level {
	case Debug:
		return "DEBUG", "\033[90m"
	case Warn:
		return "WARN", "\033[33m"
	case Error:
		return "ERROR", "\033[31m"
	default:
		return "INFO", "\033[32m"
	}
}

func componentColor(label string) string {
	switch strings.ToUpper(label) {
	case "MCP":
		return "\033[36m"
	case "AUTH":
		return "\033[33m"
	case "TUNNEL":
		return "\033[35m"
	default:
		return "\033[34m"
	}
}
