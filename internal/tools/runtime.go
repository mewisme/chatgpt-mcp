package tools

import (
	"context"
	"errors"

	"go.mewis.me/chatgpt-mcp/internal/checkpoint"
	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

type Runtime struct {
	Registry    *Registry
	Workspaces  *workspace.Manager
	Checkpoints *checkpoint.Store
}

func NewRuntime() *Runtime {
	workspaces := workspace.NewManager(workspace.DefaultStorePath())
	checkpoints := checkpoint.NewStore(checkpoint.DefaultRoot())
	registry := NewRegistry()
	runtime := &Runtime{Registry: registry, Workspaces: workspaces, Checkpoints: checkpoints}
	RegisterWorkspaceTools(registry, workspaces)
	RegisterCore(registry, workspaces, checkpoints)
	return runtime
}

func (r *Runtime) List() []Schema      { return r.Registry.ListSchemas() }
func (r *Runtime) ListTools() []Schema { return r.List() }

func (r *Runtime) Call(ctx context.Context, name string, args map[string]any) (Result, error) {
	result, err := r.Registry.Call(ctx, name, args)
	if err == nil {
		return result, nil
	}
	if errors.Is(err, ErrToolNotFound) {
		return Result{}, err
	}
	return ErrorResult(err), nil
}
