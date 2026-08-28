package tools

import (
	"context"
	"errors"

	"go.mewis.me/chatgpt-mcp/internal/workspace"
)

type Runtime struct {
	Registry   *Registry
	Workspaces *workspace.Manager
}

func NewRuntime() *Runtime {
	workspaces := workspace.NewManager(workspace.DefaultStorePath())
	registry := NewRegistry()
	runtime := &Runtime{Registry: registry, Workspaces: workspaces}
	RegisterWorkspaceTools(registry, workspaces)
	RegisterCore(registry, workspaces)
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
