package runtimeplugin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

const (
	Version          = 1
	MaxMessageBytes  = 64 << 10
	MaxStderrBytes   = 32 << 10
	EnvPluginID      = "CGM_PLUGIN_ID"
	EnvPluginVersion = "CGM_PLUGIN_VERSION"
	EnvPluginDataDir = "CGM_PLUGIN_DATA_DIR"
)

const (
	MethodDescribe = "describe"
	MethodStatus   = "status"
	MethodStart    = "start"
	MethodStop     = "stop"
	MethodShutdown = "shutdown"
	EventStatus    = "status"
	EventLog       = "log"
)

const (
	CodeProtocolVersion = "protocol_version"
	CodeMalformed       = "malformed"
	CodeUnknownMethod   = "unknown_method"
	CodeTimeout         = "timeout"
	CodeCrashed         = "crashed"
	CodeClosed          = "closed"
	CodeTooLarge        = "too_large"
	CodeInvalid         = "invalid"
)

var (
	ErrProtocolVersion = errors.New("runtime plugin protocol version mismatch")
	ErrMalformed       = errors.New("runtime plugin protocol message is malformed")
	ErrTooLarge        = errors.New("runtime plugin protocol message exceeds size limit")
	ErrUnknownMethod   = errors.New("runtime plugin method is unknown")
	ErrCrashed         = errors.New("runtime plugin process crashed")
	ErrClosed          = errors.New("runtime plugin session is closed")
	ErrTimeout         = errors.New("runtime plugin request timed out")
)

type Envelope struct {
	Version int             `json:"version"`
	ID      uint64          `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Event   string          `json:"event,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (err Error) Error() string {
	if err.Code == "" {
		return err.Message
	}
	if err.Message == "" {
		return err.Code
	}
	return err.Code + ": " + err.Message
}

func MethodTimeout(method string) time.Duration {
	switch method {
	case MethodStart:
		return 15 * time.Second
	case MethodStop:
		return 10 * time.Second
	case MethodShutdown:
		return 3 * time.Second
	default:
		return 2 * time.Second
	}
}

func decodeEnvelope(line []byte) (Envelope, error) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return Envelope{}, fmt.Errorf("%w: empty line", ErrMalformed)
	}
	if len(line) > MaxMessageBytes {
		return Envelope{}, ErrTooLarge
	}
	var env Envelope
	if err := json.Unmarshal(line, &env); err != nil {
		return Envelope{}, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	if env.Version != Version {
		return Envelope{}, fmt.Errorf("%w: %d", ErrProtocolVersion, env.Version)
	}
	switch {
	case env.Method != "":
		if env.ID == 0 {
			return Envelope{}, fmt.Errorf("%w: request id is required", ErrMalformed)
		}
	case env.Event != "":
		if env.ID != 0 || env.Method != "" || env.Result != nil || env.Error != nil {
			return Envelope{}, fmt.Errorf("%w: event fields", ErrMalformed)
		}
	case env.ID != 0:
		if env.Result == nil && env.Error == nil {
			return Envelope{}, fmt.Errorf("%w: response result or error is required", ErrMalformed)
		}
	default:
		return Envelope{}, fmt.Errorf("%w: unclassified message", ErrMalformed)
	}
	return env, nil
}

func encodeEnvelope(env Envelope) ([]byte, error) {
	env.Version = Version
	data, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}
	if len(data) > MaxMessageBytes {
		return nil, ErrTooLarge
	}
	return append(data, '\n'), nil
}
