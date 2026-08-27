package tools

import "errors"

type Context struct { WorkingDirectory string; SessionID string }

type Handler func(Context, any) (any, error)

type Registry struct { tools map[string]Handler }

func NewRegistry() *Registry { return &Registry{tools: map[string]Handler{}} }

func (r *Registry) Register(name string, handler Handler) { r.tools[name] = handler }

func (r *Registry) Call(name string, ctx Context, args any) (any, error) {
	if ctx.WorkingDirectory == "" { return nil, errors.New("working_directory is required") }
	h, ok := r.tools[name]; if !ok { return nil, errors.New("tool not found") }
	return h(ctx, args)
}
