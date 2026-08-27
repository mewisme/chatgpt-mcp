package tools

import (
	"context"
)

type Runtime struct {
	Registry *Registry
}

func NewRuntime() *Runtime {
	r := &Runtime{Registry: NewRegistry()}
	RegisterCore(r.Registry)
	return r
}

func (r *Runtime) List() []string { return r.Registry.List() }

func (r *Runtime) Call(ctx context.Context, name string, args map[string]any) (any, error) {
	if err := RequireWorkingDirectory(args); err != nil {
		return nil, err
	}
	value, _, err := r.Registry.Call(name, args)
	return value, err
}
