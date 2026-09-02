package tools

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/caveman"
	"go.mewis.me/chatgpt-mcp/internal/checkpoint"
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
	runtime := &Runtime{Registry: registry, Workspaces: workspaces, Checkpoints: checkpoints, Upstream: upstreams, SessionBindings: NewSessionWorkspaceBinder(), ponytailManager: ponytail.NewManager(), cavemanManager: caveman.NewManager()}
	shell := shellruntime.NewManager(workspaces, shellruntime.DefaultStateRoot())
	RegisterWorkspaceTools(registry, workspaces, shell)
	RegisterWorkspaceListTool(registry, runtime)
	RegisterCore(registry, workspaces, checkpoints, shell)
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
	started := time.Now()
	source := CallSource(ctx)
	receivedBy := ReceivedByInstanceID(ctx)
	if receivedBy == "" {
		receivedBy = r.runtimeInstanceID()
	}
	workspaceID, _ := args["workspace_id"].(string)
	if workspaceID != "" && r.Workspaces != nil {
		if canonical, err := r.Workspaces.CanonicalID(workspaceID); err == nil && canonical != workspaceID {
			args = cloneMap(args)
			args["workspace_id"] = canonical
			workspaceID = canonical
		}
	}
	raw := callRaw(ctx, source, name, args)
	r.observeCall(CallObservation{Phase: "start", Source: source, Tool: name, WorkspaceID: workspaceID, Raw: raw, ReceivedByInstanceID: receivedBy})

	result, err := r.Registry.Call(ctx, name, args)
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
		finishRaw["result"] = result
		r.observeCall(CallObservation{Phase: "finish", Source: source, Tool: name, WorkspaceID: workspaceID, Status: status, DurationMS: time.Since(started).Milliseconds(), Message: message, ResultType: result.ResultType, Raw: finishRaw, ReceivedByInstanceID: receivedBy, ExecutedByInstanceID: executedBy})
		return result, nil
	}

	status, message := "error", err.Error()
	if ctx != nil && ctx.Err() != nil {
		status, message = "cancelled", ctx.Err().Error()
	}
	finishRaw["status"] = status
	finishRaw["error"] = message
	if errors.Is(err, ErrToolNotFound) {
		r.observeCall(CallObservation{Phase: "finish", Source: source, Tool: name, WorkspaceID: workspaceID, Status: status, DurationMS: time.Since(started).Milliseconds(), Message: message, Raw: finishRaw, ReceivedByInstanceID: receivedBy, ExecutedByInstanceID: executedBy})
		return Result{}, err
	}
	result = ErrorResult(err)
	finishRaw["result_type"] = result.ResultType
	finishRaw["result"] = result
	r.observeCall(CallObservation{Phase: "finish", Source: source, Tool: name, WorkspaceID: workspaceID, Status: status, DurationMS: time.Since(started).Milliseconds(), Message: message, ResultType: result.ResultType, Raw: finishRaw, ReceivedByInstanceID: receivedBy, ExecutedByInstanceID: executedBy})
	return result, nil
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
