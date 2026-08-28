package tools

import "context"

type Runtime struct{ Registry *Registry }

func NewRuntime() *Runtime {
	r := NewRegistry()
	RegisterCore(r)
	return &Runtime{Registry: r}
}

func (r *Runtime) List() []Schema      { return r.Registry.ListSchemas() }
func (r *Runtime) ListTools() []Schema { return r.List() }
func (r *Runtime) Call(ctx context.Context, name string, args map[string]any) (any, error) {
	return r.Registry.Call(ctx, name, args)
}
