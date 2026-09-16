package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/runtimeplugin"
)

func main() {
	switch strings.TrimSpace(os.Getenv("TESTPLUGIN_MODE")) {
	case "bad-version":
		fmt.Fprintln(os.Stdout, `{"version":99,"id":1,"result":{}}`)
		return
	case "malformed":
		fmt.Fprintln(os.Stdout, "not-json")
		return
	case "oversized":
		fmt.Fprintln(os.Stdout, `{"version":1,"id":1,"result":{"pad":"`+strings.Repeat("a", runtimeplugin.MaxMessageBytes)+`"}}`)
		return
	case "stderr-flood":
		_, _ = io.Copy(os.Stderr, strings.NewReader(strings.Repeat("e", runtimeplugin.MaxStderrBytes+64)))
	}
	handler := &fixtureHandler{targets: map[string]runtimeplugin.TargetStatus{}}
	if err := runtimeplugin.Serve(context.Background(), os.Stdin, os.Stdout, handler); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

type fixtureHandler struct {
	mu      sync.Mutex
	targets map[string]runtimeplugin.TargetStatus
}

func (h *fixtureHandler) Describe(context.Context, json.RawMessage) (any, error) {
	return runtimeplugin.DescribeResult{
		Provider: "test",
		Name:     "Test Tunnel",
		Targets: []runtimeplugin.Target{
			{ID: "mcp", OriginKind: "mcp-http", RequiresAuthenticatedOrigin: true},
			{ID: "admin", OriginKind: "admin-http", RequiresAuthenticatedOrigin: true},
		},
	}, nil
}

func (h *fixtureHandler) Status(context.Context, json.RawMessage) (any, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	targets := make([]runtimeplugin.TargetStatus, 0, len(h.targets))
	for _, item := range h.targets {
		targets = append(targets, item)
	}
	return runtimeplugin.StatusResult{State: "ok", Targets: targets}, nil
}

func (h *fixtureHandler) Start(_ context.Context, params json.RawMessage) (any, error) {
	var start runtimeplugin.StartParams
	if err := json.Unmarshal(params, &start); err != nil {
		return nil, err
	}
	switch start.Target {
	case "hang":
		time.Sleep(30 * time.Second)
	case "crash":
		os.Exit(2)
	case "secret":
		return nil, fmt.Errorf("failed token=%s", start.Origin)
	}
	h.mu.Lock()
	h.targets[start.Target] = runtimeplugin.TargetStatus{Target: start.Target, Desired: true, Running: true, Ready: true, Origin: start.Origin, URL: "https://example.trycloudflare.com", Ephemeral: true}
	h.mu.Unlock()
	return runtimeplugin.StatusResult{State: "started"}, nil
}

func (h *fixtureHandler) Stop(_ context.Context, params json.RawMessage) (any, error) {
	var stop runtimeplugin.StopParams
	if err := json.Unmarshal(params, &stop); err != nil {
		return nil, err
	}
	h.mu.Lock()
	delete(h.targets, stop.Target)
	h.mu.Unlock()
	return runtimeplugin.StatusResult{State: "stopped"}, nil
}

func (h *fixtureHandler) Shutdown(context.Context, json.RawMessage) (any, error) {
	if strings.TrimSpace(os.Getenv("TESTPLUGIN_MODE")) == "ignore-shutdown" {
		time.Sleep(30 * time.Second)
	}
	return struct{}{}, nil
}
