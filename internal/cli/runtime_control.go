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
	"strings"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/application"
	"go.mewis.me/chatgpt-mcp/internal/approval"
	"go.mewis.me/chatgpt-mcp/internal/auth"
	"go.mewis.me/chatgpt-mcp/internal/config"
	"go.mewis.me/chatgpt-mcp/internal/controlguard"
	"go.mewis.me/chatgpt-mcp/internal/logger"
	"go.mewis.me/chatgpt-mcp/internal/runtimecontrol"
	"go.mewis.me/chatgpt-mcp/internal/runtimeevent"
	shellruntime "go.mewis.me/chatgpt-mcp/internal/shell"
	"go.mewis.me/chatgpt-mcp/internal/state"
)

type runtimeControlState = runtimecontrol.State

type runtimeReloadResult = runtimecontrol.ReloadResult
type runtimeStatusResult = runtimecontrol.RuntimeStatus
type workspaceReloadResult = runtimecontrol.WorkspaceReloadResult

type runtimeControlOptions struct {
	RunID            string
	Managed          bool
	ServiceID        string
	ServiceScope     string
	StartedAt        time.Time
	Events           *runtimeevent.Stream
	Reload           func(context.Context) (runtimeReloadResult, error)
	ReloadWorkspaces func() (workspaceReloadResult, error)
	Status           func() runtimeStatusResult
	StatusWait       func(context.Context, string) runtimeStatusResult
	Shutdown         func()
	ClearLogs        func() error
	Approvals        *approval.Manager
	Executions       *shellruntime.ExecutionHub
	Log              *logger.Logger
}

type runtimeControl struct {
	state    runtimeControlState
	listener net.Listener
	server   *http.Server
	path     string
}

func runtimeControlPath() string { return runtimecontrol.Path() }

func reloadResult(cfg config.Config, networkRestarted bool) runtimeReloadResult {
	return runtimeReloadResult{PID: os.Getpid(), NetworkRestarted: networkRestarted, ServerEnabled: cfg.Server.Enabled, ServerPort: cfg.Server.Port, AdminEnabled: cfg.Admin.Enabled, AdminPort: cfg.Admin.Port, Exposure: cfg.Server.Expose.Mode}
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
	mux.HandleFunc("/workspaces/reload", authenticatedControl(controlState.Token, http.MethodPost, func(w http.ResponseWriter, _ *http.Request) {
		if options.ReloadWorkspaces == nil {
			writeControlJSON(w, nil, errors.New("workspace reload handler is unavailable"))
			return
		}
		result, err := options.ReloadWorkspaces()
		writeControlJSON(w, result, err)
	}))
	mux.HandleFunc("/status", authenticatedControl(controlState.Token, http.MethodGet, func(w http.ResponseWriter, _ *http.Request) {
		writeControlJSON(w, options.Status(), nil)
	}))
	mux.HandleFunc("/status/wait", authenticatedControl(controlState.Token, http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		if options.StatusWait == nil {
			writeControlJSON(w, options.Status(), nil)
			return
		}
		writeControlJSON(w, options.StatusWait(r.Context(), r.URL.Query().Get("lifecycle")), nil)
	}))
	mux.HandleFunc("/shutdown", authenticatedControl(controlState.Token, http.MethodPost, func(w http.ResponseWriter, _ *http.Request) {
		writeControlJSON(w, map[string]bool{"ok": true}, nil)
		options.Shutdown()
	}))
	mux.HandleFunc("/logs/clear", authenticatedControl(controlState.Token, http.MethodPost, func(w http.ResponseWriter, _ *http.Request) {
		writeControlJSON(w, map[string]bool{"ok": true}, options.ClearLogs())
	}))
	mux.HandleFunc("/requests", authenticatedControl(controlState.Token, http.MethodGet, func(w http.ResponseWriter, _ *http.Request) {
		if options.Approvals == nil {
			writeControlJSON(w, nil, errors.New("control approval manager is unavailable"))
			return
		}
		writeControlJSON(w, options.Approvals.List(approval.Filter{}), nil)
	}))
	mux.HandleFunc("/requests/view", authenticatedControl(controlState.Token, http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		if options.Approvals == nil {
			writeControlJSON(w, nil, errors.New("control approval manager is unavailable"))
			return
		}
		request, err := options.Approvals.Resolve(r.URL.Query().Get("id"))
		writeControlJSON(w, request, err)
	}))
	mux.HandleFunc("/requests/create-dummy", authenticatedControl(controlState.Token, http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		if options.Approvals == nil {
			writeControlJSON(w, nil, errors.New("control approval manager is unavailable"))
			return
		}
		var input struct {
			WorkspaceID string `json:"workspace_id"`
			Title       string `json:"title"`
			Command     string `json:"command"`
		}
		if err := decodeControlJSON(r, &input); err != nil {
			writeControlJSON(w, nil, err)
			return
		}
		workspaceID := strings.TrimSpace(input.WorkspaceID)
		if workspaceID == "" {
			workspaceID = "ws_dummy"
		}
		title := strings.TrimSpace(input.Title)
		if title == "" {
			title = "Allow dummy command"
		}
		command := strings.TrimSpace(input.Command)
		if command == "" {
			command = "echo dummy approval"
		}
		sessionID := auth.GenerateToken("dummy")
		challenge, _, err := options.Approvals.CreateChallenge(approval.ChallengeInput{
			SessionID: sessionID, SessionHash: "dummy", WorkspaceID: workspaceID, Source: "cli-dummy", TargetTool: "run_command",
			Arguments: map[string]any{"workspace_id": workspaceID, "command": command, "dummy": true}, GuardCode: controlguard.CodeControlPlaneMutation,
			GuardReason: "dummy approval request created for UI testing", Title: title, Command: command,
		})
		if err != nil {
			writeControlJSON(w, nil, err)
			return
		}
		request, _, err := options.Approvals.CreateRequest(challenge.ID, sessionID, workspaceID)
		writeControlJSON(w, request, err)
	}))
	resolveRequest := func(status approval.Status) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if options.Approvals == nil {
				writeControlJSON(w, nil, errors.New("control approval manager is unavailable"))
				return
			}
			var input struct {
				ID           string `json:"id"`
				Reason       string `json:"reason,omitempty"`
				AllowSimilar bool   `json:"allow_similar,omitempty"`
			}
			if err := decodeControlJSON(r, &input); err != nil {
				writeControlJSON(w, nil, err)
				return
			}
			request, err := options.Approvals.Resolve(input.ID)
			if err != nil {
				writeControlJSON(w, nil, err)
				return
			}
			if status == approval.StatusApproved {
				if input.AllowSimilar {
					request, err = options.Approvals.ApproveRuntimeSession(request.ID, "cli", input.Reason)
				} else {
					request, err = options.Approvals.Approve(request.ID, "cli", input.Reason)
				}
			} else {
				request, err = options.Approvals.Deny(request.ID, "cli", input.Reason)
			}
			writeControlJSON(w, request, err)
		}
	}
	mux.HandleFunc("/requests/approve", authenticatedControl(controlState.Token, http.MethodPost, resolveRequest(approval.StatusApproved)))
	mux.HandleFunc("/requests/deny", authenticatedControl(controlState.Token, http.MethodPost, resolveRequest(approval.StatusDenied)))
	mux.HandleFunc("/requests/consume-cli", authenticatedControl(controlState.Token, http.MethodPost, func(w http.ResponseWriter, r *http.Request) {
		if options.Approvals == nil {
			writeControlJSON(w, nil, errors.New("control approval manager is unavailable"))
			return
		}
		var input struct {
			Capability string   `json:"capability"`
			Args       []string `json:"args"`
		}
		if err := decodeControlJSON(r, &input); err != nil {
			writeControlJSON(w, nil, err)
			return
		}
		requestID, err := options.Approvals.ConsumeCLI(input.Capability, input.Args)
		writeControlJSON(w, map[string]string{"request_id": requestID}, err)
	}))
	mux.HandleFunc("/events", authenticatedControl(controlState.Token, http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		serveRuntimeEvents(w, r, options.Events)
	}))
	mux.HandleFunc("/executions/stream", authenticatedControl(controlState.Token, http.MethodGet, func(w http.ResponseWriter, r *http.Request) {
		serveRuntimeExecutionFeed(w, r, options.Executions)
	}))
	server := newHTTPServer(mux)
	control := &runtimeControl{state: controlState, listener: listener, server: server, path: path}
	go serveRuntimeControl(server, listener, options.Log)
	return control, nil
}

func serveRuntimeControl(server *http.Server, listener net.Listener, log *logger.Logger) {
	if server == nil || listener == nil {
		return
	}
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) && log != nil {
		log.Failure("CONTROL", "runtime.control.failed", "Runtime control server stopped unexpectedly", err,
			logger.WithVerbose("address", listener.Addr().String()),
		)
	}
}

func serveRuntimeExecutionFeed(w http.ResponseWriter, r *http.Request, hub *shellruntime.ExecutionHub) {
	if hub == nil {
		http.Error(w, "execution stream unavailable", http.StatusServiceUnavailable)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	sub, snapshot := hub.SubscribeFeed("")
	if sub == nil {
		http.Error(w, "execution stream unavailable", http.StatusServiceUnavailable)
		return
	}
	defer hub.UnsubscribeFeed(sub)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	data, err := json.Marshal(snapshot)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := fmt.Fprintf(w, "event: ready\ndata: %s\n\n", data); err != nil {
		return
	}
	flusher.Flush()
	latestSequence := snapshot.LatestSequence
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case overflow := <-sub.Overflow:
			if overflow.DroppedSequence == 0 {
				return
			}
			_, _ = fmt.Fprintf(w, "event: overflow\ndata: {\"dropped_sequence\":%d}\n\n", overflow.DroppedSequence)
			flusher.Flush()
			return
		case <-heartbeat.C:
			if _, err := fmt.Fprintf(w, "event: heartbeat\ndata: {\"latest_sequence\":%d}\n\n", latestSequence); err != nil {
				return
			}
			flusher.Flush()
		case event, ok := <-sub.Events:
			if !ok {
				return
			}
			latestSequence = event.Sequence
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", event.Sequence, event.Type, data); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func decodeControlJSON(r *http.Request, output any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 64*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		return fmt.Errorf("decode runtime control request: %w", err)
	}
	return nil
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

func runtimeControlRequest(ctx context.Context, method, path string, output any) (runtimeControlState, error) {
	return runtimecontrol.Request(ctx, method, path, nil, output)
}

func runtimeControlJSONRequest(ctx context.Context, method, path string, input, output any) (runtimeControlState, error) {
	return runtimecontrol.Request(ctx, method, path, input, output)
}

func requestRuntimeCLIApproval(ctx context.Context, capability string, args []string) error {
	var result struct {
		RequestID string `json:"request_id"`
	}
	_, err := runtimeControlJSONRequest(ctx, http.MethodPost, "/requests/consume-cli", map[string]any{"capability": capability, "args": args}, &result)
	if err != nil {
		return err
	}
	if strings.TrimSpace(result.RequestID) == "" {
		return errors.New("runtime control approval response is missing request id")
	}
	return nil
}

func requestRuntimeApprovalList(ctx context.Context) ([]approval.Request, error) {
	return application.ListApprovalRequests(ctx)
}

func requestRuntimeApprovalView(ctx context.Context, id string) (approval.Request, error) {
	return application.GetApprovalRequest(ctx, id)
}

func requestRuntimeApprovalCreateDummy(ctx context.Context, workspaceID, title, command string) (approval.Request, error) {
	return application.CreateDummyApprovalRequest(ctx, workspaceID, title, command)
}

func requestRuntimeApprovalResolve(ctx context.Context, action, id, reason string) (approval.Request, error) {
	switch action {
	case "approve":
		return application.ResolveApprovalRequest(ctx, id, true, reason)
	case "deny":
		return application.ResolveApprovalRequest(ctx, id, false, reason)
	default:
		return approval.Request{}, fmt.Errorf("unsupported approval action: %s", action)
	}
}

func requestRuntimeApprovalApprove(ctx context.Context, id, reason string) (approval.Request, error) {
	return requestRuntimeApprovalResolve(ctx, "approve", id, reason)
}

func requestRuntimeApprovalDeny(ctx context.Context, id, reason string) (approval.Request, error) {
	return requestRuntimeApprovalResolve(ctx, "deny", id, reason)
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
