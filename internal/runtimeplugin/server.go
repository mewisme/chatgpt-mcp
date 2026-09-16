package runtimeplugin

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
)

type Handler interface {
	Describe(ctx context.Context, params json.RawMessage) (any, error)
	Status(ctx context.Context, params json.RawMessage) (any, error)
	Start(ctx context.Context, params json.RawMessage) (any, error)
	Stop(ctx context.Context, params json.RawMessage) (any, error)
	Shutdown(ctx context.Context, params json.RawMessage) (any, error)
}

type Server struct {
	Handler Handler

	mu     sync.Mutex
	writer io.Writer
	closed bool
}

func Serve(ctx context.Context, stdin io.Reader, stdout io.Writer, handler Handler) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if handler == nil {
		return errors.New("runtime plugin handler is required")
	}
	server := &Server{Handler: handler, writer: stdout}
	scanner := bufio.NewScanner(stdin)
	scanner.Buffer(make([]byte, 0, 4<<10), MaxMessageBytes+1)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		env, err := decodeEnvelope(scanner.Bytes())
		if err != nil {
			return err
		}
		if env.Method == "" {
			continue
		}
		if err := server.handle(ctx, env); err != nil {
			return err
		}
		if env.Method == MethodShutdown {
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return ErrTooLarge
		}
		return err
	}
	return nil
}

func (s *Server) Emit(event string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return s.write(Envelope{Event: event, Data: payload})
}

func (s *Server) handle(ctx context.Context, env Envelope) error {
	result, err := s.dispatch(ctx, env.Method, env.Params)
	if err != nil {
		code := CodeInvalid
		switch {
		case errors.Is(err, ErrUnknownMethod):
			code = CodeUnknownMethod
		case errors.Is(err, ErrProtocolVersion):
			code = CodeProtocolVersion
		}
		return s.write(Envelope{ID: env.ID, Error: &Error{Code: code, Message: err.Error()}})
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return s.write(Envelope{ID: env.ID, Result: payload})
}

func (s *Server) dispatch(ctx context.Context, method string, params json.RawMessage) (any, error) {
	switch method {
	case MethodDescribe:
		return s.Handler.Describe(ctx, params)
	case MethodStatus:
		return s.Handler.Status(ctx, params)
	case MethodStart:
		return s.Handler.Start(ctx, params)
	case MethodStop:
		return s.Handler.Stop(ctx, params)
	case MethodShutdown:
		return s.Handler.Shutdown(ctx, params)
	default:
		return nil, ErrUnknownMethod
	}
}

func (s *Server) write(env Envelope) error {
	line, err := encodeEnvelope(env)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrClosed
	}
	_, err = s.writer.Write(line)
	return err
}
