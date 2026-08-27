package tools

import "context"

type Runtime struct{ Registry *Registry }

func NewRuntime(registry *Registry) *Runtime { return &Runtime{Registry: registry} }

func (r *Runtime) List() []string { return r.Registry.List() }

func (r *Runtime) Call(ctx context.Context, name string, args map[string]any) (any, error) {
	value, _, err := r.Registry.Call(name, args)
	return value, err
}
