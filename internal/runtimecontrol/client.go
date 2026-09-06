package runtimecontrol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/config"
)

const FileName = ".runtime-control.json"

type State struct {
	PID          int       `json:"pid"`
	Address      string    `json:"address"`
	Token        string    `json:"token"`
	RunID        string    `json:"run_id,omitempty"`
	Managed      bool      `json:"managed,omitempty"`
	ServiceID    string    `json:"service_id,omitempty"`
	ServiceScope string    `json:"service_scope,omitempty"`
	StartedAt    time.Time `json:"started_at,omitempty"`
	ConfigRoot   string    `json:"config_root"`
}

func Path() string { return filepath.Join(config.RootPath(), FileName) }

func Load() (State, error) {
	data, err := os.ReadFile(Path())
	if err != nil {
		if os.IsNotExist(err) {
			return State{}, errors.New("no running server found for this config directory")
		}
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, fmt.Errorf("decode runtime control state: %w", err)
	}
	if state.PID <= 0 || strings.TrimSpace(state.Token) == "" {
		return State{}, errors.New("runtime control state is invalid")
	}
	host, _, err := net.SplitHostPort(state.Address)
	if err != nil {
		return State{}, errors.New("runtime control address is invalid")
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil || !ip.IsLoopback() {
		return State{}, errors.New("runtime control address is not loopback")
	}
	return state, nil
}

func Request(ctx context.Context, method, path string, input, output any) (State, error) {
	state, err := Load()
	if err != nil {
		return State{}, err
	}
	path = strings.TrimSpace(path)
	if !strings.HasPrefix(path, "/") {
		return State{}, errors.New("runtime control path must be absolute")
	}
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return State{}, fmt.Errorf("encode runtime control request: %w", err)
		}
		body = bytes.NewReader(data)
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://"+state.Address+path, body)
	if err != nil {
		return State{}, err
	}
	request.Header.Set("Authorization", "Bearer "+state.Token)
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{Proxy: nil}}
	response, err := client.Do(request)
	if err != nil {
		return State{}, fmt.Errorf("running server control endpoint unavailable: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
		var failure struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &failure) == nil && failure.Error != "" {
			return State{}, errors.New(failure.Error)
		}
		return State{}, fmt.Errorf("runtime control request failed with HTTP %d", response.StatusCode)
	}
	if output != nil {
		if err := json.NewDecoder(response.Body).Decode(output); err != nil {
			return State{}, fmt.Errorf("decode runtime control response: %w", err)
		}
	}
	return state, nil
}
