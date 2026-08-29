package tools

import (
	"context"
	"errors"
	"time"

	"go.mewis.me/chatgpt-mcp/internal/checkpoint"
	"go.mewis.me/chatgpt-mcp/internal/upstream"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

type Runtime struct {
	Registry     *Registry
	Workspaces   *workspace.Manager
	Checkpoints  *checkpoint.Store
	Upstream     *upstream.Manager
	CallObserver CallObserver
}

func NewRuntime() *Runtime {
	workspaces := workspace.NewManager(workspace.DefaultStorePath())
	checkpoints := checkpoint.NewStore(checkpoint.DefaultRoot())
	upstreams := upstream.NewManager(upstream.NewStore(upstream.Path()))
	_ = upstreams.Load()
	registry := NewRegistry()
	runtime := &Runtime{Registry: registry, Workspaces: workspaces, Checkpoints: checkpoints, Upstream: upstreams}
	RegisterWorkspaceTools(registry, workspaces)
	RegisterCore(registry, workspaces, checkpoints)
	RegisterUpstreamTools(registry, upstreams)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	_ = RefreshUpstreamProxies(ctx, registry, upstreams, false)
	cancel()
	return runtime
}

func (r *Runtime) List() []Schema      { return r.Registry.ListSchemas() }
func (r *Runtime) ListTools() []Schema { return r.List() }

func (r *Runtime) Call(ctx context.Context, name string, args map[string]any) (Result, error) {
	started := time.Now()
	source := CallSource(ctx)
	workspaceID, _ := args["workspace_id"].(string)
	r.observeCall(CallObservation{Phase: "start", Source: source, Tool: name, WorkspaceID: workspaceID})

	result, err := r.Registry.Call(ctx, name, args)
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
		r.observeCall(CallObservation{Phase: "finish", Source: source, Tool: name, WorkspaceID: workspaceID, Status: status, DurationMS: time.Since(started).Milliseconds(), Message: message, ResultType: result.ResultType})
		return result, nil
	}

	status, message := "error", err.Error()
	if ctx != nil && ctx.Err() != nil {
		status, message = "cancelled", ctx.Err().Error()
	}
	if errors.Is(err, ErrToolNotFound) {
		r.observeCall(CallObservation{Phase: "finish", Source: source, Tool: name, WorkspaceID: workspaceID, Status: status, DurationMS: time.Since(started).Milliseconds(), Message: message})
		return Result{}, err
	}
	result = ErrorResult(err)
	r.observeCall(CallObservation{Phase: "finish", Source: source, Tool: name, WorkspaceID: workspaceID, Status: status, DurationMS: time.Since(started).Milliseconds(), Message: message, ResultType: result.ResultType})
	return result, nil
}
