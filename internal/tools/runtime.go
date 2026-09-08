package tools

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

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

const (
	tunnelToolBudget         = 100 * time.Second
	tunnelResponseReserveMax = 5 * time.Second
)

var errTunnelResponseBudgetExceeded = errors.New("tunnel response budget exhausted")

type Runtime struct {
	Registry        *Registry
	Workspaces      *workspace.Manager
	Checkpoints     *checkpoint.Store
	Upstream        *upstream.Manager
	CallObserver    CallObserver
	SessionAccess   *SessionWorkspaceAccessManager
	Approvals       *approval.Manager
	Executions      *shellruntime.ExecutionHub
	callSequence    atomic.Uint64
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

func NewRuntimeWithAccess(featureConfig features.Config, globalAllowDirs []string, environments ...ProjectContextEnvironment) *Runtime {
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
	runtime := &Runtime{Registry: registry, Workspaces: workspaces, Checkpoints: checkpoints, Upstream: upstreams, SessionAccess: NewSessionWorkspaceAccessManager(), Approvals: approval.NewManager(identity.ID), Executions: executions, ponytailManager: ponytail.NewManager(featureConfig.Ponytail.Active, ponytail.Mode(featureConfig.Ponytail.Mode)), cavemanManager: caveman.NewManager(featureConfig.Caveman.Active, caveman.Mode(featureConfig.Caveman.Mode))}
	shell := shellruntime.NewManagerWithExecutions(workspaces, shellruntime.DefaultStateRoot(), executions)
	RegisterWorkspaceTools(registry, workspaces, shell)
	RegisterWorkspaceListTool(registry, runtime)
	var environment ProjectContextEnvironment
	if len(environments) > 0 {
		environment = environments[0]
	}
	registerCore(registry, workspaces, checkpoints, environment, shell)
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
		r.ponytailManager = ponytail.NewManager(featureConfig.Ponytail.Active, ponytail.Mode(featureConfig.Ponytail.Mode))
	}
	if r.cavemanManager == nil {
		r.cavemanManager = caveman.NewManager(featureConfig.Caveman.Active, caveman.Mode(featureConfig.Caveman.Mode))
	}
	if err := r.Registry.ReplaceOwnedPrefix("feature:", featureToolEntries(r.Workspaces, r.ponytailManager, r.cavemanManager)); err != nil {
		return err
	}
	r.ponytailManager.SetDefaults(featureConfig.Ponytail.Active, ponytail.Mode(featureConfig.Ponytail.Mode))
	r.cavemanManager.SetDefaults(featureConfig.Caveman.Active, caveman.Mode(featureConfig.Caveman.Mode))
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

func (r *Runtime) ReloadWorkspaces() error {
	if r == nil || r.Workspaces == nil {
		return errors.New("tool runtime is unavailable")
	}
	return r.Workspaces.Reload()
}

func (r *Runtime) SetShellApprovalPolicy(value string) error {
	if r == nil || r.Workspaces == nil {
		return errors.New("tool runtime is unavailable")
	}
	policy, ok := workspace.NormalizeShellApprovalPolicy(value)
	if !ok {
		return fmt.Errorf("unsupported shell approval policy: %q", value)
	}
	return r.Workspaces.SetShellApprovalPolicy(policy)
}

func (r *Runtime) SetShellApprovalCommands(allow, deny []string) error {
	if r == nil || r.Workspaces == nil {
		return errors.New("tool runtime is unavailable")
	}
	return r.Workspaces.SetShellApprovalCommands(allow, deny)
}

func (r *Runtime) SetShellEnvironmentPolicy(value string) error {
	if r == nil || r.Workspaces == nil {
		return errors.New("tool runtime is unavailable")
	}
	policy, ok := workspace.NormalizeShellEnvironmentPolicy(value)
	if !ok {
		return fmt.Errorf("unsupported shell environment policy: %q", value)
	}
	return r.Workspaces.SetShellEnvironmentPolicy(policy)
}

func (r *Runtime) SetShellSandboxPolicy(value string) error {
	if r == nil || r.Workspaces == nil {
		return errors.New("tool runtime is unavailable")
	}
	policy, ok := workspace.NormalizeShellSandboxPolicy(value)
	if !ok {
		return fmt.Errorf("unsupported shell sandbox policy: %q", value)
	}
	return r.Workspaces.SetShellSandboxPolicy(policy)
}

func (r *Runtime) SetShellNetworkPolicy(value string) error {
	if r == nil || r.Workspaces == nil {
		return errors.New("tool runtime is unavailable")
	}
	policy, ok := workspace.NormalizeShellNetworkPolicy(value)
	if !ok {
		return fmt.Errorf("unsupported shell network policy: %q", value)
	}
	return r.Workspaces.SetShellNetworkPolicy(policy)
}

func (r *Runtime) SetShellEnvironmentAllow(names []string) {
	if r != nil && r.Workspaces != nil {
		r.Workspaces.SetShellEnvironmentAllow(names)
	}
}

func (r *Runtime) SetShellPath(paths []string) {
	if r != nil && r.Workspaces != nil {
		r.Workspaces.SetShellPath(paths)
	}
}

func (r *Runtime) List() []Schema      { return r.Registry.ListSchemas() }
func (r *Runtime) ListTools() []Schema { return r.List() }

func (r *Runtime) Call(ctx context.Context, name string, args map[string]any) (Result, error) {
	callID := r.nextCallID()
	started := time.Now()
	source := CallSource(ctx)
	callCtx, cancelCall := toolCallContext(ctx, source, started)
	defer cancelCall()
	ctx = callCtx
	receivedBy := ReceivedByInstanceID(ctx)
	if receivedBy == "" {
		receivedBy = r.runtimeInstanceID()
	}
	workspaceID := ""
	sessionID := MCPSessionID(ctx)
	sessionHash := MCPSessionFingerprint(sessionID)
	sessionAccess := SessionWorkspaceAccessDecision("")
	sessionWorkspaceCount := 0
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
						_, decision, count, err := r.sessionAccessManager().CheckOrGrant(sessionID, workspaceID)
						sessionAccess = decision
						sessionWorkspaceCount = count
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
		raw["session"] = map[string]any{"hash": sessionHash, "access": sessionAccess, "workspace_count": sessionWorkspaceCount}
	}
	r.observeCall(CallObservation{CallID: callID, Phase: "start", Source: source, Tool: name, WorkspaceID: workspaceID, Raw: raw, SessionHash: sessionHash, SessionAccess: sessionAccess, SessionWorkspaceCount: sessionWorkspaceCount, ReceivedByInstanceID: receivedBy})

	result, err := Result{}, preflightErr
	if err == nil && forcedResult != nil {
		result = *forcedResult
	} else if err == nil {
		result, err = r.Registry.Call(ctx, name, args)
	}
	if err != nil && errors.Is(context.Cause(ctx), errTunnelResponseBudgetExceeded) && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
		err = tunnelResponseBudgetError(name)
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
		r.observeCall(CallObservation{CallID: callID, Phase: "finish", Source: source, Tool: name, WorkspaceID: workspaceID, Status: status, DurationMS: time.Since(started).Milliseconds(), Message: message, ResultType: result.ResultType, Raw: finishRaw, SessionHash: sessionHash, SessionAccess: sessionAccess, SessionWorkspaceCount: sessionWorkspaceCount, ReceivedByInstanceID: receivedBy, ExecutedByInstanceID: executedBy})
		return result, nil
	}

	status, message := "error", err.Error()
	if errors.Is(context.Cause(ctx), errTunnelResponseBudgetExceeded) {
		message = err.Error()
	} else if ctx != nil && ctx.Err() != nil {
		status, message = "cancelled", ctx.Err().Error()
	}
	finishRaw["status"] = status
	finishRaw["error"] = message
	if errors.Is(err, ErrToolNotFound) {
		r.observeCall(CallObservation{CallID: callID, Phase: "finish", Source: source, Tool: name, WorkspaceID: workspaceID, Status: status, DurationMS: time.Since(started).Milliseconds(), Message: message, Raw: finishRaw, SessionHash: sessionHash, SessionAccess: sessionAccess, SessionWorkspaceCount: sessionWorkspaceCount, ReceivedByInstanceID: receivedBy, ExecutedByInstanceID: executedBy})
		return Result{}, err
	}
	result = ErrorResult(err)
	finishRaw["result_type"] = result.ResultType
	finishRaw["result"] = observedResult(name, result)
	r.observeCall(CallObservation{CallID: callID, Phase: "finish", Source: source, Tool: name, WorkspaceID: workspaceID, Status: status, DurationMS: time.Since(started).Milliseconds(), Message: message, ResultType: result.ResultType, Raw: finishRaw, SessionHash: sessionHash, SessionAccess: sessionAccess, SessionWorkspaceCount: sessionWorkspaceCount, ReceivedByInstanceID: receivedBy, ExecutedByInstanceID: executedBy})
	return result, nil
}

func toolCallContext(parent context.Context, source string, now time.Time) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	if source != "tunnel" {
		return parent, func() {}
	}
	budgetDeadline := now.Add(tunnelToolBudget)
	if deadline, ok := parent.Deadline(); ok {
		remaining := deadline.Sub(now)
		if remaining <= 0 {
			return context.WithDeadlineCause(parent, deadline, errTunnelResponseBudgetExceeded)
		}
		reserve := remaining / 4
		if reserve > tunnelResponseReserveMax {
			reserve = tunnelResponseReserveMax
		}
		if reserve > 0 {
			reservedDeadline := deadline.Add(-reserve)
			if reservedDeadline.Before(budgetDeadline) {
				budgetDeadline = reservedDeadline
			}
		}
	}
	return context.WithDeadlineCause(parent, budgetDeadline, errTunnelResponseBudgetExceeded)
}

func tunnelResponseBudgetError(name string) error {
	if name == "run_command" {
		return errors.New("run_command exceeded the synchronous tunnel response budget; use start_process for long-running commands, then poll with process_status and process_output")
	}
	return fmt.Errorf("%s exceeded the synchronous tunnel response budget; split the operation into shorter tool calls", name)
}

func (r *Runtime) nextCallID() string {
	return fmt.Sprintf("call_%x_%x", time.Now().UnixMilli(), r.callSequence.Add(1))
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

func (r *Runtime) sessionAccessManager() *SessionWorkspaceAccessManager {
	r.sessionMu.Lock()
	defer r.sessionMu.Unlock()
	if r.SessionAccess == nil {
		r.SessionAccess = NewSessionWorkspaceAccessManager()
	}
	return r.SessionAccess
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
