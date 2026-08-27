package tools

import "sync"

type Handler func(map[string]any) (any, error)

type Entry struct {
	Schema  Schema
	Handler Handler
}

type Registry struct {
	mu    sync.RWMutex
	tools map[string]Entry
}

func NewRegistry() *Registry { return &Registry{tools: map[string]Entry{}} }

func (r *Registry) Register(name string, schema Schema, handler Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[name] = Entry{Schema: schema, Handler: handler}
}

func (r *Registry) ListSchemas() []Schema {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Schema, 0, len(r.tools))
	for _, entry := range r.tools {
		out = append(out, entry.Schema)
	}
	return out
}

func (r *Registry) Call(name string, args map[string]any) (any, bool, error) {
	r.mu.RLock()
	entry, ok := r.tools[name]
	r.mu.RUnlock()
	if !ok {
		return nil, false, nil
	}
	value, err := entry.Handler(args)
	return value, true, err
}
