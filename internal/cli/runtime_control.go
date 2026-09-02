package cli

import (
	"context"
	"crypto/subtle"
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

	"go.mewis.me/chatgpt-mcp/internal/auth"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/runtimeevent"
	"go.mewis.me/chatgpt-mcp/internal/state"
)

const runtimeControlFile = ".runtime-control.json"

type runtimeControlState struct {
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

type runtimeReloadResult struct {
	PID              int                 `json:"pid"`
	NetworkRestarted bool                `json:"network_restarted"`
	ServerPort       int                 `json:"server_port"`
	AdminEnabled     bool                `json:"admin_enabled"`
	AdminPort        int                 `json:"admin_port"`
	Exposure         config.ExposureMode `json:"exposure"`
}

type runtimeStatusResult struct {
	PID              int                 `json:"pid"`
	RunID            string              `json:"run_id,omitempty"`
	Managed          bool                `json:"managed"`
	ServiceID        string              `json:"service_id,omitempty"`
	ServiceScope     string              `json:"service_scope,omitempty"`
	StartedAt        time.Time           `json:"started_at"`
	ConfigRoot       string              `json:"config_root"`
	ServerPort       int                 `json:"server_port"`
	AdminEnabled     bool                `json:"admin_enabled"`
	AdminPort        int                 `json:"admin_port"`
	Exposure         config.ExposureMode `json:"exposure"`
	TunnelEnabled    bool                `json:"tunnel_enabled"`
	TunnelConfigured bool                `json:"tunnel_configured"`
	TunnelRunning    bool                `json:"tunnel_running"`
	TunnelReady      bool                `json:"tunnel_ready"`
	TunnelRestarting bool                `json:"tunnel_restarting"`
	TunnelID         string              `json:"tunnel_id,omitempty"`
	TunnelLastError  string              `json:"tunnel_last_error,omitempty"`
}

type runtimeControlOptions struct {
	RunID        string
	Managed      bool
	ServiceID    string
	ServiceScope string
	StartedAt    time.Time
	Events       *runtimeevent.Stream
	Reload       func(context.Context) (runtimeReloadResult, error)
	Status       func() runtimeStatusResult
	Shutdown     func()
	ClearLogs    func() error
}

type runtimeControl struct {
	state    runtimeControlState
	listener net.Listener
	server   *http.Server
	path     string
}

func runtimeControlPath() string { return filepath.Join(config.RootPath(), runtimeControlFile) }

func reloadResult(cfg config.Config, networkRestarted bool) runtimeReloadResult {
	return runtimeReloadResult{PID: os.Getpid(), NetworkRestarted: networkRestarted, ServerPort: cfg.Server.Port, AdminEnabled: cfg.Admin.Enabled, AdminPort: cfg.Admin.Port, Exposure: cfg.Server.Expose.Mode}
}

func startRuntimeControl(options runtimeControlOptions) (*runtimeControl, error) {
	if options.Reload == nil || options.Status == nil || options.Shutdown == nil || options.ClearLogs == nil {
		return nil, errors.New("runtime control handlers are incomplete")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	startedAt := options.StartedAt.UTC()
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	controlState := runtimeControlState{PID: os.Getpid(), Address: listener.Addr().String(), Token: auth.GenerateToken("runtime"), RunID: options.RunID, Managed: options.Managed, ServiceID: options.ServiceID, ServiceScope: options.ServiceScope, StartedAt: startedAt, ConfigRoot: config.RootPath()}
	path := runtimeControlPath()
	data, err := json.Marshal(controlState)
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	if err := state.WriteFileAtomic(path, append(data, '\n'), 0600); err != nil {
		_ = listener.Close()
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/reload", authenticatedControl(controlState.Token, http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		result, err := options.Reload(r.Context())
		writeControlJSON(w, result, err)
	}))
	mux.HandleFunc("/status", authenticatedControl(controlState.Token, http.MethodGet, func(w http.ResponseWriter, _ *http.Request) {
		writeControlJSON(w, options.Status(), nil)
	}))
	mux.HandleFunc("/shutdown", authenticatedControl(controlState.Token, http.MethodPost, func(w http.ResponseWriter, _ *http.Request) {
		writeControlJSON(w, map[string]bool{"ok": true}, nil)
		options.Shutdown()
	}))
	mux.HandleFunc("/logs/clear", authenticatedControl(controlState.Token, http.MethodPost, func(w http.ResponseWriter, _ *http.Request) {
		writeControlJSON(w, map[string]bool{"ok": true}, options.ClearLogs())
	}))
	mux.HandleFunc("/events", authenticatedControl(controlState.Token, http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		serveRuntimeEvents(w, r, options.Events)
	}))
	server := newHTTPServer(mux)
	control := &runtimeControl{state: controlState, listener: listener, server: server, path: path}
	go func() { _ = server.Serve(listener) }()
	return control, nil
}

func authenticatedControl(token, method string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if len(provided) != len(token) || subtle.ConstantTimeCompare([]byte(provided), []byte(token)) != 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func writeControlJSON(w http.ResponseWriter, value any, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(value)
}

func serveRuntimeEvents(w http.ResponseWriter, r *http.Request, stream *runtimeevent.Stream) {
	if stream == nil {
		http.Error(w, "runtime event stream unavailable", http.StatusServiceUnavailable)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	sub := stream.Subscribe()
	defer stream.Unsubscribe(sub)
	_, _ = fmt.Fprintf(w, "event: ready\ndata: {\"latest_sequence\":%d}\n\n", stream.LatestSequence())
	flusher.Flush()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			_, _ = fmt.Fprintf(w, "event: heartbeat\ndata: {\"latest_sequence\":%d}\n\n", stream.LatestSequence())
			flusher.Flush()
		case event, ok := <-sub:
			if !ok {
				return
			}
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			_, _ = fmt.Fprintf(w, "id: %d\nevent: runtime\ndata: %s\n\n", event.Sequence, data)
			flusher.Flush()
		}
	}
}

func (c *runtimeControl) Close() error {
	if c == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := c.server.Shutdown(ctx)
	_ = c.listener.Close()
	if data, readErr := os.ReadFile(c.path); readErr == nil {
		var current runtimeControlState
		if json.Unmarshal(data, &current) == nil && current.Token == c.state.Token {
			_ = os.Remove(c.path)
		}
	}
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func loadRuntimeControlState() (runtimeControlState, error) {
	data, err := os.ReadFile(runtimeControlPath())
	if err != nil {
		if os.IsNotExist(err) {
			return runtimeControlState{}, errors.New("no running server found for this config directory")
		}
		return runtimeControlState{}, err
	}
	var control runtimeControlState
	if err := json.Unmarshal(data, &control); err != nil {
		return runtimeControlState{}, fmt.Errorf("decode runtime control state: %w", err)
	}
	if control.PID <= 0 || strings.TrimSpace(control.Token) == "" {
		return runtimeControlState{}, errors.New("runtime control state is invalid")
	}
	host, _, err := net.SplitHostPort(control.Address)
	if err != nil {
		return runtimeControlState{}, errors.New("runtime control address is invalid")
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil || !ip.IsLoopback() {
		return runtimeControlState{}, errors.New("runtime control address is not loopback")
	}
	return control, nil
}

func runtimeControlRequest(ctx context.Context, method, path string, output any) (runtimeControlState, error) {
	control, err := loadRuntimeControlState()
	if err != nil {
		return runtimeControlState{}, err
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://"+control.Address+path, nil)
	if err != nil {
		return runtimeControlState{}, err
	}
	request.Header.Set("Authorization", "Bearer "+control.Token)
	client := &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{Proxy: nil}}
	response, err := client.Do(request)
	if err != nil {
		return runtimeControlState{}, fmt.Errorf("running server control endpoint unavailable: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
		var failure struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &failure) == nil && failure.Error != "" {
			return runtimeControlState{}, errors.New(failure.Error)
		}
		return runtimeControlState{}, fmt.Errorf("runtime control request failed with HTTP %d", response.StatusCode)
	}
	if output != nil {
		if err := json.NewDecoder(response.Body).Decode(output); err != nil {
			return runtimeControlState{}, fmt.Errorf("decode runtime control response: %w", err)
		}
	}
	return control, nil
}

func requestRuntimeReload(ctx context.Context) (runtimeReloadResult, error) {
	var result runtimeReloadResult
	control, err := runtimeControlRequest(ctx, http.MethodPost, "/reload", &result)
	if err != nil {
		return runtimeReloadResult{}, err
	}
	if result.PID != control.PID {
		return runtimeReloadResult{}, fmt.Errorf("runtime control PID mismatch: expected %d, got %d", control.PID, result.PID)
	}
	return result, nil
}

func requestRuntimeStatus(ctx context.Context) (runtimeStatusResult, error) {
	var result runtimeStatusResult
	control, err := runtimeControlRequest(ctx, http.MethodGet, "/status", &result)
	if err != nil {
		return runtimeStatusResult{}, err
	}
	if result.PID != control.PID {
		return runtimeStatusResult{}, fmt.Errorf("runtime control PID mismatch: expected %d, got %d", control.PID, result.PID)
	}
	return result, nil
}

func requestRuntimeShutdown(ctx context.Context) error {
	_, err := runtimeControlRequest(ctx, http.MethodPost, "/shutdown", &map[string]bool{})
	return err
}

func requestRuntimeClearLogs(ctx context.Context) error {
	_, err := runtimeControlRequest(ctx, http.MethodPost, "/logs/clear", &map[string]bool{})
	return err
}

func openRuntimeEventStream(ctx context.Context) (*http.Response, runtimeControlState, error) {
	control, err := loadRuntimeControlState()
	if err != nil {
		return nil, runtimeControlState{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+control.Address+"/events", nil)
	if err != nil {
		return nil, runtimeControlState{}, err
	}
	request.Header.Set("Authorization", "Bearer "+control.Token)
	client := &http.Client{Transport: &http.Transport{Proxy: nil}}
	response, err := client.Do(request)
	if err != nil {
		return nil, runtimeControlState{}, fmt.Errorf("running server control endpoint unavailable: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
		_ = response.Body.Close()
		return nil, runtimeControlState{}, fmt.Errorf("runtime event stream failed with HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	return response, control, nil
}
