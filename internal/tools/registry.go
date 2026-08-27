package tools

import "sync"

type Handler func(map[string]any) (any, error)

type Registry struct {
	mu    sync.RWMutex
	tools map[string]Handler
}

func NewRegistry() *Registry { return &Registry{tools: map[string]Handler{}} }

func (r *Registry) Register(name string, handler Handler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[name] = handler
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

func (r *Registry) Call(name string, args map[string]any) (any, bool, error) {
	r.mu.RLock()
	h, ok := r.tools[name]
	r.mu.RUnlock()
	if !ok {
		return nil, false, nil
	}
	value, err := h(args)
	return value, true, err
}
