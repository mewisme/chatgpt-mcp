package runtimecontrol

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	shellruntime "go.mewis.me/chatgpt-mcp/internal/shell"
)

var (
	ErrExecutionFeedOverflow    = errors.New("execution feed overflowed")
	ErrExecutionFeedUnsupported = errors.New("execution feed unsupported by running server")
)

type ExecutionFeedStream struct {
	response *http.Response
	scanner  *bufio.Scanner
	snapshot shellruntime.ExecutionFeedSnapshot
}

func OpenExecutionFeed(ctx context.Context) (*ExecutionFeedStream, State, error) {
	state, err := Load()
	if err != nil {
		return nil, State{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+state.Address+"/executions/stream", nil)
	if err != nil {
		return nil, state, err
	}
	request.Header.Set("Authorization", "Bearer "+state.Token)
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	client := &http.Client{Transport: &http.Transport{Proxy: nil, DialContext: dialer.DialContext, ResponseHeaderTimeout: 5 * time.Second}}
	response, err := client.Do(request)
	if err != nil {
		return nil, state, fmt.Errorf("running server execution endpoint unavailable: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
		_ = response.Body.Close()
		if response.StatusCode == http.StatusNotFound {
			return nil, state, fmt.Errorf("%w: restart the running server to enable command execution streaming", ErrExecutionFeedUnsupported)
		}
		return nil, state, fmt.Errorf("runtime execution stream failed with HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	snapshot, err := readExecutionFeedReady(scanner)
	if err != nil {
		_ = response.Body.Close()
		return nil, state, err
	}
	return &ExecutionFeedStream{response: response, scanner: scanner, snapshot: snapshot}, state, nil
}

func readExecutionFeedReady(scanner *bufio.Scanner) (shellruntime.ExecutionFeedSnapshot, error) {
	eventType := ""
	var data strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if eventType == "ready" {
				var snapshot shellruntime.ExecutionFeedSnapshot
				if err := json.Unmarshal([]byte(data.String()), &snapshot); err != nil {
					return shellruntime.ExecutionFeedSnapshot{}, fmt.Errorf("decode execution feed ready frame: %w", err)
				}
				return snapshot, nil
			}
			eventType = ""
			data.Reset()
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := scanner.Err(); err != nil {
		return shellruntime.ExecutionFeedSnapshot{}, err
	}
	return shellruntime.ExecutionFeedSnapshot{}, io.EOF
}

func (stream *ExecutionFeedStream) Snapshot() shellruntime.ExecutionFeedSnapshot {
	if stream == nil {
		return shellruntime.ExecutionFeedSnapshot{Events: []shellruntime.ExecutionFeedEvent{}}
	}
	return stream.snapshot
}

func (stream *ExecutionFeedStream) Next() (shellruntime.ExecutionFeedEvent, error) {
	if stream == nil || stream.scanner == nil {
		return shellruntime.ExecutionFeedEvent{}, io.EOF
	}
	eventType := ""
	var data strings.Builder
	flush := func() (shellruntime.ExecutionFeedEvent, bool, error) {
		if eventType == "overflow" {
			return shellruntime.ExecutionFeedEvent{}, false, ErrExecutionFeedOverflow
		}
		if eventType != shellruntime.ExecutionEventStarted && eventType != shellruntime.ExecutionEventOutput && eventType != shellruntime.ExecutionEventCompleted {
			return shellruntime.ExecutionFeedEvent{}, false, nil
		}
		if strings.TrimSpace(data.String()) == "" {
			return shellruntime.ExecutionFeedEvent{}, false, nil
		}
		var event shellruntime.ExecutionFeedEvent
		if err := json.Unmarshal([]byte(data.String()), &event); err != nil {
			return shellruntime.ExecutionFeedEvent{}, false, fmt.Errorf("decode execution feed event: %w", err)
		}
		return event, true, nil
	}
	for stream.scanner.Scan() {
		line := stream.scanner.Text()
		if line == "" {
			event, ok, err := flush()
			if err != nil || ok {
				return event, err
			}
			eventType = ""
			data.Reset()
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventType = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := stream.scanner.Err(); err != nil {
		return shellruntime.ExecutionFeedEvent{}, err
	}
	return shellruntime.ExecutionFeedEvent{}, io.EOF
}

func (stream *ExecutionFeedStream) Close() error {
	if stream == nil || stream.response == nil || stream.response.Body == nil {
		return nil
	}
	return stream.response.Body.Close()
}
