package logger

import (
	"fmt"
	"strings"
	"time"
)

type Level uint8

const (
	Debug Level = iota
	Info
	Warn
	Error
)

type Mode uint8

const (
	ModeDefault Mode = iota
	ModeVerbose
	ModeDebug
)

type TimeMode uint8

const (
	TimeAuto TimeMode = iota
	TimeShow
	TimeHide
)

type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"
)

type Visibility uint8

const (
	VisibilityDefault Visibility = iota
	VisibilityVerbose
	VisibilityDebug
)

type Kind uint8

const (
	KindInfo Kind = iota
	KindAction
	KindSuccess
	KindWarning
	KindError
)

type Field struct {
	Key        string
	Value      any
	Visibility Visibility
}

type Event struct {
	Time         time.Time
	Level        Level
	Name         string
	Message      string
	Fields       []Field
	Err          error
	Component    string
	Kind         Kind
	Visibility   Visibility
	RunID        string
	PID          int
	Managed      bool
	ServiceID    string
	ServiceScope string
}

func ParseFormat(value string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(FormatText):
		return FormatText, nil
	case string(FormatJSON):
		return FormatJSON, nil
	default:
		return "", fmt.Errorf("unsupported log format %q; use text or json", value)
	}
}

func ModeFor(verbose, debug bool) Mode {
	if debug {
		return ModeDebug
	}
	if verbose {
		return ModeVerbose
	}
	return ModeDefault
}

func With(key string, value any) Field { return Field{Key: key, Value: value} }
func WithVerbose(key string, value any) Field {
	return Field{Key: key, Value: value, Visibility: VisibilityVerbose}
}
func WithDebug(key string, value any) Field {
	return Field{Key: key, Value: value, Visibility: VisibilityDebug}
}

func (level Level) String() string {
	switch level {
	case Debug:
		return "debug"
	case Warn:
		return "warn"
	case Error:
		return "error"
	default:
		return "info"
	}
}

func (kind Kind) String() string {
	switch kind {
	case KindAction:
		return "action"
	case KindSuccess:
		return "success"
	case KindWarning:
		return "warning"
	case KindError:
		return "error"
	default:
		return "info"
	}
}
