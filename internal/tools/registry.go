package tools

import "sync"

type Handler func(map[string]any) (any, error)

type Registry struct {
	mu       sync.RWMutex
	tools    map[string]Schema
	handlers map[string]Handler
}

func NewRegistry() *Registry {
	return &Registry{tools: map[string]Schema{}, handlers: map[string]Handler{}}
}

func (r *Registry) Register(name string, schemaOrHandler any, handlers ...Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var schema Schema
	var handler Handler
	if value, ok := schemaOrHandler.(Schema); ok {
		schema = value
		handler = handlers[0]
	} else {
		handler = schemaOrHandler.(Handler)
		schema = DefaultSchema(name, "")
	}
	schema.Name = name
	r.tools[name] = schema
	r.handlers[name] = handler
}

func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.tools))
	for name := range r.tools {
		out = append(out, name)
	}
	return out
}

func (r *Registry) ListSchemas() []Schema {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Schema, 0, len(r.tools))
	for _, schema := range r.tools {
		out = append(out, schema)
	}
	return out
}

func (r *Registry) Call(name string, args map[string]any) (any, bool, error) {
	r.mu.RLock()
	h, ok := r.handlers[name]
	r.mu.RUnlock()
	if !ok {
		return nil, false, nil
	}
	value, err := h(args)
	return value, true, err
}
