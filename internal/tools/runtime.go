package tools

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.mewis.me/chatgpt-mcp/internal/approval"
	"go.mewis.me/chatgpt-mcp/internal/caveman"
	"go.mewis.me/chatgpt-mcp/internal/checkpoint"
	"go.mewis.me/chatgpt-mcp/internal/controlguard"
	"go.mewis.me/chatgpt-mcp/internal/features"
	"go.mewis.me/chatgpt-mcp/internal/ponytail"
	shellruntime "go.mewis.me/chatgpt-mcp/internal/shell"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

type Runtime struct {
	Registry        *Registry
	Workspaces      *workspace.Manager
	Checkpoints     *checkpoint.Store
	Upstream        *upstream.Manager
	CallObserver    CallObserver
	SessionBindings *SessionWorkspaceBinder
	Approvals       *approval.Manager
	Executions      *shellruntime.ExecutionHub
	sessionMu       sync.Mutex
	featureMu       sync.Mutex
	features        features.Config
	ponytailManager *ponytail.Manager
	cavemanManager  *caveman.Manager
}

func NewRuntime() *Runtime {
	return NewRuntimeWithFeatures(features.Default())
}

func NewRuntimeWithFeatures(featureConfig features.Config) *Runtime {
	return NewRuntimeWithAccess(featureConfig, nil)
}

func NewRuntimeWithAccess(featureConfig features.Config, globalAllowDirs []string) *Runtime {
	workspaces := workspace.NewManagerWithGlobalAllowDirs(workspace.DefaultStorePath(), globalAllowDirs)
	checkpoints := checkpoint.NewStore(checkpoint.DefaultRoot())
	upstreams := upstream.NewManager(upstream.NewStore(upstream.Path()))
	_ = upstreams.Load()
	registry := NewRegistry()
	identity, err := workspaces.Instance()
	if err != nil {
		panic(err)
	}
	executions := shellruntime.NewExecutionHub()
	runtime := &Runtime{Registry: registry, Workspaces: workspaces, Checkpoints: checkpoints, Upstream: upstreams, SessionBindings: NewSessionWorkspaceBinder(), Approvals: approval.NewManager(identity.ID), Executions: executions, ponytailManager: ponytail.NewManager(), cavemanManager: caveman.NewManager()}
	shell := shellruntime.NewManagerWithExecutions(workspaces, shellruntime.DefaultStateRoot(), executions)
	RegisterWorkspaceTools(registry, workspaces, shell)
	RegisterWorkspaceListTool(registry, runtime)
	RegisterCore(registry, workspaces, checkpoints, shell)
	RegisterApprovalTools(registry, runtime)
	RegisterUpstreamTools(registry, upstreams)
	if err := runtime.SyncFeatures(featureConfig); err != nil {
		panic(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = RefreshUpstreamProxies(ctx, registry, upstreams, false)
	cancel()
	return runtime
}

func (r *Runtime) SyncFeatures(featureConfig features.Config) error {
	if r == nil || r.Registry == nil || r.Workspaces == nil {
		return errors.New("tool runtime is unavailable")
	}
	r.featureMu.Lock()
	defer r.featureMu.Unlock()
	if r.ponytailManager == nil {
		r.ponytailManager = ponytail.NewManager()
	}
	if r.cavemanManager == nil {
		r.cavemanManager = caveman.NewManager()
	}
	if err := r.Registry.ReplaceOwnedPrefix("feature:", featureToolEntries(r.Workspaces, r.ponytailManager, r.cavemanManager, featureConfig)); err != nil {
		return err
	}
	r.features = featureConfig
	return nil
}

func (r *Runtime) Features() features.Config {
	if r == nil {
		return features.Config{}
	}
	r.featureMu.Lock()
	defer r.featureMu.Unlock()
	return r.features
}

func (r *Runtime) SetGlobalAllowDirs(allowDirs []string) {
	if r != nil && r.Workspaces != nil {
		r.Workspaces.SetGlobalAllowDirs(allowDirs)
	}
}

func (r *Runtime) List() []Schema      { return r.Registry.ListSchemas() }
func (r *Runtime) ListTools() []Schema { return r.List() }

func (r *Runtime) Call(ctx context.Context, name string, args map[string]any) (Result, error) {
	callUUID, err := uuid.NewV7()
	if err != nil {
		return Result{}, fmt.Errorf("generate tool call id: %w", err)
	}
	callID := callUUID.String()
	started := time.Now()
	source := CallSource(ctx)
	receivedBy := ReceivedByInstanceID(ctx)
	if receivedBy == "" {
		receivedBy = r.runtimeInstanceID()
	}
	workspaceID := ""
	sessionID := MCPSessionID(ctx)
	sessionHash := MCPSessionFingerprint(sessionID)
	sessionBinding := SessionBindingDecision("")
	sessionWorkspaceID := ""
	var preflightErr error
	if r.Registry != nil {
		workspaceScoped, err := r.Registry.WorkspaceScoped(name)
		if err != nil {
			preflightErr = err
		} else if workspaceScoped {
			workspaceID, preflightErr = requiredString(args, "workspace_id")
			if preflightErr == nil && r.Workspaces == nil {
				preflightErr = errors.New("workspace manager is unavailable")
			}
			if preflightErr == nil {
				canonical, err := r.Workspaces.CanonicalID(workspaceID)
				if err != nil {
					preflightErr = err
				} else {
					if canonical != workspaceID {
						args = cloneMap(args)
						args["workspace_id"] = canonical
					}
					workspaceID = canonical
					if sessionID != "" {
						binding, decision, err := r.sessionBinder().CheckOrBind(sessionID, workspaceID)
						sessionBinding = decision
						sessionWorkspaceID = binding.WorkspaceID
						preflightErr = err
					}
				}
			}
		}
	}
	claimedApproval := approval.Request{}
	var forcedResult *Result
	if preflightErr == nil {
		ctx, claimedApproval, forcedResult, preflightErr = r.prepareApprovalRetry(ctx, sessionID, workspaceID, source, name, args)
	}
	raw := callRaw(ctx, source, name, args)
	raw["call_id"] = callID
	if sessionHash != "" {
		raw["session"] = map[string]any{"hash": sessionHash, "binding": sessionBinding, "workspace_id": sessionWorkspaceID}
	}
	r.observeCall(CallObservation{CallID: callID, Phase: "start", Source: source, Tool: name, WorkspaceID: workspaceID, Raw: raw, SessionHash: sessionHash, SessionBinding: sessionBinding, SessionWorkspaceID: sessionWorkspaceID, ReceivedByInstanceID: receivedBy})

	result, err := Result{}, preflightErr
	if err == nil && forcedResult != nil {
		result = *forcedResult
	} else if err == nil {
		result, err = r.Registry.Call(ctx, name, args)
	}
	if err != nil {
		if guard, ok := controlguard.As(err); ok {
			if guardedResult, handled, guardErr := r.approvalResultForGuard(guard, sessionID, sessionHash, workspaceID, source, name, args, claimedApproval); guardErr != nil {
				err = guardErr
			} else if handled {
				result, err = guardedResult, nil
			}
		}
	}
	executedBy := r.runtimeInstanceID()
	finishRaw := cloneMap(raw)
	finishRaw["routing"] = map[string]any{"received_by_instance_id": receivedBy, "executed_by_instance_id": executedBy}
	if err == nil {
		if result.ResultType == "" {
			result.ResultType = "complete"
		}
		status, message := "ok", ""
		if result.IsError {
			status = "error"
			if len(result.Content) > 0 {
				message = result.Content[0].Text
			}
		}
		finishRaw["status"] = status
		finishRaw["result_type"] = result.ResultType
		finishRaw["result"] = observedResult(name, result)
		r.observeCall(CallObservation{CallID: callID, Phase: "finish", Source: source, Tool: name, WorkspaceID: workspaceID, Status: status, DurationMS: time.Since(started).Milliseconds(), Message: message, ResultType: result.ResultType, Raw: finishRaw, SessionHash: sessionHash, SessionBinding: sessionBinding, SessionWorkspaceID: sessionWorkspaceID, ReceivedByInstanceID: receivedBy, ExecutedByInstanceID: executedBy})
		return result, nil
	}

	status, message := "error", err.Error()
	if ctx != nil && ctx.Err() != nil {
		status, message = "cancelled", ctx.Err().Error()
	}
	finishRaw["status"] = status
	finishRaw["error"] = message
	if errors.Is(err, ErrToolNotFound) {
		r.observeCall(CallObservation{CallID: callID, Phase: "finish", Source: source, Tool: name, WorkspaceID: workspaceID, Status: status, DurationMS: time.Since(started).Milliseconds(), Message: message, Raw: finishRaw, SessionHash: sessionHash, SessionBinding: sessionBinding, SessionWorkspaceID: sessionWorkspaceID, ReceivedByInstanceID: receivedBy, ExecutedByInstanceID: executedBy})
		return Result{}, err
	}
	result = ErrorResult(err)
	finishRaw["result_type"] = result.ResultType
	finishRaw["result"] = observedResult(name, result)
	r.observeCall(CallObservation{CallID: callID, Phase: "finish", Source: source, Tool: name, WorkspaceID: workspaceID, Status: status, DurationMS: time.Since(started).Milliseconds(), Message: message, ResultType: result.ResultType, Raw: finishRaw, SessionHash: sessionHash, SessionBinding: sessionBinding, SessionWorkspaceID: sessionWorkspaceID, ReceivedByInstanceID: receivedBy, ExecutedByInstanceID: executedBy})
	return result, nil
}

func observedResult(name string, result Result) any {
	if name != "run_command" {
		return result
	}
	value, ok := result.StructuredContent.(shellruntime.ExecResult)
	if !ok {
		return map[string]any{"result_type": result.ResultType, "is_error": result.IsError}
	}
	return map[string]any{
		"result_type": result.ResultType,
		"is_error":    result.IsError,
		"command":     value.Command,
		"cwd":         value.CWD,
		"exit_code":   value.ExitCode,
		"timed_out":   value.TimedOut,
	}
}

func (r *Runtime) sessionBinder() *SessionWorkspaceBinder {
	r.sessionMu.Lock()
	defer r.sessionMu.Unlock()
	if r.SessionBindings == nil {
		r.SessionBindings = NewSessionWorkspaceBinder()
	}
	return r.SessionBindings
}

func (r *Runtime) runtimeInstanceID() string {
	if r == nil || r.Workspaces == nil {
		return ""
	}
	identity, err := r.Workspaces.Instance()
	if err != nil {
		return ""
	}
	return identity.ID
}
