package runtimecontrol

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/runtimeevent"
)

type EventStream struct {
	response *http.Response
	scanner  *bufio.Scanner
}

func OpenEvents(ctx context.Context) (*EventStream, State, error) {
	state, err := Load()
	if err != nil {
		return nil, State{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+state.Address+"/events", nil)
	if err != nil {
		return nil, State{}, err
	}
	request.Header.Set("Authorization", "Bearer "+state.Token)
	dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
	client := &http.Client{Transport: &http.Transport{Proxy: nil, DialContext: dialer.DialContext, ResponseHeaderTimeout: 5 * time.Second}}
	response, err := client.Do(request)
	if err != nil {
		return nil, State{}, fmt.Errorf("running server control endpoint unavailable: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
		_ = response.Body.Close()
		return nil, State{}, fmt.Errorf("runtime event stream failed with HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	scanner := bufio.NewScanner(response.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	return &EventStream{response: response, scanner: scanner}, state, nil
}

func (stream *EventStream) Next() (runtimeevent.Event, error) {
	if stream == nil || stream.scanner == nil {
		return runtimeevent.Event{}, io.EOF
	}
	eventType := ""
	var data strings.Builder
	flush := func() (runtimeevent.Event, bool, error) {
		if eventType != "runtime" || strings.TrimSpace(data.String()) == "" {
			return runtimeevent.Event{}, false, nil
		}
		var event runtimeevent.Event
		if err := json.Unmarshal([]byte(data.String()), &event); err != nil {
			return runtimeevent.Event{}, false, fmt.Errorf("decode runtime event: %w", err)
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
		return runtimeevent.Event{}, err
	}
	return runtimeevent.Event{}, io.EOF
}

func (stream *EventStream) Close() error {
	if stream == nil || stream.response == nil || stream.response.Body == nil {
		return nil
	}
	return stream.response.Body.Close()
}
