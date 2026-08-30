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
	"go.mewis.me/chatgpt-mcp/internal/state"
)

const runtimeControlFile = ".runtime-control.json"

type runtimeControlState struct {
	PID     int    `json:"pid"`
	Address string `json:"address"`
	Token   string `json:"token"`
}

type runtimeReloadResult struct {
	PID              int                 `json:"pid"`
	NetworkRestarted bool                `json:"network_restarted"`
	ServerPort       int                 `json:"server_port"`
	AdminEnabled     bool                `json:"admin_enabled"`
	AdminPort        int                 `json:"admin_port"`
	Exposure         config.ExposureMode `json:"exposure"`
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

func startRuntimeControl(reload func(context.Context) (runtimeReloadResult, error)) (*runtimeControl, error) {
	if reload == nil {
		return nil, errors.New("runtime reload handler is required")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	controlState := runtimeControlState{PID: os.Getpid(), Address: listener.Addr().String(), Token: auth.GenerateToken("runtime")}
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
	mux.HandleFunc("/reload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if len(provided) != len(controlState.Token) || subtle.ConstantTimeCompare([]byte(provided), []byte(controlState.Token)) != 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		result, err := reload(r.Context())
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(result)
	})
	server := newHTTPServer(mux)
	control := &runtimeControl{state: controlState, listener: listener, server: server, path: path}
	go func() { _ = server.Serve(listener) }()
	return control, nil
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

func requestRuntimeReload(ctx context.Context) (runtimeReloadResult, error) {
	data, err := os.ReadFile(runtimeControlPath())
	if err != nil {
		if os.IsNotExist(err) {
			return runtimeReloadResult{}, errors.New("no running server found for this config directory")
		}
		return runtimeReloadResult{}, err
	}
	var control runtimeControlState
	if err := json.Unmarshal(data, &control); err != nil {
		return runtimeReloadResult{}, fmt.Errorf("decode runtime control state: %w", err)
	}
	if control.PID <= 0 || strings.TrimSpace(control.Token) == "" {
		return runtimeReloadResult{}, errors.New("runtime control state is invalid")
	}
	host, _, err := net.SplitHostPort(control.Address)
	if err != nil {
		return runtimeReloadResult{}, errors.New("runtime control address is invalid")
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil || !ip.IsLoopback() {
		return runtimeReloadResult{}, errors.New("runtime control address is not loopback")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+control.Address+"/reload", nil)
	if err != nil {
		return runtimeReloadResult{}, err
	}
	request.Header.Set("Authorization", "Bearer "+control.Token)
	client := &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{Proxy: nil}}
	response, err := client.Do(request)
	if err != nil {
		return runtimeReloadResult{}, fmt.Errorf("running server control endpoint unavailable: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
		var failure struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &failure) == nil && failure.Error != "" {
			return runtimeReloadResult{}, errors.New(failure.Error)
		}
		return runtimeReloadResult{}, fmt.Errorf("runtime reload failed with HTTP %d", response.StatusCode)
	}
	var result runtimeReloadResult
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return runtimeReloadResult{}, fmt.Errorf("decode runtime reload response: %w", err)
	}
	if result.PID != control.PID {
		return runtimeReloadResult{}, fmt.Errorf("runtime control PID mismatch: expected %d, got %d", control.PID, result.PID)
	}
	return result, nil
}
