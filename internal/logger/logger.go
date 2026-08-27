package logger

import (
	"io"
	"os"
	"runtime/debug"

	"github.com/charmbracelet/log"
)

type Level = log.Level

const (
	Debug = log.DebugLevel
	Info  = log.InfoLevel
	Warn  = log.WarnLevel
	Error = log.ErrorLevel
)

type Logger struct{ value *log.Logger }

func New(level Level) *Logger {
	return &Logger{value: log.NewWithOptions(os.Stdout, log.Options{Level: level, ReportTimestamp: true})}
}

func NewWithWriter(level Level, writer io.Writer) *Logger {
	return &Logger{value: log.NewWithOptions(writer, log.Options{Level: level, ReportTimestamp: true})}
}

func (l *Logger) Debug(component, message string, fields ...any) {
	l.value.Debug(message, append([]any{"component", component}, fields...)...)
}
func (l *Logger) Info(component, message string, fields ...any) {
	l.value.Info(message, append([]any{"component", component}, fields...)...)
}
func (l *Logger) Warn(component, message string, fields ...any) {
	l.value.Warn(message, append([]any{"component", component}, fields...)...)
}
func (l *Logger) Error(component, message string, fields ...any) {
	l.value.Error(message, append([]any{"component", component}, fields...)...)
}

func Version() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		return info.Main.Version
	}
	return "dev"
}
