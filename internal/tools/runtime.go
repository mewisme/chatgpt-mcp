package tools

import "context"

type Runtime struct {
	Registry *Registry
}

func NewRuntime() *Runtime {
	r := NewRegistry()
	RegisterCore(r)
	return &Runtime{Registry: r}
}

func (r *Runtime) List() []string { return r.Registry.List() }

func (r *Runtime) Call(ctx context.Context, name string, args map[string]any) (any, error) {
	value, _, err := r.Registry.Call(name, args)
	return value, err
}

func (r *Runtime) Context(ctx context.Context) context.Context { return ctx }
