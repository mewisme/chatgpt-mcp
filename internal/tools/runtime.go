package tools

import (
	"context"
	"errors"
)

type Runtime struct{ Registry *Registry }

func NewRuntime() *Runtime {
	r := NewRegistry()
	RegisterCore(r)
	return &Runtime{Registry: r}
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
