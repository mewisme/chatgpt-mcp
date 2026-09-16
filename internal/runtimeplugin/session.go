package runtimeplugin

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Spec struct {
	ID         string
	Version    string
	Entrypoint string
	WorkDir    string
	DataDir    string
	ExtraEnv   []string
}

type Session struct {
	spec Spec
	cmd  *exec.Cmd

	stdin  io.WriteCloser
	cancel context.CancelFunc

	mu       sync.Mutex
	nextID   uint64
	pending  map[uint64]chan Envelope
	events   chan Event
	err      error
	exited   chan struct{}
	stderr   boundedBuffer
	closed   bool
	stopping bool
}

func Start(ctx context.Context, spec Spec) (*Session, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	entrypoint := filepath.Clean(strings.TrimSpace(spec.Entrypoint))
	if entrypoint == "" {
		return nil, errors.New("runtime plugin entrypoint is required")
	}
	if !filepath.IsAbs(entrypoint) {
		return nil, errors.New("runtime plugin entrypoint must be an absolute path")
	}
	workDir := strings.TrimSpace(spec.WorkDir)
	if workDir == "" {
		workDir = filepath.Dir(entrypoint)
	}
	runCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	cmd := exec.CommandContext(runCtx, entrypoint)
	cmd.Dir = workDir
	cmd.Env = safeProcessEnvironment(append([]string{
		EnvPluginID + "=" + spec.ID,
		EnvPluginVersion + "=" + spec.Version,
		EnvPluginDataDir + "=" + spec.DataDir,
	}, spec.ExtraEnv...)...)
	configureProcess(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		cancel()
		return nil, err
	}
	session := &Session{spec: spec, cmd: cmd, stdin: stdin, cancel: cancel, pending: map[uint64]chan Envelope{}, events: make(chan Event, 16), exited: make(chan struct{})}
	cmd.Stderr = &session.stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		cancel()
		return nil, err
	}
	go session.read(stdout)
	go session.wait()
	return session, nil
}

func (s *Session) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := MethodTimeout(method)
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 && remaining < timeout {
			timeout = remaining
		}
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	payload, err := marshalParams(params)
	if err != nil {
		return nil, err
	}
	reply, err := s.roundTrip(callCtx, method, payload)
	if err != nil {
		return nil, err
	}
	if reply.Error != nil {
		return nil, *reply.Error
	}
	return reply.Result, nil
}

func (s *Session) Events() <-chan Event { return s.events }

func (s *Session) CommandPath() string {
	if s == nil || s.cmd == nil || len(s.cmd.Args) == 0 {
		return ""
	}
	return s.cmd.Args[0]
}

func (s *Session) Err() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *Session) Shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if s == nil {
		return nil
	}
	s.mu.Lock()
	s.stopping = true
	s.mu.Unlock()
	_, _ = s.Call(ctx, MethodShutdown, struct{}{})
	s.closeStdin()
	timer := time.NewTimer(MethodTimeout(MethodShutdown))
	defer timer.Stop()
	select {
	case <-s.exited:
		return s.Err()
	case <-ctx.Done():
		_ = signalProcess(s.cmd, true)
		<-s.exited
		return ctx.Err()
	case <-timer.C:
		_ = signalProcess(s.cmd, true)
		<-s.exited
		return s.Err()
	}
}

func (s *Session) roundTrip(ctx context.Context, method string, params json.RawMessage) (Envelope, error) {
	s.mu.Lock()
	if s.closed || s.err != nil {
		err := s.err
		s.mu.Unlock()
		if err == nil {
			err = ErrClosed
		}
		return Envelope{}, err
	}
	s.nextID++
	id := s.nextID
	reply := make(chan Envelope, 1)
	s.pending[id] = reply
	line, err := encodeEnvelope(Envelope{ID: id, Method: method, Params: params})
	if err != nil {
		delete(s.pending, id)
		s.mu.Unlock()
		return Envelope{}, err
	}
	_, err = s.stdin.Write(line)
	s.mu.Unlock()
	if err != nil {
		s.fail(fmt.Errorf("write runtime plugin stdin: %w", err), false)
		return Envelope{}, err
	}
	select {
	case env, ok := <-reply:
		if !ok {
			if err := s.Err(); err != nil {
				return Envelope{}, err
			}
			return Envelope{}, ErrClosed
		}
		return env, nil
	case <-ctx.Done():
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Envelope{}, fmt.Errorf("%w: %s", ErrTimeout, method)
		}
		return Envelope{}, ctx.Err()
	case <-s.exited:
		if err := s.Err(); err != nil {
			return Envelope{}, err
		}
		return Envelope{}, ErrClosed
	}
}

func (s *Session) read(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 4<<10), MaxMessageBytes+1)
	for scanner.Scan() {
		env, err := decodeEnvelope(scanner.Bytes())
		if err != nil {
			s.fail(err, true)
			return
		}
		s.dispatch(env)
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			s.fail(ErrTooLarge, true)
			return
		}
		s.fail(err, false)
	}
}

func (s *Session) dispatch(env Envelope) {
	if env.Event != "" {
		select {
		case s.events <- Event{Name: env.Event, Data: bytes.Clone(env.Data)}:
		default:
		}
		return
	}
	s.mu.Lock()
	reply := s.pending[env.ID]
	delete(s.pending, env.ID)
	s.mu.Unlock()
	if reply == nil {
		return
	}
	reply <- env
}

func (s *Session) wait() {
	err := s.cmd.Wait()
	s.fail(err, false)
}

func (s *Session) fail(err error, protocol bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	switch {
	case protocol:
		s.err = err
	case s.stopping:
	case err != nil && s.cmd.ProcessState != nil && !s.cmd.ProcessState.Success():
		s.err = fmt.Errorf("%w: %v", ErrCrashed, err)
	case err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, os.ErrProcessDone) && !errors.Is(err, context.Canceled):
		s.err = err
	}
	for id, reply := range s.pending {
		close(reply)
		delete(s.pending, id)
	}
	close(s.events)
	s.cancel()
	select {
	case <-s.exited:
	default:
		close(s.exited)
	}
}

func (s *Session) closeStdin() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stdin != nil {
		_ = s.stdin.Close()
		s.stdin = nil
	}
}

type boundedBuffer struct {
	mu       sync.Mutex
	buf      bytes.Buffer
	exceeded bool
}

func (b *boundedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	original := len(data)
	remaining := MaxStderrBytes + 1 - b.buf.Len()
	if remaining > 0 {
		if len(data) > remaining {
			data = data[:remaining]
		}
		_, _ = b.buf.Write(data)
	}
	if b.buf.Len() > MaxStderrBytes || len(data) < original {
		b.exceeded = true
	}
	return original, nil
}

func (b *boundedBuffer) exceededLimit() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.exceeded
}

func marshalParams(params any) (json.RawMessage, error) {
	if params == nil {
		return json.RawMessage("{}"), nil
	}
	if raw, ok := params.(json.RawMessage); ok {
		if len(raw) == 0 {
			return json.RawMessage("{}"), nil
		}
		return raw, nil
	}
	data, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	return data, nil
}
